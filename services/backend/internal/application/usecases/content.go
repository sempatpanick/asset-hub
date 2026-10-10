package usecases

import (
	"assethub/internal/application/ports"
	"assethub/internal/domain"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

type chunkFlight struct {
	done chan struct{}
	data []byte
	err  error
}
type contentState struct {
	cache   ports.ChunkCache
	signer  ports.ContentSigner
	mu      sync.Mutex
	flights map[string]*chunkFlight
}

func (i *Ingestion) ConfigureContent(cache ports.ChunkCache, signer ports.ContentSigner) {
	i.content = &contentState{cache: cache, signer: signer, flights: map[string]*chunkFlight{}}
}
func (i *Ingestion) ContentEnabled() bool { return i.Enabled() && i.content != nil }
func (i *Ingestion) SignedLink(ctx context.Context, p domain.Principal, id string, seconds int) (string, error) {
	if !i.ContentEnabled() || i.content.signer == nil {
		return "", domain.ErrUnavailable
	}
	a, err := i.repo.Asset(ctx, id)
	if err != nil {
		return "", err
	}
	if err = i.auth.Require(ctx, p, a.ProjectID, "sign"); err != nil {
		return "", err
	}
	if a.Status != "ready" {
		return "", domain.ErrConflict
	}
	if seconds == 0 {
		seconds = 900
	}
	if seconds < 1 || seconds > 86400 {
		return "", domain.ErrInvalid
	}
	token, err := i.content.signer.Sign(domain.ContentClaims{AssetID: id, Version: a.Version, Expires: i.auth.clock.Now().Add(time.Duration(seconds) * time.Second).Unix()})
	if err != nil {
		return "", err
	}
	if err = i.repo.AuditAction(ctx, p, "asset.link_created", a.ProjectID, id, fmt.Sprintf("Content capability issued for %d seconds", seconds)); err != nil {
		return "", err
	}
	return token, nil
}
func (i *Ingestion) AuthorizeContent(ctx context.Context, p domain.Principal, id, token string) (domain.StoredAsset, error) {
	if !i.ContentEnabled() {
		return domain.StoredAsset{}, domain.ErrUnavailable
	}
	a, err := i.repo.Asset(ctx, id)
	if err != nil {
		return a, err
	}
	if a.Status != "ready" {
		return domain.StoredAsset{}, domain.ErrNotFound
	}
	if token != "" {
		if i.content.signer == nil {
			return domain.StoredAsset{}, domain.ErrUnauthorized
		}
		claims, err := i.content.signer.Verify(token, i.auth.clock.Now())
		if err != nil || claims.AssetID != id || claims.Version != a.Version {
			return domain.StoredAsset{}, domain.ErrUnauthorized
		}
		return a, nil
	}
	if a.Visibility == "public" {
		return a, nil
	}
	if p.KeyID == "" && p.User.ID == "" {
		return domain.StoredAsset{}, domain.ErrUnauthorized
	}
	if err = i.auth.Require(ctx, p, a.ProjectID, "read"); err != nil {
		return domain.StoredAsset{}, err
	}
	return a, nil
}
func (i *Ingestion) Update(ctx context.Context, p domain.Principal, id string, name, visibility *string) error {
	a, err := i.repo.Asset(ctx, id)
	if err != nil {
		return err
	}
	if a.Status == "deleted" {
		return domain.ErrNotFound
	}
	if name == nil && visibility == nil {
		return domain.ErrInvalid
	}
	if err = i.auth.Require(ctx, p, a.ProjectID, "write"); err != nil {
		return err
	}
	if name != nil {
		a.Name, err = domain.CleanName(*name)
		if err != nil {
			return err
		}
	}
	if visibility != nil {
		if p.KeyID != "" {
			return domain.ErrForbidden
		}
		role, e := i.auth.repo.ProjectRole(ctx, p, a.ProjectID)
		if e != nil {
			return e
		}
		if role != "manager" {
			return domain.ErrForbidden
		}
		if *visibility != "private" && *visibility != "public" {
			return domain.ErrInvalid
		}
		a.Visibility = *visibility
	}
	return i.repo.UpdateAsset(ctx, p, id, a.Name, a.Visibility)
}
func (i *Ingestion) Delete(ctx context.Context, p domain.Principal, id string) error {
	a, err := i.repo.Asset(ctx, id)
	if err != nil {
		return err
	}
	if err = i.auth.Require(ctx, p, a.ProjectID, "delete"); err != nil {
		return err
	}
	if a.Status == "deleted" {
		return nil
	}
	return i.repo.DeleteAsset(ctx, p, id)
}
func (i *Ingestion) Restore(ctx context.Context, p domain.Principal, id string) error {
	if !p.IsAdmin() {
		return domain.ErrForbidden
	}
	if !i.Enabled() || !i.BackupEnabled() {
		return domain.ErrUnavailable
	}
	a, err := i.repo.Asset(ctx, id)
	if err != nil {
		return err
	}
	if a.Status != "deleted" || a.BackupPurged || a.DeletedAt == nil || !a.DeletedAt.Add(30*24*time.Hour).After(i.auth.clock.Now()) {
		return domain.ErrConflict
	}
	key, err := i.crypto.UnwrapKey(id, a.WrappedKey)
	if err != nil {
		return err
	}
	hash := sha256.New()
	var size int64
	for index, c := range a.Chunks {
		if c.Index != index {
			return domain.ErrIntegrity
		}
		data, err := i.backup.OpenChunk(id, c)
		if err != nil {
			return domain.ErrUnavailable
		}
		plain, err := i.crypto.Decrypt(id, index, key, data)
		if err != nil {
			return err
		}
		hash.Write(plain)
		size += int64(len(plain))
	}
	if size != a.Size || hex.EncodeToString(hash.Sum(nil)) != a.SHA256 {
		return domain.ErrIntegrity
	}
	return i.repo.RestoreAsset(ctx, p, id)
}
func (i *Ingestion) chunk(ctx context.Context, a domain.StoredAsset, c domain.Chunk) ([]byte, error) {
	id := fmt.Sprintf("%s:%d", a.ID, c.Index)
	state := i.content
	state.mu.Lock()
	if flight := state.flights[id]; flight != nil {
		state.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return flight.data, flight.err
		}
	}
	flight := &chunkFlight{done: make(chan struct{})}
	state.flights[id] = flight
	state.mu.Unlock()
	data, err := i.fetchChunk(ctx, a, c)
	state.mu.Lock()
	flight.data, flight.err = data, err
	delete(state.flights, id)
	close(flight.done)
	state.mu.Unlock()
	return data, err
}
func (i *Ingestion) fetchChunk(ctx context.Context, a domain.StoredAsset, c domain.Chunk) ([]byte, error) {
	valid := func(data []byte) bool {
		sum := sha256.Sum256(data)
		return int64(len(data)) == c.CipherSize && hex.EncodeToString(sum[:]) == c.SHA256
	}
	if c.CipherSize < 28 || c.CipherSize > domain.StorageChunkSize+28 {
		return nil, domain.ErrIntegrity
	}
	if i.content.cache != nil {
		data, err := i.content.cache.Get(a.ID, c)
		if err == nil && len(data) > 0 && valid(data) {
			return data, nil
		}
	}
	var data []byte
	var err error = domain.ErrUnavailable
	if a.Provider == i.provider.Name() && c.PurgeStatus != "purged" {
		ref, e := i.crypto.OpenReference(a.ID, c.Index, c.Reference)
		if e == nil {
			data, err = i.provider.Open(ctx, ref)
		}
		if err == nil && !valid(data) {
			err = domain.ErrIntegrity
		}
	}
	if err != nil && i.BackupEnabled() {
		data, err = i.backup.OpenChunk(a.ID, c)
	}
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	if !valid(data) {
		return nil, domain.ErrIntegrity
	}
	if i.content.cache != nil {
		_ = i.content.cache.Put(a.ID, c, data)
	}
	return data, nil
}

