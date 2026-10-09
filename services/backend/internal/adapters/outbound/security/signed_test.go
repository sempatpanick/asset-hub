package security

import (
	"assethub/internal/domain"
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSignedCapabilitiesExpireAndRejectTampering(t *testing.T) {
	s, _ := NewSigner(bytes.Repeat([]byte{4}, 32))
	now := time.Now()
	claims := domain.ContentClaims{AssetID: "asset", Version: 3, Expires: now.Add(time.Minute).Unix()}
	token, err := s.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	value, err := s.Verify(token, now)
	if err != nil || value != claims {
		t.Fatal("round trip failed")
	}
	if _, err = s.Verify(token, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired link accepted")
	}
	parts := strings.Split(token, ".")
	parts[0] = "e30"
	if _, err = s.Verify(strings.Join(parts, "."), now); err == nil {
		t.Fatal("payload tampering accepted")
	}
	other, _ := NewSigner(bytes.Repeat([]byte{9}, 32))
	if _, err = other.Verify(token, now); err == nil {
		t.Fatal("wrong signing key accepted")
	}
}
