package filesystem

import (
	"assethub/internal/domain"
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestEncryptedCheckpointIntegrityAndCleanup(t *testing.T) {
	zero := uint64(0)
	stage := &Staging{Root: t.TempDir(), ReservePercent: &zero}
	id := "d04f9983-91a6-4663-9f40-5c952ab9fd94"
	data := bytes.Repeat([]byte{42}, 50)
	c := domain.Chunk{Index: 0, CipherSize: int64(len(data)), SHA256: digest(data)}
	if err := stage.PutCheckpoint(id, c, data); err != nil {
		t.Fatal(err)
	}
	got, err := stage.OpenCheckpoint(id, c)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("checkpoint differs", err)
	}
	if err := stage.PutCheckpoint("../../unsafe", c, data); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("path traversal accepted")
	}
	path, _ := stage.checkpointPath(id, c)
	if err := os.WriteFile(path, bytes.Repeat([]byte{43}, len(data)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.OpenCheckpoint(id, c); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("corrupt checkpoint accepted", err)
	}
	if err := stage.Remove(id); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.OpenCheckpoint(id, c); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("checkpoint survived cleanup", err)
	}
}
