package filesystem

import (
	"assethub/internal/domain"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStagingIntegrityAndBackupFailClosed(t *testing.T) {
	root := t.TempDir()
	zero := uint64(0)
	stage := &Staging{Root: filepath.Join(root, "staging"), ReservePercent: &zero}
	id := "d04f9983-91a6-4663-9f40-5c952ab9fd94"
	data := []byte("exact bytes")
	part := domain.UploadPart{Index: 0, Size: int64(len(data)), SHA256: digest(data)}
	if err := stage.Put(context.Background(), id, part, bytes.NewReader([]byte("wrong bytes"))); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("checksum mismatch accepted")
	}
	if err := stage.Put(context.Background(), id, part, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	backup := &Backup{Root: filepath.Join(root, "backup"), StagingRoot: stage.Root, ReservePercent: &zero}
	os.MkdirAll(backup.Root, 0700)
	if err := backup.Check(); err == nil {
		t.Fatal("missing marker accepted")
	}
	marker := filepath.Join(backup.Root, ".assethub-backup")
	os.WriteFile(marker, []byte("fixture"), 0600)
	backup.RequireIndependent = true
	if err := backup.Check(); err == nil {
		t.Fatal("same filesystem accepted as independent")
	}
	backup.RequireIndependent = false
	c := domain.Chunk{Index: 0, CipherSize: int64(len(data)), SHA256: digest(data)}
	if err := backup.PutChunk(id, c, data); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(backup.Root, "assets", id, "0.chunk"), []byte("corrupted"), 0600)
	if _, err := backup.OpenChunk(id, c); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("corrupted backup accepted")
	}
	if err := stage.Remove("../../unsafe"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unsafe stage path accepted")
	}
}
