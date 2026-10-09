package domain

import (
	"errors"
	"time"
)

const UploadPartSize int64 = 8 << 20
const StorageChunkSize int64 = 16 << 20
const MaxAssetSize int64 = 512 << 20

var ErrUnavailable = errors.New("service unavailable")
var ErrCapacity = errors.New("capacity exceeded")
var ErrIntegrity = errors.New("integrity verification failed")

type Upload struct {
	ID, ProjectID, Name, Status, AssetID, ExpectedSHA256, Owner string
	Size                                                        int64
	Parts                                                       []UploadPart
	ExpiresAt, CreatedAt                                        time.Time
	AssetStatus, ErrorCode                                      string
	UploadedChunks                                              int
}
type UploadPart struct {
	Index  int
	Size   int64
	SHA256 string
}

func (u Upload) PartLength(index int) (int64, error) {
	if index < 0 || int64(index)*UploadPartSize >= u.Size {
		return 0, ErrInvalid
	}
	n := u.Size - int64(index)*UploadPartSize
	if n > UploadPartSize {
		n = UploadPartSize
	}
	return n, nil
}

type StoredAsset struct {
	Asset
	UploadID, Provider, ErrorCode string
	WrappedKey                    []byte
	Chunks                        []Chunk
	Attempts                      int
	Version                       int
	DeletedAt                     *time.Time
	BackupPurged                  bool
}
type Chunk struct {
	Index       int
	PlainSize   int64
	SHA256      string
	CipherSize  int64
	Reference   []byte
	PurgeStatus string
}
type ContentClaims struct {
	AssetID string
	Version int
	Expires int64
}

// Provider errors never carry token-bearing URLs or private locators.
type StorageError struct {
	Code                 string
	RetryAfter           time.Duration
	Permanent, Ambiguous bool
}

func (e *StorageError) Error() string { return e.Code }
