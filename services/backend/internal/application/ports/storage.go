package ports

import (
	"assethub/internal/domain"
	"context"
	"io"
	"time"
)

type UploadRepository interface {
	ReserveUpload(context.Context, domain.Principal, domain.Upload, int64) error
	Upload(context.Context, string) (domain.Upload, error)
	Uploads(context.Context, domain.Principal, int, int) ([]domain.Upload, error)
	RecordPart(context.Context, string, domain.UploadPart) error
	BeginCompletion(context.Context, string) (domain.Upload, error)
	ResetCompletion(context.Context, string) error
	CompleteUpload(context.Context, domain.Principal, domain.Upload, domain.StoredAsset) error
	CancelUpload(context.Context, domain.Principal, string) error
	Asset(context.Context, string) (domain.StoredAsset, error)
	DueJobs(context.Context, int) ([]string, error)
	ClaimJob(context.Context, string, string) (domain.StoredAsset, error)
	SaveChunk(context.Context, string, string, domain.Chunk) error
	ReadyAsset(context.Context, string, string) error
	FailJob(context.Context, string, string, string, time.Duration, bool) error
	RetryAsset(context.Context, domain.Principal, string) error
	CleanupUploads(context.Context) ([]string, error)
	MarkCleaned(context.Context, string) error
	UpdateAsset(context.Context, domain.Principal, string, string, string) error
	DeleteAsset(context.Context, domain.Principal, string) error
	RestoreAsset(context.Context, domain.Principal, string) error
	SavePurge(context.Context, string, string, int, string) error
	FinishPurge(context.Context, string, string) error
	ExpiredBackups(context.Context) ([]string, error)
	MarkBackupPurged(context.Context, string) error
	JobHealth(context.Context) (domain.Health, error)
	AuditAction(context.Context, domain.Principal, string, string, string, string) error
}
type Staging interface {
	Put(context.Context, string, domain.UploadPart, io.Reader) error
	Open(string, domain.UploadPart) (io.ReadCloser, error)
	Remove(string) error
}
type FileInspector interface{ MIME([]byte) string }

// UploadCheckpoints retain ciphertext during unfinished uploads, not for recovery.
type UploadCheckpoints interface {
	PutCheckpoint(string, domain.Chunk, []byte) error
	OpenCheckpoint(string, domain.Chunk) ([]byte, error)
}
type Cryptography interface {
	NewKey(string) ([]byte, []byte, error)
	UnwrapKey(string, []byte) ([]byte, error)
	Encrypt(string, int, []byte, []byte) ([]byte, error)
	Decrypt(string, int, []byte, []byte) ([]byte, error)
	ProtectReference(string, int, []byte) ([]byte, error)
	OpenReference(string, int, []byte) ([]byte, error)
	ProtectManifest(string, []byte) ([]byte, error)
}

// Provider references are opaque bytes. Transport-specific values stay inside adapters.
type StorageProvider interface {
	Name() string
	Put(context.Context, string, []byte) ([]byte, error)
	Open(context.Context, []byte) ([]byte, error)
	Delete(context.Context, []byte) error
	Health(context.Context) domain.Health
}
type Backup interface {
	Check() error
	PutChunk(string, domain.Chunk, []byte) error
	OpenChunk(string, domain.Chunk) ([]byte, error)
	CommitManifest(string, []byte) error
	RemoveAsset(string) error
}
type ChunkCache interface {
	Get(string, domain.Chunk) ([]byte, error)
	Put(string, domain.Chunk, []byte) error
}
type ContentSigner interface {
	Sign(domain.ContentClaims) (string, error)
	Verify(string, time.Time) (domain.ContentClaims, error)
}
type JobQueue interface {
	Push(context.Context, []string) error
	Pop(context.Context) (string, error)
	Heartbeat(context.Context) error
}
