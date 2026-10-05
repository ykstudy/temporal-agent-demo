package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agentstudio/internal/agent"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	cfg := agent.EnvConfig()
	service, err := agent.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer service.Close()
	if cfg.Mode == "temporal" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		c, e := client.DialContext(ctx, client.Options{HostPort: cfg.TemporalAddress, Namespace: cfg.TemporalNamespace, Identity: service.RuntimeIdentity()})
		if e != nil {
			log.Fatalf("Temporal is required in temporal mode: %v", e)
		}
		service.Temporal = c
		w := worker.New(c, cfg.TaskQueue, worker.Options{Identity: service.RuntimeIdentity(), OnFatalError: func(error) { service.SetWorkerRunning(false) }})
		service.RegisterWorker(w)
		if e = w.Start(); e != nil {
			log.Fatal(e)
		}
		service.SetWorkerRunning(true)
		defer func() { service.SetWorkerRunning(false); w.Stop() }()
	}
	mux := http.NewServeMux()
	service.Register(mux)
	server := &http.Server{Addr: cfg.Address, Handler: service.RuntimeMiddleware(agent.CORS(mux)), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopping
		service.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Printf("Agent Studio API on %s; execution_mode=%s; provider=%s", cfg.Address, cfg.Mode, service.Provider())
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
