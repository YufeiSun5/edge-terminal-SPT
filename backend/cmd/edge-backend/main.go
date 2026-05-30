package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/runtime"
)

func main() {
	cfgPath := os.Getenv("EDGE_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/config.json"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("connect database failed: %v", err)
	}

	if cfg.Database.AutoMigrate {
		if err := database.AutoMigrate(db); err != nil {
			log.Fatalf("auto migrate failed: %v", err)
		}
	}

	kernel := runtime.NewKernel(cfg, db)
	if err := kernel.Start(); err != nil {
		log.Fatalf("start backend kernel failed: %v", err)
	}

	server := &http.Server{
		Addr:              cfg.App.HTTPAddr,
		Handler:           kernel.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("edge backend listening on %s", cfg.App.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("http shutdown failed: %v", err)
	}
	kernel.Stop()
	log.Println("edge backend stopped")
}
