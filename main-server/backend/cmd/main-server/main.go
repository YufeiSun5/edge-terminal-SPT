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

	router := server.NewRouter(cfg, db)
	httpServer := &http.Server{
		Addr:              cfg.App.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("http shutdown failed: %v", err)
	}
	log.Println("main server backend stopped")
}
