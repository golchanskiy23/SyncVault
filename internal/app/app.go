package app

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

type App struct {
	httpServer *http.Server
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	shutdown   bool
	mu         sync.RWMutex
}

func New() *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Добавить файл конфигурации из которого получать данные для сервера и т.п.
// Добавить паттерн функциональных опций
func (a *App) Run(ctx context.Context) error {
	log.Println("Starting application...")

	a.httpServer = &http.Server{
		Addr:         ":8080",
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	a.wg.Add(1)
	// Правильно ли запускать гоутину в горутине и какие поблемы это может вызывать?
	go func() {
		defer a.wg.Done()
		log.Printf("HTTP server starting on %s", a.httpServer.Addr)

		go func() {
			<-a.ctx.Done()
			log.Println("Application context cancelled, shutting down HTTP server...")

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
				log.Printf("HTTP server shutdown error: %v", err)
			}
		}()

		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// a.startBackgroundServices()

	log.Println("Application started successfully")

	<-a.ctx.Done()
	log.Println("Application context cancelled, exiting Run()")

	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	if a.shutdown {
		a.mu.Unlock()
		return nil
	}
	a.shutdown = true
	a.mu.Unlock()

	log.Println("Starting application shutdown...")

	a.cancel()

	if a.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		log.Println("Shutting down HTTP server...")
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}

	//a.shutdownBackgroundServices(ctx)

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All background services shutdown completed")
	case <-ctx.Done():
		log.Println("Shutdown timeout reached, forcing exit")
		return ctx.Err()
	}

	log.Println("Application shutdown completed")
	return nil
}

/*
func (a *App) startBackgroundServices() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-a.ctx.Done():
				log.Println("Background service: context cancelled, stopping...")
				return
			case <-ticker.C:
				log.Println("Background service: performing periodic task")
			}
		}
	}()

	log.Println("Background services started")
}


func (a *App) shutdownBackgroundServices(ctx context.Context) {
	log.Println("Shutting down background services...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Second)
		close(done)
	}()

	select {
	case <-done:
		log.Println("Background services shutdown completed")
	case <-shutdownCtx.Done():
		log.Println("Background services shutdown timeout")
	}
}*/
