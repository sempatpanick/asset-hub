package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"strings"
	"time"
)

type Security struct{}

func (Security) ID() string { return uuid.NewString() }
func (Security) Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("entropy unavailable")
	}
	return hex.EncodeToString(b)
}
func (Security) HashToken(t string) string {
	b := sha256.Sum256([]byte(t))
	return hex.EncodeToString(b[:])
}
func (Security) HashPassword(p string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(p), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(h)), nil
}
func (Security) VerifyPassword(encoded, p string) bool {
	if encoded == "" {
		// An unknown account takes the same expensive verification path.
		encoded = "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	if len(p) > 256 {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=2" {
		return false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[4])
	expected, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(salt) != 16 || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(p), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

type Clock struct{}

func (Clock) Now() time.Time { return time.Now().UTC() }
