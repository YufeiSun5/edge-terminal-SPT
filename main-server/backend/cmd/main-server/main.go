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

	"spindle-main-server/backend/internal/config"
	"spindle-main-server/backend/internal/database"
	"spindle-main-server/backend/internal/query"
	"spindle-main-server/backend/internal/reports"
	"spindle-main-server/backend/internal/server"
)

func main() {
	cfgPath := os.Getenv("MAIN_SERVER_CONFIG")
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
	if err := query.EnsureMainServerSchema(db); err != nil {
		log.Fatalf("ensure main server query schema failed: %v", err)
	}
	reportService := reports.NewService(db, query.NewStationViewQuery(db), reports.Options{
		ArtifactDir:            cfg.Reports.ArtifactDir,
		DefaultTemplateCode:    cfg.Reports.DefaultTemplateCode,
		DefaultTemplateVersion: cfg.Reports.DefaultTemplateVersion,
		DefaultTemplateFileRef: cfg.Reports.DefaultTemplateFileRef,
		MaxAttempts:            cfg.Reports.MaxAttempts,
	})
	if err := reportService.EnsureSchema(); err != nil {
		log.Fatalf("ensure main report job schema failed: %v", err)
	}

	router := server.NewRouter(cfg, db)
	httpServer := &http.Server{
		Addr:              cfg.App.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	if cfg.Reports.IsWorkerEnabled() {
		reports.StartWorker(ctx, reportService, time.Duration(cfg.Reports.WorkerIntervalSeconds)*time.Second)
	}

	go func() {
		log.Printf("main server backend listening on %s", cfg.App.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown failed: %v", err)
	}
	log.Println("main server backend stopped")
}
