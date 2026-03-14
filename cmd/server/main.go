package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"syncvault/internal/app"
	"syncvault/internal/config"
)

func main() {
	cfg, err := config.LoadFromFile("internal/config/config.yml")
	if err != nil {
		log.Fatalf("Failed to load config.yml: %v", err)
	}

	application, err := app.New(app.WithConfig(cfg))
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := application.Run(ctx); err != nil {
			log.Printf("Application error: %v", err)
		}
	}()

	sig := <-sigChan
	log.Printf("Received signal: %v, starting graceful shutdown", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
		os.Exit(1)
	}

	wg.Wait()
	log.Println("Application shutdown completed")
}
