package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestRuntimeDatabasePrivileges(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("isolated PostgreSQL test instance required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	owner, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal("invalid test connection")
	}
	defer owner.Close()
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		t.Fatal(err)
	}
	name := "assethub_privileges_" + hex.EncodeToString(id)
	if _, err = owner.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal("cannot create test database")
	}
	defer owner.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
	u, _ := url.Parse(base)
	u.Path = "/" + name
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal("cannot connect to test database")
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, "CREATE TABLE privilege_probe(id bigserial PRIMARY KEY, value text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(secret)
	if err = applicationRole(ctx, db, password); err != nil {
		t.Fatal(err)
	}
	if err = applicationRole(ctx, db, password); err != nil {
		t.Fatal("role provisioning is not repeatable", err)
	}
	u.User = url.UserPassword("assethub_api", password)
	runtimeDB, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal("cannot open runtime connection")
	}
	defer runtimeDB.Close()
	var privileged bool
	if err = runtimeDB.QueryRowContext(ctx, "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication FROM pg_roles WHERE rolname=current_user").Scan(&privileged); err != nil || privileged {
		t.Fatal("runtime role has elevated privileges")
	}
	var value string
	if err = runtimeDB.QueryRowContext(ctx, "INSERT INTO privilege_probe(value) VALUES('persistent') RETURNING value").Scan(&value); err != nil || value != "persistent" {
		t.Fatal("runtime table/sequence grants failed")
	}
	if _, err = runtimeDB.ExecContext(ctx, "UPDATE privilege_probe SET value='updated'"); err != nil {
		t.Fatal(err)
	}
	if _, err = runtimeDB.ExecContext(ctx, "DELETE FROM privilege_probe"); err != nil {
		t.Fatal(err)
	}
	if _, err = runtimeDB.ExecContext(ctx, "ALTER TABLE privilege_probe ADD COLUMN forbidden text"); err == nil {
		t.Fatal("runtime role changed the schema")
	}
	if _, err = runtimeDB.ExecContext(ctx, "CREATE TABLE forbidden_probe(id int)"); err == nil {
		t.Fatal("runtime role created a table")
	}
	if err = applicationRole(ctx, db, "invalid"); err == nil {
		t.Fatal("invalid runtime secret accepted")
	}
}
