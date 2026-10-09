package main

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
)

func applicationRole(ctx context.Context, db *sql.DB, secret string) error {
	if secret == "" {
		return nil
	} // Standalone development migrators may use a pre-provisioned role.
	if !regexp.MustCompile(`^[a-fA-F0-9]{64}$`).MatchString(secret) {
		return fmt.Errorf("APP_DATABASE_PASSWORD must be 64 hexadecimal characters")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("role provisioning failed")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(874303)"); err != nil {
		return fmt.Errorf("role provisioning failed")
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='assethub_api')").Scan(&exists); err != nil {
		return fmt.Errorf("role provisioning failed")
	}
	if !exists {
		if _, err = tx.ExecContext(ctx, "CREATE ROLE assethub_api LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION"); err != nil {
			return fmt.Errorf("role provisioning failed")
		}
	}
	var statement string
	if err = tx.QueryRowContext(ctx, "SELECT format('ALTER ROLE assethub_api WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L',$1::text)", secret).Scan(&statement); err != nil {
		return fmt.Errorf("role provisioning failed")
	}
	if _, err = tx.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("role provisioning failed")
	}
	for _, grant := range []string{"GRANT USAGE ON SCHEMA public TO assethub_api", "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO assethub_api", "GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO assethub_api"} {
		if _, err = tx.ExecContext(ctx, grant); err != nil {
			return fmt.Errorf("application grants failed")
		}
	}
	return tx.Commit()
}
