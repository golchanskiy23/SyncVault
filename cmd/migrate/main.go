package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	DatabaseURL    string
	MigrationsPath string
	Steps          int
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/syncvault?sslmode=disable"
	}

	config := Config{
		DatabaseURL:    dbURL,
		MigrationsPath: "file://../../migrations",
		Steps:          0,
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "up":
			runMigrationsUp(config)
		case "down":
			runMigrationsDown(config)
		case "force":
			if len(os.Args) < 3 {
				log.Fatal("Force command requires version argument")
			}
			forceVersion(config, os.Args[2])
		case "version":
			getCurrentVersion(config)
		case "create":
			if len(os.Args) < 3 {
				log.Fatal("Create command requires migration name argument")
			}
			createMigration(config, os.Args[2])
		case "status":
			getMigrationStatus(config)
		default:
			printUsage()
		}
	} else {
		runMigrationsUp(config)
	}
}

func runMigrationsUp(config Config) {
	log.Println("Running database migrations...")

	m, err := migrate.New(
		config.MigrationsPath,
		config.DatabaseURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	currentVersion, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		log.Printf("Warning: Could not get current version: %v", err)
	} else {
		if dirty {
			log.Printf("Current version: %d (dirty)", currentVersion)
		} else if err != migrate.ErrNilVersion {
			log.Printf("Current version: %d", currentVersion)
		}
	}

	start := time.Now()
	var steps int
	if config.Steps > 0 {
		steps = config.Steps
		err = m.Steps(steps)
	} else {
		err = m.Up()
	}
	duration := time.Since(start)

	if err != nil {
		if err == migrate.ErrNoChange {
			log.Println("No migrations to run")
			return
		}
		log.Fatalf("Migration failed: %v", err)
	}

	newVersion, dirty, err := m.Version()
	if err != nil {
		log.Printf("Warning: Could not get new version: %v", err)
	} else {
		if dirty {
			log.Printf("New version: %d (dirty) - WARNING: Database is in dirty state!", newVersion)
		} else {
			log.Printf("Successfully migrated to version %d in %v", newVersion, duration)
		}
	}

	if err := verifyConnection(config.DatabaseURL); err != nil {
		log.Printf("Warning: Database verification failed: %v", err)
	} else {
		log.Println("Database connection verified successfully")
	}
}

func runMigrationsDown(config Config) {
	log.Println("Rolling back database migrations...")

	m, err := migrate.New(
		config.MigrationsPath,
		config.DatabaseURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	currentVersion, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			log.Println("No migrations to rollback")
			return
		}
		log.Fatalf("Could not get current version: %v", err)
	}

	if dirty {
		log.Printf("Current version: %d (dirty) - WARNING: Database is in dirty state!", currentVersion)
	} else {
		log.Printf("Current version: %d", currentVersion)
	}

	start := time.Now()
	var steps int
	if config.Steps > 0 {
		steps = config.Steps
		err = m.Steps(-steps)
	} else {
		err = m.Down()
	}
	duration := time.Since(start)

	if err != nil {
		if err == migrate.ErrNoChange {
			log.Println("No migrations to rollback")
			return
		}
		log.Fatalf("Rollback failed: %v", err)
	}

	newVersion, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		log.Printf("Warning: Could not get new version: %v", err)
	} else if err != migrate.ErrNilVersion {
		if dirty {
			log.Printf("New version: %d (dirty) - WARNING: Database is in dirty state!", newVersion)
		} else {
			log.Printf("Successfully rolled back to version %d in %v", newVersion, duration)
		}
	} else {
		log.Println("Successfully rolled back all migrations")
	}
}

