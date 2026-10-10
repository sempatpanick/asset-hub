package filesystem

import (
	"assethub/internal/domain"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

func (s *Staging) checkpointPath(id string, c domain.Chunk) (string, error) {
	if !validID(id) || c.Index < 0 || c.CipherSize < 28 || c.CipherSize > domain.StorageChunkSize+28 {
		return "", domain.ErrInvalid
	}
	return filepath.Join(s.Root, id, "checkpoints", strconv.Itoa(c.Index)+".enc"), nil
}
func (s *Staging) PutCheckpoint(id string, c domain.Chunk, data []byte) error {
	path, err := s.checkpointPath(id, c)
	if err != nil {
		return err
	}
	if int64(len(data)) != c.CipherSize || digest(data) != c.SHA256 {
		return domain.ErrIntegrity
	}
	if err = checkSpace(s.Root, int64(len(data)), s.ReservePercent); err != nil {
		return err
	}
	return atomicWrite(path, data)
}
func (s *Staging) OpenCheckpoint(id string, c domain.Chunk) ([]byte, error) {
	path, err := s.checkpointPath(id, c)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != c.CipherSize {
		return nil, domain.ErrIntegrity
	}
	data, err := io.ReadAll(io.LimitReader(f, c.CipherSize+1))
	if err != nil || int64(len(data)) != c.CipherSize || digest(data) != c.SHA256 {
		return nil, domain.ErrIntegrity
	}
	return data, nil
}
