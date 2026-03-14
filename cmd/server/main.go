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
)

func main() {
	// Добавить обработку возможных ошибок при создании приложения
	application := app.New()

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

	// Разве здесь не нужно выходить из main, при ошибке?
	if err := application.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	wg.Wait()
	log.Println("Application shutdown completed")
}
