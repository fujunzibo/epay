package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"epay/internal/app"
)

func main() {
	cfg := app.LoadConfigFromEnv()
	serverApp, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer serverApp.Close()

	root := http.NewServeMux()
	adminDist := os.Getenv("ADMIN_STATIC_DIR")
	if adminDist == "" {
		adminDist = filepath.Join("admin", "dist")
	}
	if fi, err := os.Stat(adminDist); err == nil && fi.IsDir() {
		fs := http.FileServer(http.Dir(adminDist))
		root.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/", http.StatusFound)
		})
		root.Handle("/admin/", http.StripPrefix("/admin", fs))
		log.Printf("serving admin static from %s at /admin/", adminDist)
	}
	root.Handle("/", serverApp.Handler())

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("epay listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http serve failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown failed: %v", err)
	}
}
