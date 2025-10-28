package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // For database/sql compatibility
)

// Database holds the database connection pool and all repositories
type Database struct {
	Pool               *pgxpool.Pool
	FileRepo           *FileRepository
	FileVersionRepo    *FileVersionRepository
	DirectoryRepo      *DirectoryRepository
	BatchRepo          *BatchRepository
	QueryAnalyzer      *QueryAnalyzer
}

// Config holds database configuration
type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	HealthCheckPeriod time.Duration
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Host:            "localhost",
		Port:            "5432",
		User:            "postgres",
		Password:        "postgres",
		DBName:          "syncvault",
		SSLMode:         "disable",
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: time.Minute * 30,
		HealthCheckPeriod: time.Minute,
	}
}

// NewDatabase creates a new database connection and initializes all repositories
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

	// Initialize repositories
	db := &Database{
		Pool: pool,
	}

	// Initialize all repositories
	db.FileRepo = NewFileRepository(pool)
	db.FileVersionRepo = NewFileVersionRepository(pool)
	db.DirectoryRepo = NewDirectoryRepository(pool)
	db.BatchRepo = NewBatchRepository(pool)
	db.QueryAnalyzer = NewQueryAnalyzer(pool)

	log.Printf("Database connection established successfully")
	return db, nil
}

// Close closes the database connection pool
func (db *Database) Close() {
	if db.Pool != nil {
		db.Pool.Close()
		log.Printf("Database connection pool closed")
	}
}

// HealthCheck performs a health check on the database
func (db *Database) HealthCheck(ctx context.Context) error {
	if db.Pool == nil {
		return fmt.Errorf("database pool is nil")
	}

	// Test basic connectivity
	if err := db.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Test basic query
	var result int
	err := db.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("basic query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: got %d, expected 1", result)
	}

	return nil
}

// GetStats returns database connection pool statistics
func (db *Database) GetStats() *pgxpool.Stat {
	if db.Pool == nil {
		return nil
	}
	
	stats := db.Pool.Stat()
	return &stats
}

// BeginTransaction starts a new database transaction
func (db *Database) BeginTransaction(ctx context.Context) (pgxpool.Tx, error) {
	return db.Pool.Begin(ctx)
}

// ExecuteInTransaction executes a function within a database transaction
func (db *Database) ExecuteInTransaction(ctx context.Context, fn func(pgxpool.Tx) error) error {
	tx, err := db.BeginTransaction(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return fmt.Errorf("transaction function failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Migrate runs database migrations
func (db *Database) Migrate(ctx context.Context, migrationsPath string) error {
	// This would integrate with the migration tool
	// For now, just check if basic tables exist
	query := `
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		AND table_name = 'files'
	`

	var count int
	err := db.Pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("database not migrated - files table not found")
	}

	log.Printf("Database migration check passed")
	return nil
}

// ValidateSchema validates the database schema
func (db *Database) ValidateSchema(ctx context.Context) error {
	// Check required tables
	requiredTables := []string{
		"users", "sessions", "files", "file_versions", 
		"storage_nodes", "sync_jobs", "sync_events",
	}

	for _, table := range requiredTables {
		query := `
			SELECT COUNT(*) 
			FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		`

		var count int
		err := db.Pool.QueryRow(ctx, query, table).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check table %s: %w", table, err)
		}

		if count == 0 {
			return fmt.Errorf("required table %s not found", table)
		}
	}

	// Check indexes
	requiredIndexes := []string{
		"files_user_id_idx", "files_file_path_idx", "files_checksum_sha256_idx",
		"file_versions_file_id_idx", "file_versions_checksum_sha256_idx",
		"sync_events_job_id_idx", "sync_events_created_at_idx",
	}

	for _, index := range requiredIndexes {
		query := `
			SELECT COUNT(*) 
			FROM pg_indexes 
			WHERE schemaname = 'public' 
			AND indexname = $1
		`

		var count int
		err := db.Pool.QueryRow(ctx, query, index).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check index %s: %w", index, err)
		}

		if count == 0 {
			log.Printf("Warning: index %s not found", index)
		}
	}

	log.Printf("Database schema validation passed")
	return nil
}
