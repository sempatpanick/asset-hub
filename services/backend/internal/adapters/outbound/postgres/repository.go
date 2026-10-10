package postgres

import (
	"assethub/internal/domain"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
	"time"
)

type Repository struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Repository, error) {
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	c.MaxConns = 10
	c.MinConns = 1
	c.ConnConfig.RuntimeParams["timezone"] = "UTC"
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Repository{p}, nil
}
func (r *Repository) Ping(ctx context.Context) error { return r.Pool.Ping(ctx) }
func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" {
			return domain.ErrConflict
		}
		if p.Code == "22P02" || p.Code == "23514" {
			return domain.ErrInvalid
		}
	}
	return err
}
func (r *Repository) transaction(ctx context.Context, f func(pgx.Tx) error) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = f(tx); err != nil {
		return dbError(err)
	}
	return dbError(tx.Commit(ctx))
}
func actor(p domain.Principal) string {
	if p.KeyID != "" {
		return "key:" + p.KeyID
	}
	return p.User.Email
}
func audit(ctx context.Context, tx pgx.Tx, p domain.Principal, action, project, object, detail string) error {
	var pid any
	if project != "" {
		pid = project
	}
	_, err := tx.Exec(ctx, "INSERT INTO audit_events(actor_id,action,project_id,object_id,detail) VALUES($1,$2,$3,$4,$5)", actor(p), action, pid, object, detail)
	return err
}
func (r *Repository) CreateAdmin(ctx context.Context, u domain.User) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(874301)"); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE role='admin'").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return domain.ErrConflict
		}
		_, err := tx.Exec(ctx, "INSERT INTO users(id,email,name,role,password_hash) VALUES($1,$2,$3,'admin',$4)", u.ID, u.Email, u.Name, u.PasswordHash)
		if err != nil {
			return err
		}
		return audit(ctx, tx, domain.Principal{User: u}, "admin.bootstrapped", "", u.ID, "Initial administrator created")
	})
}
func (r *Repository) UserByEmail(ctx context.Context, e string) (domain.User, error) {
	var u domain.User
	err := r.Pool.QueryRow(ctx, "SELECT id::text,email,name,role,password_hash,disabled,created_at FROM users WHERE email=$1", e).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.Disabled, &u.CreatedAt)
	return u, dbError(err)
}
func (r *Repository) CreateSession(ctx context.Context, s domain.Session) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM sessions WHERE expires_at<now()")
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO sessions VALUES($1,$2,$3,$4)", s.Hash, s.UserID, s.CSRF, s.ExpiresAt)
		if err != nil {
			return err
		}
		var e string
		if err = tx.QueryRow(ctx, "SELECT email FROM users WHERE id=$1", s.UserID).Scan(&e); err != nil {
			return err
		}
		return audit(ctx, tx, domain.Principal{User: domain.User{Email: e}}, "auth.login", "", s.UserID, "Dashboard session created")
	})
}
func (r *Repository) AuthenticateSession(ctx context.Context, h string, now time.Time) (domain.Principal, error) {
	var p domain.Principal
	err := r.Pool.QueryRow(ctx, "SELECT u.id::text,u.email,u.name,u.role,u.disabled,u.created_at,s.csrf FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>$2 AND NOT u.disabled", h, now).Scan(&p.User.ID, &p.User.Email, &p.User.Name, &p.User.Role, &p.User.Disabled, &p.User.CreatedAt, &p.CSRF)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrUnauthorized
	}
	return p, err
}
func (r *Repository) AuthenticateKey(ctx context.Context, h string, now time.Time) (domain.Principal, error) {
	var p domain.Principal
	err := r.Pool.QueryRow(ctx, "SELECT id::text,project_id::text,scopes FROM api_keys WHERE token_hash=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>$2)", h, now).Scan(&p.KeyID, &p.ProjectID, &p.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrUnauthorized
	}
	return p, err
}
func (r *Repository) DeleteSession(ctx context.Context, h string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var uid, e string
		err := tx.QueryRow(ctx, "SELECT u.id::text,u.email FROM sessions s JOIN users u ON u.id=s.user_id WHERE token_hash=$1", h).Scan(&uid, &e)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", h); err != nil {
			return err
		}
		return audit(ctx, tx, domain.Principal{User: domain.User{Email: e}}, "auth.logout", "", uid, "Dashboard session revoked")
	})
}
func (r *Repository) Invite(ctx context.Context, p domain.Principal, i domain.Invitation) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", i.Email).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return domain.ErrConflict
		}
		_, err := tx.Exec(ctx, "INSERT INTO invitations(id,email,role,token_hash,expires_at) VALUES($1,$2,$3,$4,$5)", i.ID, i.Email, i.Role, i.Hash, i.ExpiresAt)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "user.invited", "", i.ID, i.Email)
	})
}
func (r *Repository) AcceptInvitation(ctx context.Context, h string, u domain.User, now time.Time) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "UPDATE invitations SET accepted_at=$2 WHERE token_hash=$1 AND accepted_at IS NULL AND expires_at>$2 RETURNING email,role", h, now).Scan(&u.Email, &u.Role); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrConflict
			}
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO users(id,email,name,role,password_hash) VALUES($1,$2,$3,$4,$5)", u.ID, u.Email, u.Name, u.Role, u.PasswordHash)
		if err != nil {
			return err
		}
		return audit(ctx, tx, domain.Principal{User: u}, "user.joined", "", u.ID, "Invitation accepted")
	})
}

