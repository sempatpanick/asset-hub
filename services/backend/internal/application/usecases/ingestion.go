package usecases

import (
	"assethub/internal/application/ports"
	"assethub/internal/domain"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"time"
)

type Ingestion struct {
	auth      *Service
	repo      ports.UploadRepository
	stage     ports.Staging
	crypto    ports.Cryptography
	provider  ports.StorageProvider
	backup    ports.Backup
	queue     ports.JobQueue
	capacity  int64
	content   *contentState
	inspector ports.FileInspector
}

func NewIngestion(auth *Service, repo ports.UploadRepository, stage ports.Staging, crypto ports.Cryptography, provider ports.StorageProvider, backup ports.Backup, queue ports.JobQueue, capacity int64, inspector ports.FileInspector) *Ingestion {
	return &Ingestion{auth: auth, repo: repo, stage: stage, crypto: crypto, provider: provider, backup: backup, queue: queue, capacity: capacity, inspector: inspector}
}
func (s *Service) SetIngestion(i *Ingestion) { s.ingestion = i }
func (s *Service) Ingestion() *Ingestion     { return s.ingestion }
func (i *Ingestion) Enabled() bool           { return i != nil && i.crypto != nil }
func (i *Ingestion) ProviderName() string {
	if i == nil {
		return "telegram"
	}
	return i.provider.Name()
}
func validHash(hash string) bool {
	if len(hash) != 64 || strings.ToLower(hash) != hash {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}
func (i *Ingestion) Create(ctx context.Context, p domain.Principal, project, name string, size int64, hash string) (domain.Upload, error) {
	if !i.Enabled() {
		return domain.Upload{}, domain.ErrUnavailable
	}
	if err := i.auth.Require(ctx, p, project, "write"); err != nil {
		return domain.Upload{}, err
	}
	n, err := domain.CleanName(name)
	if err != nil || size < 1 || size > domain.MaxAssetSize || (hash != "" && !validHash(hash)) {
		return domain.Upload{}, domain.ErrInvalid
	}
	if err = i.backup.Check(); err != nil {
		return domain.Upload{}, domain.ErrUnavailable
	}
	u := domain.Upload{ID: i.auth.security.ID(), ProjectID: project, Name: n, Size: size, Status: "receiving", ExpectedSHA256: hash, Parts: []domain.UploadPart{}, CreatedAt: i.auth.clock.Now(), ExpiresAt: i.auth.clock.Now().Add(24 * time.Hour)}
	return u, i.repo.ReserveUpload(ctx, p, u, i.capacity)
}
func (i *Ingestion) Get(ctx context.Context, p domain.Principal, id, action string) (domain.Upload, error) {
	u, err := i.repo.Upload(ctx, id)
	if err != nil {
		return u, err
	}
	if err = i.auth.Require(ctx, p, u.ProjectID, action); err != nil {
		return domain.Upload{}, err
	}
	return u, nil
}
func (i *Ingestion) List(ctx context.Context, p domain.Principal, limit, offset int) ([]domain.Upload, error) {
	if p.KeyID != "" && !p.HasScope("assets:read") {
		return nil, domain.ErrForbidden
	}
	return i.repo.Uploads(ctx, p, limit, offset)
}
func (i *Ingestion) PutPart(ctx context.Context, p domain.Principal, id string, index int, hash string, body io.Reader) error {
	if !i.Enabled() {
		return domain.ErrUnavailable
	}
	u, err := i.Get(ctx, p, id, "write")
	if err != nil {
		return err
	}
	if u.Status != "receiving" || !u.ExpiresAt.After(i.auth.clock.Now()) {
		return domain.ErrConflict
	}
	n, err := u.PartLength(index)
	if err != nil || !validHash(hash) {
		return domain.ErrInvalid
	}
	for _, part := range u.Parts {
		if part.Index == index {
			if part.SHA256 != hash {
				return domain.ErrConflict
			}
			break
		}
	}
	part := domain.UploadPart{Index: index, Size: n, SHA256: hash}
	if err = i.stage.Put(ctx, id, part, body); err != nil {
		return err
	}
	return i.repo.RecordPart(ctx, id, part)
}
func (i *Ingestion) Cancel(ctx context.Context, p domain.Principal, id string) error {
	u, err := i.Get(ctx, p, id, "write")
	if err != nil {
		return err
	}
	if err = i.repo.CancelUpload(ctx, p, u.ID); err != nil {
		return err
	}
	// Cleanup is retried by the worker if the filesystem is unavailable.
	_ = i.stage.Remove(id) // Final cleanup waits for bounded in-flight HTTP transfers to finish.
	return nil
}
func (i *Ingestion) Complete(ctx context.Context, p domain.Principal, id string) (domain.Asset, error) {
	if !i.Enabled() {
		return domain.Asset{}, domain.ErrUnavailable
	}
	u, err := i.Get(ctx, p, id, "write")
	if err != nil {
		return domain.Asset{}, err
	}
	if u.AssetID != "" && (u.Status == "queued" || u.Status == "complete") {
		a, err := i.repo.Asset(ctx, u.AssetID)
		return a.Asset, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	u, err = i.repo.BeginCompletion(ctx, id)
	if err != nil {
		return domain.Asset{}, err
	}
	committed := false
	defer func() {
		if !committed {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = i.repo.ResetCompletion(cleanup, id)
		}
	}()
	count := int((u.Size + domain.UploadPartSize - 1) / domain.UploadPartSize)
	if len(u.Parts) != count {
		return domain.Asset{}, domain.ErrConflict
	}
	full := sha256.New()
	prefix := []byte{}
	var size int64
	for index, part := range u.Parts {
		n, err := u.PartLength(index)
		if err != nil || part.Index != index || part.Size != n {
			return domain.Asset{}, domain.ErrIntegrity
		}
		data, err := i.readPart(ctx, u.ID, part)
		if err != nil {
			return domain.Asset{}, err
		}
		full.Write(data)
		size += int64(len(data))
		if len(prefix) < 512 {
			remaining := 512 - len(prefix)
			if remaining > len(data) {
				remaining = len(data)
			}
			prefix = append(prefix, data[:remaining]...)
		}
	}
	hash := hex.EncodeToString(full.Sum(nil))
	if size != u.Size || (u.ExpectedSHA256 != "" && u.ExpectedSHA256 != hash) {
		return domain.Asset{}, domain.ErrIntegrity
	}
	idAsset := i.auth.security.ID()
	_, wrapped, err := i.crypto.NewKey(idAsset)
	if err != nil {
		return domain.Asset{}, err
	}
	a := domain.StoredAsset{Asset: domain.Asset{ID: idAsset, ProjectID: u.ProjectID, Name: u.Name, MIME: i.inspector.MIME(prefix), Size: u.Size, SHA256: hash, Status: "queued", Visibility: "private", CreatedAt: i.auth.clock.Now()}, UploadID: u.ID, Provider: i.provider.Name(), WrappedKey: wrapped}
	if err = ctx.Err(); err != nil {
		return domain.Asset{}, err
	}
	if err = i.repo.CompleteUpload(ctx, p, u, a); err != nil {
		return domain.Asset{}, err
	}
	committed = true
	// Redis enqueue is an optimization. The database outbox always remains authoritative.
	_ = i.queue.Push(ctx, []string{a.ID})
	return a.Asset, nil
}
func (i *Ingestion) readPart(ctx context.Context, id string, part domain.UploadPart) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := i.stage.Open(id, part)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, part.Size+1))
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	if int64(len(data)) != part.Size || hex.EncodeToString(hash[:]) != part.SHA256 {
		return nil, domain.ErrIntegrity
	}
	return data, nil
}
func (i *Ingestion) Metadata(ctx context.Context, p domain.Principal, id string) (domain.StoredAsset, error) {
	a, err := i.repo.Asset(ctx, id)
	if err != nil {
		return a, err
	}
	if err = i.auth.Require(ctx, p, a.ProjectID, "read"); err != nil {
		return domain.StoredAsset{}, err
	}
	if a.Status == "deleted" || a.Status == "deleting" {
		return domain.StoredAsset{}, domain.ErrNotFound
	}
	return a, nil
}
func (i *Ingestion) Retry(ctx context.Context, p domain.Principal, id string) error {
	a, err := i.repo.Asset(ctx, id)
	if err != nil {
		return err
	}
	if err = i.auth.Require(ctx, p, a.ProjectID, "write"); err != nil {
		return err
	}
	return i.repo.RetryAsset(ctx, p, id)
}
func (i *Ingestion) Dispatch(ctx context.Context) error {
	if !i.Enabled() {
		return domain.ErrUnavailable
	}
	ids, err := i.repo.DueJobs(ctx, 200)
	if err != nil {
		return err
	}
	if err = i.queue.Push(ctx, ids); err != nil {
		return err
	}
	return i.queue.Heartbeat(ctx)
}
func (i *Ingestion) Cleanup(ctx context.Context) error {
	ids, err := i.repo.CleanupUploads(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = i.stage.Remove(id); err != nil {
			return err
		}
		if err = i.repo.MarkCleaned(ctx, id); err != nil {
			return err
		}
	}
	expired, err := i.repo.ExpiredBackups(ctx)
	if err != nil {
		return err
	}
	for _, id := range expired {
		if err = i.backup.RemoveAsset(id); err != nil {
			return err
		}
		if err = i.repo.MarkBackupPurged(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (i *Ingestion) ProcessNext(ctx context.Context) (bool, error) {
	if !i.Enabled() {
		return false, domain.ErrUnavailable
	}
	id, err := i.queue.Pop(ctx)
	if err != nil || id == "" {
		return false, err
	}
	lease := i.auth.security.ID()
	a, err := i.repo.ClaimJob(ctx, id, lease)
	if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	if a.Status == "deleted" {
		err = i.processDeletion(ctx, a, lease)
	} else {
		err = i.process(ctx, a, lease)
	}
	if err == nil {
		return true, nil
	}
	code := "storage_unavailable"
	delay := time.Duration(1<<min(a.Attempts, 8))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	permanent := a.Attempts >= 8
	var storage *domain.StorageError
	if errors.As(err, &storage) {
		code = storage.Code
		permanent = permanent || storage.Permanent
		if storage.RetryAfter > delay {
			delay = storage.RetryAfter
		}
	}
	if errors.Is(err, domain.ErrIntegrity) {
		code = "integrity_failed"
		permanent = true
	}
	// Cancellation does not lose the durable job; releasing the lease enables prompt restart.
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if failure := i.repo.FailJob(cleanup, id, lease, code, delay, permanent); failure != nil {
		return true, failure
	}
	return true, nil
}
func (i *Ingestion) process(ctx context.Context, a domain.StoredAsset, lease string) error {
	if a.Provider != i.provider.Name() {
		return &domain.StorageError{Code: "provider_changed", Permanent: true}
	}
	if err := i.backup.Check(); err != nil {
		return err
	}
	key, err := i.crypto.UnwrapKey(a.ID, a.WrappedKey)
	if err != nil {
		return err
	}
	u, err := i.repo.Upload(ctx, a.UploadID)
	if err != nil {
		return err
	}
	count := int((a.Size + domain.StorageChunkSize - 1) / domain.StorageChunkSize)
	chunks := make(map[int]domain.Chunk)
	for _, c := range a.Chunks {
		chunks[c.Index] = c
	}
	a.Chunks = nil
	for index := 0; index < count; index++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		c, exists := chunks[index]
		var ciphertext []byte
		if exists {
			ciphertext, err = i.backup.OpenChunk(a.ID, c)
			if err != nil {
				return err
			}
		} else {
			plain := []byte{}
			for partIndex := index * 2; partIndex < min(index*2+2, len(u.Parts)); partIndex++ {
				data, e := i.readPart(ctx, u.ID, u.Parts[partIndex])
				if e != nil {
					return e
				}
				plain = append(plain, data...)
			}
			expected := min(domain.StorageChunkSize, a.Size-int64(index)*domain.StorageChunkSize)
			if int64(len(plain)) != expected {
				return domain.ErrIntegrity
			}
			ciphertext, err = i.crypto.Encrypt(a.ID, index, key, plain)
			if err != nil {
				return err
			}
			hash := sha256.Sum256(ciphertext)
			c = domain.Chunk{Index: index, PlainSize: expected, CipherSize: int64(len(ciphertext)), SHA256: hex.EncodeToString(hash[:])}
			if err = i.backup.PutChunk(a.ID, c, ciphertext); err != nil {
				return err
			}
			if err = i.repo.SaveChunk(ctx, a.ID, lease, c); err != nil {
				return err
			}
		}
		// Every network chunk operation is bounded below the database lease duration.
		chunkCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		if len(c.Reference) == 0 {
			raw, e := i.provider.Put(chunkCtx, fmt.Sprintf("%s/%d", a.ID, index), ciphertext)
			if e != nil {
				cancel()
				return e
			}
			c.Reference, err = i.crypto.ProtectReference(a.ID, index, raw)
			if err != nil {
				cancel()
				return err
			}
			if err = i.repo.SaveChunk(ctx, a.ID, lease, c); err != nil {
				cancel()
				return err
			}
		}
		raw, e := i.crypto.OpenReference(a.ID, index, c.Reference)
		if e != nil {
			cancel()
			return e
		}
		actual, e := i.provider.Open(chunkCtx, raw)
		cancel()
		if e != nil {
			return e
		}
		hash := sha256.Sum256(actual)
		if int64(len(actual)) != c.CipherSize || hex.EncodeToString(hash[:]) != c.SHA256 {
			return domain.ErrIntegrity
		}
		if _, err = i.crypto.Decrypt(a.ID, index, key, actual); err != nil {
			return err
		}
		// Renew the lease after retrieval, including retries of already uploaded chunks.
		if err = i.repo.SaveChunk(ctx, a.ID, lease, c); err != nil {
			return err
		}
		a.Chunks = append(a.Chunks, c)
	}
	// Recovery manifest includes protected references, key envelope, sizes and hashes.
	a.Status = "ready"
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	manifest, err := i.crypto.ProtectManifest(a.ID, data)
	if err != nil {
		return err
	}
	if err = i.backup.CommitManifest(a.ID, manifest); err != nil {
		return err
	}
	return i.repo.ReadyAsset(ctx, a.ID, lease)
}
