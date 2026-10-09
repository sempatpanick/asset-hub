package main

import (
	"assethub/cmd/internal/bootstrap"
	"assethub/internal/adapters/inbound/httpapi"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := bootstrap.Logger()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := &http.Client{Timeout: 2 * time.Second}
		r, err := c.Get("http://127.0.0.1:8085/healthz")
		if err != nil {
			os.Exit(1)
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	cfg, err := bootstrap.Read()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	app, close, err := bootstrap.Wire(ctx, cfg)
	cancel()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	defer close()
	srv := &http.Server{Addr: cfg.Address, Handler: httpapi.New(app, cfg.Origin, cfg.Secure, log), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 120 * time.Second, WriteTimeout: 150 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	log.Info("API listening", "address", cfg.Address)
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("API failed")
		os.Exit(1)
	}
}
