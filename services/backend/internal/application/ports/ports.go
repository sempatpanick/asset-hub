package ports

import (
	"assethub/internal/domain"
	"context"
	"time"
)

type Repository interface {
	Ping(context.Context) error
	CreateAdmin(context.Context, domain.User) error
	UserByEmail(context.Context, string) (domain.User, error)
	CreateSession(context.Context, domain.Session) error
	AuthenticateSession(context.Context, string, time.Time) (domain.Principal, error)
	AuthenticateKey(context.Context, string, time.Time) (domain.Principal, error)
	DeleteSession(context.Context, string) error
	Invite(context.Context, domain.Principal, domain.Invitation) error
	AcceptInvitation(context.Context, string, domain.User, time.Time) error
	Projects(context.Context, domain.Principal) ([]domain.Project, error)
	CreateProject(context.Context, domain.Principal, domain.Project) error
	ProjectRole(context.Context, domain.Principal, string) (string, error)
	UpdateProject(context.Context, domain.Principal, domain.Project) error
	Members(context.Context, string) ([]domain.Member, error)
	SetMember(context.Context, domain.Principal, string, string, string) error
	Keys(context.Context, domain.Principal, string) ([]domain.APIKey, error)
	CreateKey(context.Context, domain.Principal, domain.APIKey) error
	RevokeKey(context.Context, domain.Principal, string, string) error
	Overview(context.Context, domain.Principal) (domain.Overview, error)
	Assets(context.Context, domain.Principal, string, string, string, string, int, int) ([]domain.Asset, error)
	Activity(context.Context, domain.Principal, int, int) ([]domain.Activity, error)
}
type Security interface {
	ID() string
	Token() string
	HashToken(string) string
	HashPassword(string) (string, error)
	VerifyPassword(string, string) bool
}
type Clock interface{ Now() time.Time }
type RuntimeHealth interface {
	Check(context.Context) []domain.Health
	Ready(context.Context) error
}
