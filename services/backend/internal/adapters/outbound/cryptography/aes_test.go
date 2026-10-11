package cryptography

import (
	"bytes"
	"testing"
)

func TestAuthenticatedChunkAndEnvelopeBinding(t *testing.T) {
	a, _ := New(bytes.Repeat([]byte{9}, 32))
	key, wrapped, err := a.NewKey("asset-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.UnwrapKey("asset-b", wrapped); err == nil {
		t.Fatal("key envelope accepted wrong asset")
	}
	cipher, err := a.Encrypt("asset-a", 3, key, []byte("original data"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := a.Decrypt("asset-a", 3, key, cipher)
	if err != nil || string(plain) != "original data" {
		t.Fatal("round trip failed")
	}
	if _, err = a.Decrypt("asset-a", 4, key, cipher); err == nil {
		t.Fatal("chunk index substitution accepted")
	}
	cipher[len(cipher)-1] ^= 1
	if _, err = a.Decrypt("asset-a", 3, key, cipher); err == nil {
		t.Fatal("tampering accepted")
	}
	other, _ := New(bytes.Repeat([]byte{7}, 32))
	if _, err = other.UnwrapKey("asset-a", wrapped); err == nil {
		t.Fatal("wrong master key accepted")
	}
	ref, _ := a.ProtectReference("asset-a", 3, []byte("private locator"))
	if bytes.Contains(ref, []byte("private locator")) {
		t.Fatal("plaintext locator exposed")
	}
	if _, err = a.OpenReference("asset-b", 3, ref); err == nil {
		t.Fatal("reference substitution accepted")
	}
}

func TestAPIKeySecretBinding(t *testing.T) {
	a, _ := New(bytes.Repeat([]byte{9}, 32))
	secret := []byte("ah_test-secret")
	cipher, err := a.ProtectKeySecret("project-a", "key-a", secret)
	if err != nil || bytes.Contains(cipher, secret) {
		t.Fatal("secret not protected", err)
	}
	plain, err := a.OpenKeySecret("project-a", "key-a", cipher)
	if err != nil || !bytes.Equal(plain, secret) {
		t.Fatal("secret round trip failed", err)
	}
	for _, pair := range [][2]string{{"project-b", "key-a"}, {"project-a", "key-b"}} {
		if _, err = a.OpenKeySecret(pair[0], pair[1], cipher); err == nil {
			t.Fatal("secret substitution accepted")
		}
	}
	other, _ := New(bytes.Repeat([]byte{8}, 32))
	if _, err = other.OpenKeySecret("project-a", "key-a", cipher); err == nil {
		t.Fatal("wrong installation key accepted")
	}
	cipher[len(cipher)-1] ^= 1
	if _, err = a.OpenKeySecret("project-a", "key-a", cipher); err == nil {
		t.Fatal("tampering accepted")
	}
}
