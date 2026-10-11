package usecases

import (
	"assethub/internal/application/ports"
	"assethub/internal/domain"
	"context"
	"net/mail"
	"strings"
	"time"
)

type Service struct {
	repo       ports.Repository
	security   ports.Security
	clock      ports.Clock
	health     ports.RuntimeHealth
	ingestion  *Ingestion
	keySecrets ports.KeySecrets
}

func (s *Service) SetKeySecrets(secrets ports.KeySecrets) { s.keySecrets = secrets }

func New(repo ports.Repository, security ports.Security, clock ports.Clock, health ports.RuntimeHealth) *Service {
	return &Service{repo: repo, security: security, clock: clock, health: health}
}
func email(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	m, e := mail.ParseAddress(v)
	if e != nil || m.Address != v || len(v) > 254 {
		return "", domain.ErrInvalid
	}
	return v, nil
}
func (s *Service) Bootstrap(ctx context.Context, address, name, password string) error {
	e, err := email(address)
	if err != nil {
		return err
	}
	n, err := domain.CleanName(name)
	if err != nil {
		return err
	}
	if len(password) < 12 || len(password) > 256 {
		return domain.ErrInvalid
	}
	h, err := s.security.HashPassword(password)
	if err != nil {
		return err
	}
	return s.repo.CreateAdmin(ctx, domain.User{ID: s.security.ID(), Email: e, Name: n, Role: "admin", PasswordHash: h, CreatedAt: s.clock.Now()})
}
func (s *Service) Login(ctx context.Context, address, password string) (domain.User, string, string, error) {
	e, err := email(address)
	if err != nil {
		return domain.User{}, "", "", domain.ErrUnauthorized
	}
	u, err := s.repo.UserByEmail(ctx, e)
	verified := s.security.VerifyPassword(u.PasswordHash, password)
	if err != nil || u.Disabled || !verified {
		return domain.User{}, "", "", domain.ErrUnauthorized
	}
	token, csrf := s.security.Token(), s.security.Token()
	err = s.repo.CreateSession(ctx, domain.Session{Hash: s.security.HashToken(token), UserID: u.ID, CSRF: csrf, ExpiresAt: s.clock.Now().Add(12 * time.Hour)})
	return u, token, csrf, err
}
func (s *Service) Session(ctx context.Context, token string) (domain.Principal, error) {
	if len(token) != 64 {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	return s.repo.AuthenticateSession(ctx, s.security.HashToken(token), s.clock.Now())
}
func (s *Service) Key(ctx context.Context, token string) (domain.Principal, error) {
	if !strings.HasPrefix(token, "ah_") || len(token) != 67 {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	return s.repo.AuthenticateKey(ctx, s.security.HashToken(token), s.clock.Now())
}
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.repo.DeleteSession(ctx, s.security.HashToken(token))
}
func (s *Service) Invite(ctx context.Context, p domain.Principal, address, role string) (string, error) {
	if !p.IsAdmin() {
		return "", domain.ErrForbidden
	}
	e, err := email(address)
	if err != nil {
		return "", err
	}
	if role != "member" && role != "admin" {
		return "", domain.ErrInvalid
	}
	token := s.security.Token()
	return token, s.repo.Invite(ctx, p, domain.Invitation{ID: s.security.ID(), Email: e, Role: role, Hash: s.security.HashToken(token), ExpiresAt: s.clock.Now().Add(48 * time.Hour)})
}
func (s *Service) Accept(ctx context.Context, token, name, password string) error {
	if len(token) != 64 || len(password) < 12 || len(password) > 256 {
		return domain.ErrInvalid
	}
	n, err := domain.CleanName(name)
	if err != nil {
		return err
	}
	h, err := s.security.HashPassword(password)
	if err != nil {
		return err
	}
	return s.repo.AcceptInvitation(ctx, s.security.HashToken(token), domain.User{ID: s.security.ID(), Name: n, PasswordHash: h}, s.clock.Now())
}
func (s *Service) Projects(ctx context.Context, p domain.Principal) ([]domain.Project, error) {
	return s.repo.Projects(ctx, p)
}
func (s *Service) CreateProject(ctx context.Context, p domain.Principal, name, description string, quota int64) (domain.Project, error) {
	if p.KeyID != "" {
		return domain.Project{}, domain.ErrForbidden
	}
	n, err := domain.CleanName(name)
	if err != nil || len(description) > 2000 || quota < 1<<20 {
		return domain.Project{}, domain.ErrInvalid
	}
	v := domain.Project{ID: s.security.ID(), Name: n, Description: strings.TrimSpace(description), QuotaBytes: quota, Role: "manager", CreatedAt: s.clock.Now()}
	return v, s.repo.CreateProject(ctx, p, v)
}
func (s *Service) Require(ctx context.Context, p domain.Principal, project, action string) error {
	if p.KeyID != "" {
		if p.ProjectID != project {
			return domain.ErrNotFound
		}
		if !p.HasScope("assets:" + action) {
			return domain.ErrForbidden
		}
		return nil
	}
	r, err := s.repo.ProjectRole(ctx, p, project)
	if err != nil {
		return err
	}
	if action == "read" || r == "manager" || (r == "editor" && (action == "write" || action == "delete" || action == "sign")) {
		return nil
	}
	return domain.ErrForbidden
}
func (s *Service) UpdateProject(ctx context.Context, p domain.Principal, v domain.Project) error {
	if p.KeyID != "" {
		return domain.ErrForbidden
	}
	r, err := s.repo.ProjectRole(ctx, p, v.ID)
	if err != nil {
		return err
	}
	if r != "manager" {
		return domain.ErrForbidden
	}
	v.Name, err = domain.CleanName(v.Name)
	if err != nil || len(v.Description) > 2000 || v.QuotaBytes < 1<<20 {
		return domain.ErrInvalid
	}
	return s.repo.UpdateProject(ctx, p, v)
}
func (s *Service) Members(ctx context.Context, p domain.Principal, project string) ([]domain.Member, error) {
	if p.KeyID != "" {
		return nil, domain.ErrForbidden
	}
	if err := s.Require(ctx, p, project, "read"); err != nil {
		return nil, err
	}
	return s.repo.Members(ctx, project)
}
func (s *Service) SetMember(ctx context.Context, p domain.Principal, project, address, role string) error {
	if p.KeyID != "" {
		return domain.ErrForbidden
	}
	r, err := s.repo.ProjectRole(ctx, p, project)
	if err != nil {
		return err
	}
	if r != "manager" {
		return domain.ErrForbidden
	}
	e, err := email(address)
	if err != nil || !domain.ValidRole(role) {
		return domain.ErrInvalid
	}
	return s.repo.SetMember(ctx, p, project, e, role)
}
func (s *Service) Keys(ctx context.Context, p domain.Principal, project string) ([]domain.APIKey, error) {
	if p.KeyID != "" {
		return nil, domain.ErrForbidden
	}
	r, err := s.repo.ProjectRole(ctx, p, project)
	if err != nil {
		return nil, err
	}
	if r != "manager" {
		return nil, domain.ErrForbidden
	}
	return s.repo.Keys(ctx, p, project)
}
func (s *Service) CreateKey(ctx context.Context, p domain.Principal, project, name string, scopes []string, expiry *time.Time) (domain.APIKey, string, error) {
	if _, err := s.Keys(ctx, p, project); err != nil {
		return domain.APIKey{}, "", err
	}
	n, err := domain.CleanName(name)
	if err != nil || len(scopes) < 1 || len(scopes) > 4 {
		return domain.APIKey{}, "", domain.ErrInvalid
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if !domain.ValidScope(scope) || seen[scope] {
			return domain.APIKey{}, "", domain.ErrInvalid
		}
		seen[scope] = true
	}
	if expiry != nil && !expiry.After(s.clock.Now()) {
		return domain.APIKey{}, "", domain.ErrInvalid
	}
	secret := "ah_" + s.security.Token()
	k := domain.APIKey{ID: s.security.ID(), ProjectID: project, Name: n, Prefix: secret[:11], Hash: s.security.HashToken(secret), Scopes: scopes, ExpiresAt: expiry, CreatedAt: s.clock.Now()}
	if s.keySecrets == nil {
		return domain.APIKey{}, "", domain.ErrUnavailable
	}
	k.EncryptedSecret, err = s.keySecrets.ProtectKeySecret(project, k.ID, []byte(secret))
	if err != nil {
		return domain.APIKey{}, "", err
	}
	k.SecretAvailable = true
	return k, secret, s.repo.CreateKey(ctx, p, k)
}
func (s *Service) RevealKey(ctx context.Context, p domain.Principal, project, id, password string) (string, error) {
	if p.KeyID != "" || p.User.ID == "" {
		return "", domain.ErrForbidden
	}
	role, err := s.repo.ProjectRole(ctx, p, project)
	if err != nil {
		return "", err
	}
	if role != "manager" {
		return "", domain.ErrForbidden
	}
	if len(password) == 0 || len(password) > 256 {
		return "", domain.ErrPassword
	}
	u, err := s.repo.UserByEmail(ctx, p.User.Email)
	if err != nil || u.ID != p.User.ID || u.Disabled || !s.security.VerifyPassword(u.PasswordHash, password) {
		return "", domain.ErrPassword
	}
	if s.keySecrets == nil {
		return "", domain.ErrUnavailable
	}
	encrypted, err := s.repo.KeySecret(ctx, p, project, id, s.clock.Now())
	if err != nil {
		return "", err
	}
	plain, err := s.keySecrets.OpenKeySecret(project, id, encrypted)
	if err != nil {
		return "", err
	}
	defer clear(plain)
	return string(plain), nil
}
func (s *Service) RevokeKey(ctx context.Context, p domain.Principal, project, id string) error {
	if _, err := s.Keys(ctx, p, project); err != nil {
		return err
	}
	return s.repo.RevokeKey(ctx, p, project, id)
}
func (s *Service) Overview(ctx context.Context, p domain.Principal) (domain.Overview, error) {
	if p.KeyID != "" {
		return domain.Overview{}, domain.ErrForbidden
	}
	return s.repo.Overview(ctx, p)
}
func (s *Service) Assets(ctx context.Context, p domain.Principal, project, query, status, kind string, limit, offset int) ([]domain.Asset, error) {
	if project != "" {
		if err := s.Require(ctx, p, project, "read"); err != nil {
			return nil, err
		}
	} else if p.KeyID != "" && !p.HasScope("assets:read") {
		return nil, domain.ErrForbidden
	}
	if kind != "" && kind != "image/" && kind != "video/" && kind != "audio/" && kind != "text/" && kind != "application/" {
		return nil, domain.ErrInvalid
	}
	return s.repo.Assets(ctx, p, project, query, status, kind, limit, offset)
}
func (s *Service) Activity(ctx context.Context, p domain.Principal, query, action, project string, limit, offset int) ([]domain.Activity, error) {
	if p.KeyID != "" {
		return nil, domain.ErrForbidden
	}
	return s.repo.Activity(ctx, p, query, action, project, limit, offset)
}
func (s *Service) Health(ctx context.Context, p domain.Principal) ([]domain.Health, error) {
	if !p.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	values := s.health.Check(ctx)
	if s.ingestion != nil {
		health, err := s.ingestion.repo.JobHealth(ctx)
		if err != nil {
			return nil, err
		}
		values = append(values, health)
	}
	return values, nil
}
func (s *Service) Ready(ctx context.Context) error {
	if err := s.repo.Ping(ctx); err != nil {
		return err
	}
	return s.health.Ready(ctx)
}