type contentReader struct {
	ctx             context.Context
	ingest          *Ingestion
	asset           domain.StoredAsset
	key             []byte
	start, end, pos int64
	chunk           int
	reader          *bytes.Reader
	closed          bool
}

func (i *Ingestion) OpenContent(ctx context.Context, a domain.StoredAsset, start, end int64) (io.ReadCloser, error) {
	if start < 0 || end < start || end >= a.Size {
		return nil, domain.ErrInvalid
	}
	key, err := i.crypto.UnwrapKey(a.ID, a.WrappedKey)
	if err != nil {
		return nil, err
	}
	reader := &contentReader{ctx: ctx, ingest: i, asset: a, key: key, start: start, end: end, pos: start, chunk: int(start / domain.StorageChunkSize)}
	if err = reader.load(); err != nil {
		return nil, err
	}
	return reader, nil
}
func (r *contentReader) load() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.chunk >= len(r.asset.Chunks) || r.asset.Chunks[r.chunk].Index != r.chunk {
		return domain.ErrIntegrity
	}
	c := r.asset.Chunks[r.chunk]
	current, err := r.ingest.repo.Asset(r.ctx, r.asset.ID)
	if err != nil || current.Status != "ready" || current.Version != r.asset.Version {
		return domain.ErrNotFound
	}
	data, err := r.ingest.chunk(r.ctx, r.asset, c)
	if err != nil {
		return err
	}
	plain, err := r.ingest.crypto.Decrypt(r.asset.ID, r.chunk, r.key, data)
	if err != nil || int64(len(plain)) != c.PlainSize {
		return domain.ErrIntegrity
	}
	offset := r.pos - int64(r.chunk)*domain.StorageChunkSize
	if offset < 0 || offset >= int64(len(plain)) {
		return domain.ErrIntegrity
	}
	length := min(int64(len(plain))-offset, r.end-r.pos+1)
	r.reader = bytes.NewReader(plain[offset : offset+length])
	return nil
}
func (r *contentReader) Read(b []byte) (int, error) {
	if r.closed || r.pos > r.end {
		return 0, io.EOF
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(b)
	r.pos += int64(n)
	if err == io.EOF && r.pos <= r.end {
		r.chunk++
		if err = r.load(); err != nil {
			return 0, err
		}
		return r.Read(b)
	}
	return n, err
}
func (r *contentReader) Close() error { r.closed = true; clear(r.key); r.reader = nil; return nil }
func (i *Ingestion) processDeletion(ctx context.Context, a domain.StoredAsset, lease string) error {
	for _, c := range a.Chunks {
		if c.PurgeStatus != "" || len(c.Reference) == 0 {
			continue
		}
		ref, err := i.crypto.OpenReference(a.ID, c.Index, c.Reference)
		if err != nil {
			if err = i.repo.SavePurge(ctx, a.ID, lease, c.Index, "unavailable"); err != nil {
				return err
			}
			continue
		}
		status := "purged"
		if a.Provider != i.provider.Name() {
			status = "unavailable"
		} else {
			chunkCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			err = i.provider.Delete(chunkCtx, ref)
			cancel()
			if err != nil {
				var storage *domain.StorageError
				if (errors.As(err, &storage) && storage.Permanent) || a.Attempts >= 8 {
					status = "unavailable"
				} else {
					return err
				}
			}
		}
		if err = i.repo.SavePurge(ctx, a.ID, lease, c.Index, status); err != nil {
			return err
		}
	}
	return i.repo.FinishPurge(ctx, a.ID, lease)
}
