package app

import (
	"fmt"
	"net/http"
	"time"

	"syncvault/internal/config"
)

type Option func(*App) error

func WithConfig(cfg *config.Config) Option {
	return func(a *App) error {
		if cfg == nil {
			return fmt.Errorf("config cannot be nil")
		}
		a.config = cfg
		return nil
	}
}

func WithHTTPServer(server *http.Server) Option {
	return func(a *App) error {
		if server == nil {
			return fmt.Errorf("http server cannot be nil")
		}
		a.httpServer = server
		return nil
	}
}

func WithHTTPPort(port int) Option {
	return func(a *App) error {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid port: %d", port)
		}
		a.config.HTTP.Port = port
		return nil
	}
}

func WithHTTPHost(host string) Option {
	return func(a *App) error {
		a.config.HTTP.Host = host
		return nil
	}
}

func WithShutdownTimeout(timeout interface{}) Option {
	return func(a *App) error {
		switch v := timeout.(type) {
		case int:
			a.config.Shutdown.Timeout = time.Duration(v) * time.Second
		case time.Duration:
			a.config.Shutdown.Timeout = v
		default:
			return fmt.Errorf("invalid timeout type, expected int or time.Duration")
		}
		return nil
	}
}
