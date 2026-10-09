package main

import (
	"assethub/cmd/internal/bootstrap"
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const heartbeat = "/tmp/assethub-worker-heartbeat"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		info, err := os.Stat(heartbeat)
		if err != nil || time.Since(info.ModTime()) > 30*time.Second {
			os.Exit(1)
		}
		return
	}
	log := bootstrap.Logger()
	c, err := bootstrap.Read()
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, close, err := bootstrap.Wire(ctx, c)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	defer close()
	ingest := app.Ingestion()
	if !ingest.Enabled() {
		log.Error("worker requires an asset master key and configured storage provider")
		os.Exit(1)
	}
	var group sync.WaitGroup
	for n := 0; n < 2; n++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for ctx.Err() == nil {
				worked, e := ingest.ProcessNext(ctx)
				if e != nil {
					log.Error("worker queue or checkpoint unavailable")
				}
				if !worked || e != nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Second):
					}
				}
			}
		}()
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastCleanup := time.Time{}
	for ctx.Err() == nil {
		check, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = ingest.Dispatch(check)
		cancel()
		if err == nil {
			_ = os.WriteFile(heartbeat, []byte("alive"), 0600)
		} else {
			log.Error("outbox dispatch unavailable")
		}
		if time.Since(lastCleanup) > time.Minute {
			check, cancel := context.WithTimeout(ctx, 30*time.Second)
			if ingest.Cleanup(check) != nil {
				log.Error("staging cleanup failed")
			}
			cancel()
			lastCleanup = time.Now()
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
	group.Wait()
}
