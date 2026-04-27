package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"epay/internal/app"
)

func main() {
	cfg := app.LoadConfigFromEnv()
	retryInterval := 30 * time.Second
	if raw := os.Getenv("RETRY_INTERVAL_SECONDS"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			retryInterval = time.Duration(v) * time.Second
		}
	}

	svc, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer svc.Close()

	log.Printf("retry worker started (interval=%s)", retryInterval.String())
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		retried, failed, err := svc.RetryUncreditedOrders(ctx, 100)
		synced, syncFailed, syncErr := svc.RetryOneAPICredits(ctx, 100)
		cancel()
		if err != nil {
			log.Printf("retry iteration failed: %v", err)
		} else if retried > 0 || failed > 0 {
			log.Printf("retry iteration finished retried=%d failed=%d", retried, failed)
		}
		if syncErr != nil {
			log.Printf("oneapi sync iteration failed: %v", syncErr)
		} else if synced > 0 || syncFailed > 0 {
			log.Printf("oneapi sync iteration finished synced=%d failed=%d", synced, syncFailed)
		}
		<-ticker.C
	}
}
