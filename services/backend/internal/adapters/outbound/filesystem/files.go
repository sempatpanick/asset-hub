package filesystem

import (
	"assethub/internal/domain"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// All paths are derived from server-generated IDs, never client filenames.
func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

type Staging struct {
	Root           string
	ReservePercent *uint64
}

func (s *Staging) partPath(id string, p domain.UploadPart) (string, error) {
	if !validID(id) || p.Index < 0 || len(p.SHA256) != 64 {
		return "", domain.ErrInvalid
	}
	if _, err := hex.DecodeString(p.SHA256); err != nil {
		return "", domain.ErrInvalid
	}
	return filepath.Join(s.Root, id, fmt.Sprintf("%d-%s.part", p.Index, p.SHA256)), nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}
func (s *Staging) Put(ctx context.Context, id string, p domain.UploadPart, reader io.Reader) error {
	path, err := s.partPath(id, p)
	if err != nil {
		return err
	}
	if p.Size < 1 || p.Size > domain.UploadPartSize {
		return domain.ErrInvalid
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err = checkSpace(s.Root, p.Size, s.ReservePercent); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".part-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	hash := sha256.New()
	err = f.Chmod(0600)
	var n int64
	if err == nil {
		n, err = io.Copy(io.MultiWriter(f, hash), io.LimitReader(contextReader{ctx, reader}, p.Size+1))
	}
	if err == nil && (n != p.Size || hex.EncodeToString(hash.Sum(nil)) != p.SHA256) {
		err = domain.ErrIntegrity
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func (s *Staging) Open(id string, p domain.UploadPart) (io.ReadCloser, error) {
	path, err := s.partPath(id, p)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, domain.ErrIntegrity
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != p.Size {
		f.Close()
		return nil, domain.ErrIntegrity
	}
	return f, nil
}
func (s *Staging) Remove(id string) error {
	if !validID(id) {
		return domain.ErrInvalid
	}
	return os.RemoveAll(filepath.Join(s.Root, id))
}

type Backup struct {
	Root, StagingRoot  string
	RequireIndependent bool
	ReservePercent     *uint64
}

func (b *Backup) Check() error {
	info, err := os.Lstat(filepath.Join(b.Root, ".assethub-backup"))
	if err != nil || !info.Mode().IsRegular() {
		return &domain.StorageError{Code: "backup_unavailable"}
	}
	if b.RequireIndependent {
		same, err := sameVolume(b.Root, b.StagingRoot)
		if err != nil || same {
			return &domain.StorageError{Code: "backup_not_independent"}
		}
	}
	return nil
}
func (b *Backup) path(id string, index int) (string, error) {
	if !validID(id) || index < 0 {
		return "", domain.ErrInvalid
	}
	if err := b.Check(); err != nil {
		return "", err
	}
	return filepath.Join(b.Root, "assets", id, strconv.Itoa(index)+".chunk"), nil
}
func (b *Backup) PutChunk(id string, c domain.Chunk, data []byte) error {
	path, err := b.path(id, c.Index)
	if err != nil {
		return err
	}
	if int64(len(data)) != c.CipherSize || digest(data) != c.SHA256 {
		return domain.ErrIntegrity
	}
	if err = checkSpace(b.Root, int64(len(data)), b.ReservePercent); err != nil {
		return &domain.StorageError{Code: "backup_capacity"}
	}
	if err = atomicWrite(path, data); err != nil {
		return &domain.StorageError{Code: "backup_write_failed"}
	}
	_, err = b.OpenChunk(id, c)
	return err
}
func (b *Backup) OpenChunk(id string, c domain.Chunk) ([]byte, error) {
	path, err := b.path(id, c.Index)
	if err != nil {
		return nil, err
	}
	if c.CipherSize < 1 || c.CipherSize > domain.StorageChunkSize+28 {
		return nil, domain.ErrIntegrity
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, &domain.StorageError{Code: "backup_read_failed"}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, c.CipherSize+1))
	if err != nil || int64(len(data)) != c.CipherSize || digest(data) != c.SHA256 {
		return nil, domain.ErrIntegrity
	}
	return data, nil
}
func (b *Backup) CommitManifest(id string, data []byte) error {
	if !validID(id) || len(data) > 1<<20 {
		return domain.ErrInvalid
	}
	if err := b.Check(); err != nil {
		return err
	}
	path := filepath.Join(b.Root, "assets", id, "manifest.enc")
	if err := atomicWrite(path, data); err != nil {
		return &domain.StorageError{Code: "backup_write_failed"}
	}
	actual, err := os.ReadFile(path)
	if err != nil || digest(actual) != digest(data) {
		return domain.ErrIntegrity
	}
	return b.Check()
}

// Local is an explicitly selected development provider, with opaque safe references.
func (b *Backup) RemoveAsset(id string) error {
	if !validID(id) {
		return domain.ErrInvalid
	}
	if err := b.Check(); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(b.Root, "assets", id))
}

type Local struct {
	Root           string
	ReservePercent *uint64
}

func (l *Local) Name() string { return "local" }
func (l *Local) Put(ctx context.Context, name string, data []byte) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	parts := strings.Split(name, "/")
	if len(parts) != 2 || !validID(parts[0]) {
		return nil, domain.ErrInvalid
	}
	i, err := strconv.Atoi(parts[1])
	if err != nil || i < 0 {
		return nil, domain.ErrInvalid
	}
	if err = os.MkdirAll(l.Root, 0700); err != nil {
		return nil, domain.ErrUnavailable
	}
	if err = checkSpace(l.Root, int64(len(data)), l.ReservePercent); err != nil {
		return nil, err
	}
	if err = atomicWrite(filepath.Join(l.Root, parts[0], parts[1]), data); err != nil {
		return nil, domain.ErrUnavailable
	}
	return []byte(name), nil
}
func (l *Local) refPath(ref []byte) (string, error) {
	parts := strings.Split(string(ref), "/")
	if len(parts) != 2 || !validID(parts[0]) {
		return "", domain.ErrInvalid
	}
	i, err := strconv.Atoi(parts[1])
	if err != nil || i < 0 || strconv.Itoa(i) != parts[1] {
		return "", domain.ErrInvalid
	}
	return filepath.Join(l.Root, parts[0], parts[1]), nil
}
func (l *Local) Open(ctx context.Context, ref []byte) ([]byte, error) {
	p, err := l.refPath(ref)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(contextReader{ctx, f}, domain.StorageChunkSize+29))
}
func (l *Local) Delete(ctx context.Context, ref []byte) error {
	p, err := l.refPath(ref)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (l *Local) Health(ctx context.Context) domain.Health {
	return domain.Health{Component: "local", Status: "development", Detail: "Local development storage selected; not Telegram"}
}

func reservePercent(value *uint64) uint64 {
	if value == nil {
		return 10
	}
	return *value
}

func DiskHealth(path, component string) domain.Health {
	h := domain.Health{Component: component, Status: "healthy"}
	available, total, err := diskStats(path)
	if err != nil {
		h.Status = "unavailable"
		h.Detail = "Filesystem unavailable"
		return h
	}
	h.Detail = fmt.Sprintf("%d MiB available; 10%% production reserve required", available/(1<<20))
	if available < total/10 {
		h.Status = "degraded"
	}
	return h
}