func forceVersion(config Config, versionStr string) {
	log.Printf("Forcing migration version to %s...", versionStr)

	m, err := migrate.New(
		config.MigrationsPath,
		config.DatabaseURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	var version uint
	_, err = fmt.Sscanf(versionStr, "%d", &version)
	if err != nil {
		log.Fatalf("Invalid version format: %v", err)
	}

	err = m.Force(int(version))
	if err != nil {
		log.Fatalf("Failed to force version: %v", err)
	}

	log.Printf("Successfully forced version to %d", version)
}

func getCurrentVersion(config Config) {
	m, err := migrate.New(
		config.MigrationsPath,
		config.DatabaseURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	version, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			log.Println("No migrations applied")
		} else {
			log.Fatalf("Could not get version: %v", err)
		}
	} else {
		if dirty {
			log.Printf("Current version: %d (dirty) - WARNING: Database is in dirty state!", version)
		} else {
			log.Printf("Current version: %d", version)
		}
	}
}

func getMigrationStatus(config Config) {
	m, err := migrate.New(
		config.MigrationsPath,
		config.DatabaseURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	currentVersion, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		log.Fatalf("Could not get version: %v", err)
	}

	log.Println("Migration Status:")
	log.Println("=================")

	if err == migrate.ErrNilVersion {
		log.Println("No migrations applied")
	} else {
		status := "Applied"
		if dirty {
			status = "Applied (DIRTY)"
		}
		log.Printf("Version %d: %s", currentVersion, status)
	}

	log.Println("\nNote: Use 'migrate version' to see current version")
	log.Println("Use 'migrate up' to apply pending migrations")
	log.Println("Use 'migrate down' to rollback migrations")
}

func createMigration(config Config, name string) {
	m, err := migrate.New(
		config.MigrationsPath,
		config.DatabaseURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	version, _, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		log.Fatalf("Could not get current version: %v", err)
	}

	nextVersion := version + 1
	versionStr := fmt.Sprintf("%04d", nextVersion)

	upFile := fmt.Sprintf("../../migrations/%s_%s.up.sql", versionStr, name)
	downFile := fmt.Sprintf("../../migrations/%s_%s.down.sql", versionStr, name)

	upContent := fmt.Sprintf(`-- %s_%s.up.sql
-- Add your migration SQL here

-- Example:
-- ALTER TABLE users ADD COLUMN new_field VARCHAR(100);
-- CREATE INDEX idx_users_new_field ON users (new_field);
`, versionStr, name)

	err = os.WriteFile(upFile, []byte(upContent), 0644)
	if err != nil {
		log.Fatalf("Failed to create up migration file: %v", err)
	}

	downContent := fmt.Sprintf(`-- %s_%s.down.sql
-- Rollback your migration SQL here

-- Example:
-- DROP INDEX IF EXISTS idx_users_new_field;
-- ALTER TABLE users DROP COLUMN IF EXISTS new_field;
`, versionStr, name)

	err = os.WriteFile(downFile, []byte(downContent), 0644)
	if err != nil {
		log.Fatalf("Failed to create down migration file: %v", err)
	}

	log.Printf("Created migration files:")
	log.Printf("  %s", upFile)
	log.Printf("  %s", downFile)
}

func verifyConnection(dbURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	var result int
	err = pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("database query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: %d", result)
	}

	tables := []string{"users", "files", "sync_jobs", "sync_events"}
	for _, table := range tables {
		var exists bool
		err = pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)",
			table).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check table %s: %w", table, err)
		}
		if !exists {
			log.Printf("Warning: Table %s does not exist", table)
		}
	}

	return nil
}

func printUsage() {
	fmt.Println("SyncVault Migration Tool")
	fmt.Println("========================")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run ./cmd/migrate [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  up                    Run all pending migrations (default)")
	fmt.Println("  down                  Rollback all migrations")
	fmt.Println("  force <version>       Force database to specific version")
	fmt.Println("  version               Show current migration version")
	fmt.Println("  status                Show migration status")
	fmt.Println("  create <name>          Create new migration files")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  DATABASE_URL          PostgreSQL connection URL")
	fmt.Println("                        (default: postgres://localhost/syncvault?sslmode=disable)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run ./cmd/migrate up")
	fmt.Println("  go run ./cmd/migrate down")
	fmt.Println("  go run ./cmd/migrate force 3")
	fmt.Println("  go run ./cmd/migrate create add_user_preferences")
	fmt.Println("  DATABASE_URL=postgres://user:pass@host/db go run ./cmd/migrate up")
}
