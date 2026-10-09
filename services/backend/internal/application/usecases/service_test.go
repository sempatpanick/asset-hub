package usecases

import (
	"assethub/internal/application/ports"
	"assethub/internal/domain"
	"context"
	"errors"
	"testing"
	"time"
)

type roleRepo struct {
	ports.Repository
	role    string
	err     error
	updates int
}

func (r *roleRepo) ProjectRole(context.Context, domain.Principal, string) (string, error) {
	return r.role, r.err
}
func (r *roleRepo) SetMember(context.Context, domain.Principal, string, string, string) error {
	r.updates++
	return nil
}

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC) }
func TestAuthorizationMatrix(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		role, action string
		allowed      bool
	}{{"viewer", "read", true}, {"viewer", "write", false}, {"viewer", "delete", false}, {"viewer", "sign", false}, {"editor", "read", true}, {"editor", "write", true}, {"editor", "delete", true}, {"editor", "sign", true}, {"manager", "manage", true}} {
		t.Run(tc.role+"/"+tc.action, func(t *testing.T) {
			s := New(&roleRepo{role: tc.role}, nil, clock{}, nil)
			err := s.Require(ctx, domain.Principal{}, "project", tc.action)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v err=%v", tc.allowed, err)
			}
		})
	}
}
func TestKeyProjectAndScopeIsolation(t *testing.T) {
	s := New(&roleRepo{role: "manager"}, nil, clock{}, nil)
	p := domain.Principal{KeyID: "key", ProjectID: "one", Scopes: []string{"assets:read"}}
	if err := s.Require(context.Background(), p, "two", "read"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.Require(context.Background(), p, "one", "write"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
	if err := s.Require(context.Background(), p, "one", "read"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject(context.Background(), p, "Project", "", 1<<30); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal(err)
	}
}
func TestViewerCannotMutateMembers(t *testing.T) {
	repo := &roleRepo{role: "viewer"}
	s := New(repo, nil, clock{}, nil)
	err := s.SetMember(context.Background(), domain.Principal{}, "p", "user@example.test", "manager")
	if !errors.Is(err, domain.ErrForbidden) || repo.updates != 0 {
		t.Fatal("viewer changed membership", err)
	}
}

func TestDashboardPrivilegesAreCheckedInUseCases(t *testing.T) {
	s := New(nil, nil, clock{}, nil)
	if _, err := s.Health(context.Background(), domain.Principal{User: domain.User{Role: "member"}}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("member could inspect installation health")
	}
	if _, err := s.Overview(context.Background(), domain.Principal{KeyID: "key"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("application key could inspect dashboard overview")
	}
}
