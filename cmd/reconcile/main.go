package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"epay/internal/app"
)

func main() {
	cfg := app.LoadConfigFromEnv()
	svc, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer svc.Close()

	since := time.Now().Add(-24 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	report, err := svc.BuildReconcileSummary(ctx, since)
	if err != nil {
		log.Fatalf("build report failed: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		log.Fatalf("print report failed: %v", err)
	}
}
