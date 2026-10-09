package main

import (
	"assethub/cmd/internal/bootstrap"
	"assethub/migrations"
	"bufio"
	"context"
	"database/sql"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"golang.org/x/term"
	"os"
	"strings"
	"time"
)

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: admin migrate|bootstrap")
	}
	cfg, err := bootstrap.Read()
	if err != nil {
		return err
	}
	if os.Args[1] == "migrate" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		db, err := sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("invalid database configuration")
		}
		defer db.Close()
		goose.SetBaseFS(migrations.Files)
		if err = goose.SetDialect("postgres"); err != nil {
			return err
		}
		if err = goose.UpContext(ctx, db, "."); err != nil {
			return fmt.Errorf("database migration failed")
		}
		return applicationRole(ctx, db, os.Getenv("APP_DATABASE_PASSWORD"))
	}
	if os.Args[1] != "bootstrap" {
		return fmt.Errorf("unknown command")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("bootstrap requires an interactive terminal; credentials must not be command arguments")
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Administrator email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	fmt.Print("Display name: ")
	name, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	fmt.Print("Password (at least 12 characters): ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	defer clear(password)
	fmt.Println()
	if err != nil {
		return err
	}
	fmt.Print("Confirm password: ")
	confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
	defer clear(confirmation)
	fmt.Println()
	if err != nil {
		return err
	}
	if string(password) != string(confirmation) {
		return fmt.Errorf("passwords do not match")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	app, close, err := bootstrap.Wire(ctx, cfg)
	if err != nil {
		return err
	}
	defer close()
	err = app.Bootstrap(ctx, strings.TrimSpace(email), strings.TrimSpace(name), string(password))
	clear(password)
	clear(confirmation)
	if err != nil {
		return err
	}
	fmt.Println("Administrator created. Sign in through the dashboard.")
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Command failed:", err)
		os.Exit(1)
	}
}