const access = "($1 OR p.id::text=$2 OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id::text=$3))"

func (r *Repository) Projects(ctx context.Context, p domain.Principal) ([]domain.Project, error) {
	rows, err := r.Pool.Query(ctx, "SELECT p.id::text,p.name,p.description,p.quota_bytes,p.created_at,COALESCE((SELECT sum(a.size_bytes) FROM assets a WHERE a.project_id=p.id AND a.status NOT IN ('deleted','deleting')),0)::bigint,(SELECT count(*) FROM assets a WHERE a.project_id=p.id AND a.status NOT IN ('deleted','deleting')),CASE WHEN $1 THEN 'manager' WHEN p.id::text=$2 THEN 'application' ELSE (SELECT role FROM project_members WHERE project_id=p.id AND user_id::text=$3) END FROM projects p WHERE "+access+" ORDER BY p.created_at DESC", p.IsAdmin(), p.ProjectID, p.User.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Project{}
	for rows.Next() {
		var v domain.Project
		if err = rows.Scan(&v.ID, &v.Name, &v.Description, &v.QuotaBytes, &v.CreatedAt, &v.UsedBytes, &v.Files, &v.Role); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (r *Repository) CreateProject(ctx context.Context, p domain.Principal, v domain.Project) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO projects(id,name,description,quota_bytes,created_by) VALUES($1,$2,$3,$4,$5)", v.ID, v.Name, v.Description, v.QuotaBytes, p.User.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO project_members VALUES($1,$2,'manager')", v.ID, p.User.ID)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "project.created", v.ID, v.ID, v.Name)
	})
}
func (r *Repository) ProjectRole(ctx context.Context, p domain.Principal, id string) (string, error) {
	var role string
	err := r.Pool.QueryRow(ctx, "SELECT CASE WHEN $1 THEN 'manager' ELSE (SELECT role FROM project_members m WHERE m.project_id=p.id AND m.user_id::text=$3) END FROM projects p WHERE p.id::text=$2 AND ($1 OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id::text=$3))", p.IsAdmin(), id, p.User.ID).Scan(&role)
	return role, dbError(err)
}
func (r *Repository) UpdateProject(ctx context.Context, p domain.Principal, v domain.Project) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var quota int64
		if err := tx.QueryRow(ctx, "SELECT quota_bytes FROM projects WHERE id::text=$1 FOR UPDATE", v.ID).Scan(&quota); err != nil {
			return err
		}
		var used int64
		if err := tx.QueryRow(ctx, "SELECT COALESCE((SELECT sum(size_bytes) FROM assets WHERE project_id::text=$1 AND status NOT IN ('deleted','deleting')),0)::bigint+COALESCE((SELECT sum(size_bytes) FROM uploads WHERE project_id::text=$1 AND (status='completing' OR (status='receiving' AND expires_at>now()))),0)::bigint", v.ID).Scan(&used); err != nil {
			return err
		}
		if err := domain.ValidateQuota(v.QuotaBytes, used); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE projects SET name=$2,description=$3,quota_bytes=$4 WHERE id::text=$1", v.ID, v.Name, v.Description, v.QuotaBytes)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "project.updated", v.ID, v.ID, v.Name)
	})
}
func (r *Repository) Members(ctx context.Context, id string) ([]domain.Member, error) {
	rows, err := r.Pool.Query(ctx, "SELECT u.id::text,u.email,u.name,m.role FROM project_members m JOIN users u ON u.id=m.user_id WHERE m.project_id::text=$1 ORDER BY u.name", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Member{}
	for rows.Next() {
		var m domain.Member
		if err = rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
func (r *Repository) SetMember(ctx context.Context, p domain.Principal, project, e, role string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, "SELECT id::text FROM users WHERE email=$1 AND NOT disabled", e).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT id FROM projects WHERE id::text=$1 FOR UPDATE", project); err != nil {
			return err
		}
		var old string
		err := tx.QueryRow(ctx, "SELECT role FROM project_members WHERE project_id::text=$1 AND user_id::text=$2", project, id).Scan(&old)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var n int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM project_members WHERE project_id::text=$1 AND role='manager'", project).Scan(&n); err != nil {
			return err
		}
		if err = domain.ValidateMemberChange(old, role, n); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT(project_id,user_id) DO UPDATE SET role=excluded.role", project, id, role)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "member.updated", project, id, e+" · "+role)
	})
}
func (r *Repository) Keys(ctx context.Context, p domain.Principal, project string) ([]domain.APIKey, error) {
	rows, err := r.Pool.Query(ctx, "SELECT id::text,project_id::text,name,prefix,scopes,expires_at,revoked_at,created_at FROM api_keys WHERE project_id::text=$1 ORDER BY created_at DESC", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.APIKey{}
	for rows.Next() {
		var k domain.APIKey
		if err = rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.Scopes, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, k)
	}
	return result, rows.Err()
}
func (r *Repository) CreateKey(ctx context.Context, p domain.Principal, k domain.APIKey) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO api_keys(id,project_id,name,prefix,token_hash,scopes,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)", k.ID, k.ProjectID, k.Name, k.Prefix, k.Hash, k.Scopes, k.ExpiresAt)
		if err != nil {
			return err
		}
		return audit(ctx, tx, p, "key.created", k.ProjectID, k.ID, k.Name)
	})
}
func (r *Repository) RevokeKey(ctx context.Context, p domain.Principal, project, id string) error {
	return r.transaction(ctx, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, "UPDATE api_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE id::text=$1 AND project_id::text=$2", id, project)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return audit(ctx, tx, p, "key.revoked", project, id, "Application access revoked")
	})
}
func (r *Repository) Overview(ctx context.Context, p domain.Principal) (domain.Overview, error) {
	v := domain.Overview{Recent: []domain.Asset{}, Activity: []domain.DayCount{}}
	err := r.Pool.QueryRow(ctx, "SELECT count(*),COALESCE(sum(a.size_bytes),0)::bigint,count(*) FILTER(WHERE a.status IN ('queued','uploading','verifying')),count(*) FILTER(WHERE a.status='failed') FROM assets a JOIN projects p ON p.id=a.project_id WHERE "+access+" AND a.status NOT IN ('deleted','deleting')", p.IsAdmin(), p.ProjectID, p.User.ID).Scan(&v.Files, &v.Bytes, &v.Queued, &v.Failed)
	if err != nil {
		return v, err
	}
	if err = r.Pool.QueryRow(ctx, "SELECT count(*) FROM projects p WHERE "+access, p.IsAdmin(), p.ProjectID, p.User.ID).Scan(&v.Projects); err != nil {
		return v, err
	}
	v.Recent, err = r.Assets(ctx, p, "", "", "", "", 6, 0)
	if err != nil {
		return v, err
	}
	rows, err := r.Pool.Query(ctx, "SELECT to_char(d,'YYYY-MM-DD'),(SELECT count(*) FROM assets a JOIN projects p ON p.id=a.project_id WHERE "+access+" AND a.created_at>=d AND a.created_at<d+interval '1 day') FROM generate_series(date_trunc('day',now())-interval '6 days',date_trunc('day',now()),interval '1 day') d", p.IsAdmin(), p.ProjectID, p.User.ID)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var d domain.DayCount
		if err = rows.Scan(&d.Day, &d.Count); err != nil {
			return v, err
		}
		v.Activity = append(v.Activity, d)
	}
	return v, rows.Err()
}
func (r *Repository) Assets(ctx context.Context, p domain.Principal, project, query, status, kind string, limit, offset int) ([]domain.Asset, error) {
	rows, err := r.Pool.Query(ctx, "SELECT a.id::text,a.project_id::text,a.name,a.mime,a.size_bytes,a.sha256,a.status,a.visibility,a.created_at FROM assets a JOIN projects p ON p.id=a.project_id WHERE "+access+" AND (a.status NOT IN ('deleted','deleting') OR ($1 AND $6='deleted' AND a.status='deleted')) AND ($4='' OR p.id::text=$4) AND ($5='' OR position(lower($5) in lower(a.name))>0) AND ($6='' OR a.status=$6) AND ($9='' OR starts_with(a.mime,$9)) ORDER BY a.created_at DESC,a.id DESC LIMIT $7 OFFSET $8", p.IsAdmin(), p.ProjectID, p.User.ID, project, query, status, limit, offset, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Asset{}
	for rows.Next() {
		var a domain.Asset
		if err = rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.MIME, &a.Size, &a.SHA256, &a.Status, &a.Visibility, &a.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
func (r *Repository) Activity(ctx context.Context, p domain.Principal, query, action, project string, limit, offset int) ([]domain.Activity, error) {
	rows, err := r.Pool.Query(ctx, "SELECT e.id,e.actor_id,e.action,COALESCE(e.project_id::text,''),e.object_id,e.detail,e.created_at FROM audit_events e LEFT JOIN projects p ON p.id=e.project_id WHERE ($1 OR (e.project_id IS NOT NULL AND EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=e.project_id AND m.user_id::text=$2)) OR (e.project_id IS NULL AND e.actor_id=$3)) AND ($6='' OR position(lower($6) in lower(concat_ws(' ',e.action,e.actor_id,e.detail,e.object_id)))>0) AND ($7='' OR starts_with(e.action,$7)) AND ($8='' OR e.project_id::text=$8) ORDER BY e.created_at DESC,e.id DESC LIMIT $4 OFFSET $5", p.IsAdmin(), p.User.ID, p.User.Email, limit, offset, query, action, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Activity{}
	for rows.Next() {
		var a domain.Activity
		var id int64
		if err = rows.Scan(&id, &a.Actor, &a.Action, &a.ProjectID, &a.ObjectID, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.ID = strconv.FormatInt(id, 10)
		result = append(result, a)
	}
	return result, rows.Err()
}
