package cryptography

import (
	"assethub/internal/domain"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

type AES struct{ master []byte }

func (a *AES) ProtectKeySecret(project, id string, data []byte) ([]byte, error) {
	sealed, err := seal(a.master, data, "assethub:v1:api-key:"+project+":"+id)
	return append([]byte{1}, sealed...), err
}
func (a *AES) OpenKeySecret(project, id string, data []byte) ([]byte, error) {
	if len(data) < 1 || data[0] != 1 {
		return nil, domain.ErrIntegrity
	}
	return open(a.master, data[1:], "assethub:v1:api-key:"+project+":"+id)
}

func New(master []byte) (*AES, error) {
	if len(master) != 32 {
		return nil, domain.ErrInvalid
	}
	return &AES{append([]byte(nil), master...)}, nil
}
func seal(key, data []byte, aad string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, data, []byte(aad)), nil
}
func open(key, data []byte, aad string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, domain.ErrIntegrity
	}
	g, err := cipher.NewGCM(block)
	if err != nil || len(data) < g.NonceSize()+g.Overhead() {
		return nil, domain.ErrIntegrity
	}
	plain, err := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte(aad))
	if err != nil {
		return nil, domain.ErrIntegrity
	}
	return plain, nil
}
func (a *AES) NewKey(id string) ([]byte, []byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, nil, err
	}
	wrapped, err := seal(a.master, key, "assethub:v1:key:"+id)
	return key, append([]byte{1}, wrapped...), err
}
func (a *AES) UnwrapKey(id string, wrapped []byte) ([]byte, error) {
	if len(wrapped) < 1 || wrapped[0] != 1 {
		return nil, domain.ErrIntegrity
	}
	return open(a.master, wrapped[1:], "assethub:v1:key:"+id)
}
func (a *AES) Encrypt(id string, index int, key, plain []byte) ([]byte, error) {
	return seal(key, plain, fmt.Sprintf("assethub:v1:chunk:%s:%d", id, index))
}
func (a *AES) Decrypt(id string, index int, key, data []byte) ([]byte, error) {
	return open(key, data, fmt.Sprintf("assethub:v1:chunk:%s:%d", id, index))
}
func (a *AES) ProtectReference(id string, index int, data []byte) ([]byte, error) {
	return seal(a.master, data, fmt.Sprintf("assethub:v1:reference:%s:%d", id, index))
}
func (a *AES) OpenReference(id string, index int, data []byte) ([]byte, error) {
	return open(a.master, data, fmt.Sprintf("assethub:v1:reference:%s:%d", id, index))
}
func (a *AES) ProtectManifest(id string, data []byte) ([]byte, error) {
	v, err := seal(a.master, data, "assethub:v1:manifest:"+id)
	return append([]byte{1}, v...), err
}
