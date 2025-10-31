package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // For database/sql compatibility
)

// Database holds database connection pool
type Database struct {
	Pool *pgxpool.Pool
}

// Config holds database configuration
type Config struct {
	Host              string
	Port              string
	User              string
	Password          string
	DBName            string
	SSLMode           string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Host:              "localhost",
		Port:              "5432",
		User:              "postgres",
		Password:          "postgres",
		DBName:            "syncvault",
		SSLMode:           "disable",
		MaxConns:          25,
		MinConns:          5,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   time.Minute * 30,
		HealthCheckPeriod: time.Minute,
	}
}

// NewDatabase creates a new database connection
func NewDatabase(ctx context.Context, config *Config) (*Database, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Build connection string
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		config.User,
		config.Password,
		config.Host,
		config.Port,
		config.DBName,
		config.SSLMode,
	)

	// Configure connection pool
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Set pool configuration
	poolConfig.MaxConns = config.MaxConns
	poolConfig.MinConns = config.MinConns
	poolConfig.MaxConnLifetime = config.MaxConnLifetime
	poolConfig.MaxConnIdleTime = config.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = config.HealthCheckPeriod

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Initialize database
	db := &Database{
		Pool: pool,
	}

	log.Printf("Database connection established successfully")
	return db, nil
}

// Close closes database connection pool
func (db *Database) Close() {
	if db.Pool != nil {
		db.Pool.Close()
		log.Printf("Database connection pool closed")
	}
}

// HealthCheck performs a health check on the database
func (db *Database) HealthCheck(ctx context.Context) error {
	var result int
	return db.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
}

// GetStats returns database statistics
func (db *Database) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	
	if db.Pool != nil {
		poolStats := db.Pool.Stat()
		stats["acquired_connections"] = poolStats.AcquiredConns()
		stats["total_connections"] = poolStats.TotalConns()
		stats["idle_connections"] = poolStats.IdleConns()
		stats["max_connections"] = poolStats.MaxConns()
	}
	
	return stats
}

// BeginTransaction starts a new database transaction
func (db *Database) BeginTransaction(ctx context.Context) (pgx.Tx, error) {
	return db.Pool.Begin(ctx)
}

// ExecuteInTransaction executes a function within a database transaction
func (db *Database) ExecuteInTransaction(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.BeginTransaction(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	defer tx.Rollback(ctx)
	
	if err := fn(tx); err != nil {
		return err
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}
