package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("authentication required")
	ErrForbidden    = errors.New("permission denied")
	ErrInvalid      = errors.New("invalid input")
	ErrConflict     = errors.New("already exists or expired")
)

type User struct {
	ID, Email, Name, Role, PasswordHash string
	Disabled                            bool
	CreatedAt                           time.Time
}
type Session struct {
	Hash, UserID, CSRF string
	ExpiresAt          time.Time
}
type Principal struct {
	User             User
	KeyID, ProjectID string
	Scopes           []string
	CSRF             string
}

func (p Principal) IsAdmin() bool { return p.KeyID == "" && p.User.Role == "admin" && !p.User.Disabled }
func (p Principal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

type Project struct {
	ID, Name, Description, Role  string
	QuotaBytes, UsedBytes, Files int64
	CreatedAt                    time.Time
}
type Member struct{ UserID, Email, Name, Role string }
type Invitation struct {
	ID, Email, Role, Hash string
	ExpiresAt             time.Time
}
type APIKey struct {
	ID, ProjectID, Name, Prefix, Hash string
	Scopes                            []string
	ExpiresAt, RevokedAt              *time.Time
	CreatedAt                         time.Time
}
type Asset struct {
	ID, ProjectID, Name, MIME, Status, Visibility, SHA256 string
	Size                                                  int64
	CreatedAt                                             time.Time
}
type Activity struct {
	ID, Actor, Action, ProjectID, ObjectID, Detail string
	CreatedAt                                      time.Time
}
type DayCount struct {
	Day   string
	Count int64
}
type Overview struct {
	Files, Bytes, Projects, Queued, Failed int64
	Recent                                 []Asset
	Activity                               []DayCount
}
type Health struct{ Component, Status, Detail string }

func ValidRole(role string) bool { return role == "manager" || role == "editor" || role == "viewer" }
func ValidScope(scope string) bool {
	return scope == "assets:read" || scope == "assets:write" || scope == "assets:delete" || scope == "assets:sign"
}
func CleanName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if !utf8.ValidString(n) || utf8.RuneCountInString(n) < 1 || utf8.RuneCountInString(n) > 180 || strings.ContainsAny(n, "\x00\r\n") {
		return "", ErrInvalid
	}
	return n, nil
}

func ValidateQuota(quota, used int64) error {
	if quota < 1<<20 {
		return ErrInvalid
	}
	if quota < used {
		return ErrConflict
	}
	return nil
}

func ValidateMemberChange(previous, next string, managerCount int) error {
	if !ValidRole(next) {
		return ErrInvalid
	}
	if previous == "manager" && next != "manager" && managerCount <= 1 {
		return ErrConflict
	}
	return nil
}
