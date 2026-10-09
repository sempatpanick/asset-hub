package security

import (
	"assethub/internal/domain"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type SignedURLs struct{ key []byte }

func NewSigner(key []byte) (*SignedURLs, error) {
	if len(key) != 32 {
		return nil, domain.ErrInvalid
	}
	return &SignedURLs{append([]byte(nil), key...)}, nil
}
func (s *SignedURLs) Sign(claims domain.ContentClaims) (string, error) {
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (s *SignedURLs) Verify(token string, now time.Time) (domain.ContentClaims, error) {
	var claims domain.ContentClaims
	if len(token) > 1024 {
		return claims, domain.ErrUnauthorized
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, domain.ErrUnauthorized
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, domain.ErrUnauthorized
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return claims, domain.ErrUnauthorized
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(data, &claims) != nil || claims.Expires <= now.Unix() || claims.Version < 1 || claims.AssetID == "" {
		return domain.ContentClaims{}, domain.ErrUnauthorized
	}
	return claims, nil
}
