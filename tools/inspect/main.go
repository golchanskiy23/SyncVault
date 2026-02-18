package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var (
		host     = flag.String("host", "localhost", "Database host")
		port     = flag.String("port", "5432", "Database port")
		user     = flag.String("user", "postgres", "Database user")
		password = flag.String("password", "postgres", "Database password")
		dbname   = flag.String("dbname", "syncvault", "Database name")
		table    = flag.String("table", "", "Specific table to inspect")
		limit    = flag.Int("limit", 10, "Number of rows to show")
		verbose  = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	ctx := context.Background()

	// Build connection string
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		*user, *password, *host, *port, *dbname,
	)

	// Create connection pool
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	log.Printf("Connected to database: %s:%s/%s", *host, *port, *dbname)

	// Show database overview
	if *table == "" {
		showDatabaseOverview(ctx, pool)
		showTableStats(ctx, pool)
		showRecentActivity(ctx, pool, *limit)
	} else {
		inspectTable(ctx, pool, *table, *limit, *verbose)
	}
}

// showDatabaseOverview shows general database information
func showDatabaseOverview(ctx context.Context, pool *pgxpool.Pool) {
	fmt.Println("\n=== DATABASE OVERVIEW ===")

	// Database version
	var version string
	pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	fmt.Printf("PostgreSQL Version: %s\n", strings.Split(version, ",")[0])

	// Database size
	var size string
	pool.QueryRow(ctx, "SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&size)
	fmt.Printf("Database Size: %s\n", size)

	// Connection stats
	stats := pool.Stat()
	fmt.Printf("Connections: %d/%d (max: %d)\n",
		stats.AcquiredConns(), stats.TotalConns(), stats.MaxConns())

	// Number of tables
	var tableCount int
	pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables 
		WHERE table_schema = 'public'
	`).Scan(&tableCount)
	fmt.Printf("Tables: %d\n", tableCount)
}

// showTableStats shows statistics for all tables
func showTableStats(ctx context.Context, pool *pgxpool.Pool) {
	fmt.Println("\n=== TABLE STATISTICS ===")

	query := `
		SELECT 
			table_name,
			pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size,
			n_tup_ins as inserts,
			n_tup_upd as updates,
			n_tup_del as deletes,
			n_live_tup as live_rows,
			last_vacuum,
			last_autovacuum
		FROM pg_stat_user_tables 
		ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		log.Printf("Failed to get table stats: %v", err)
		return
	}
	defer rows.Close()

	fmt.Printf("%-20s %-10s %-10s %-10s %-10s %-10s %-15s %-15s\n",
		"Table", "Size", "Inserts", "Updates", "Deletes", "Live Rows", "Last Vacuum", "Last AutoVacuum")
	fmt.Println(strings.Repeat("-", 100))

	for rows.Next() {
		var tableName, size, lastVacuum, lastAutoVacuum string
		var inserts, updates, deletes, liveRows int64

		err := rows.Scan(&tableName, &size, &inserts, &updates, &deletes, &liveRows, &lastVacuum, &lastAutoVacuum)
		if err != nil {
			log.Printf("Failed to scan table stats: %v", err)
			continue
		}

		fmt.Printf("%-20s %-10s %-10d %-10d %-10d %-10d %-15s %-15s\n",
			tableName, size, inserts, updates, deletes, liveRows, lastVacuum, lastAutoVacuum)
	}
}

// showRecentActivity shows recent database activity
func showRecentActivity(ctx context.Context, pool *pgxpool.Pool, limit int) {
	fmt.Println("\n=== RECENT ACTIVITY ===")

	// Recent files
	fmt.Printf("\n--- Recent Files (last %d) ---\n", limit)
	filesQuery := `
		SELECT id, user_id, file_name, file_size_bytes, created_at, updated_at
		FROM files 
		WHERE is_deleted = false
		ORDER BY updated_at DESC 
		LIMIT $1
	`

	rows, err := pool.Query(ctx, filesQuery, limit)
	if err == nil {
		defer rows.Close()

		fmt.Printf("%-8s %-8s %-30s %-12s %-20s %-20s\n",
			"ID", "User", "File Name", "Size", "Created", "Updated")
		fmt.Println(strings.Repeat("-", 100))

		for rows.Next() {
			var id, userID int64
			var fileName string
			var fileSize int64
			var createdAt, updatedAt time.Time

			if err := rows.Scan(&id, &userID, &fileName, &fileSize, &createdAt, &updatedAt); err == nil {
				fmt.Printf("%-8d %-8d %-30s %-12s %-20s %-20s\n",
					id, userID, fileName, formatBytes(fileSize),
					createdAt.Format("2006-01-02 15:04"),
					updatedAt.Format("2006-01-02 15:04"))
			}
		}
	}

	// Recent file versions
	fmt.Printf("\n--- Recent File Versions (last %d) ---\n", limit)
	versionsQuery := `
		SELECT fv.id, fv.file_id, fv.version_number, fv.file_size_bytes, fv.is_current, fv.created_at,
			   f.file_name
		FROM file_versions fv
		JOIN files f ON fv.file_id = f.id
		ORDER BY fv.created_at DESC 
		LIMIT $1
	`

	rows, err = pool.Query(ctx, versionsQuery, limit)
	if err == nil {
		defer rows.Close()

		fmt.Printf("%-8s %-8s %-10s %-12s %-8s %-20s %-30s\n",
			"ID", "File", "Version", "Size", "Current", "Created", "File Name")
		fmt.Println(strings.Repeat("-", 100))

		for rows.Next() {
			var id, fileID int64
			var versionNumber int
			var fileSize int64
			var isCurrent bool
			var createdAt time.Time
			var fileName string

			if err := rows.Scan(&id, &fileID, &versionNumber, &fileSize, &isCurrent, &createdAt, &fileName); err == nil {
				current := "No"
				if isCurrent {
					current = "Yes"
				}
				fmt.Printf("%-8d %-8d %-10d %-12s %-8s %-20s %-30s\n",
					id, fileID, versionNumber, formatBytes(fileSize),
					current, createdAt.Format("2006-01-02 15:04"), fileName)
			}
		}
	}

	// Recent sync events (if table exists)
	var tableExists bool
	pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'sync_events'
		)
	`).Scan(&tableExists)

	if tableExists {
		fmt.Printf("\n--- Recent Sync Events (last %d) ---\n", limit)
		eventsQuery := `
			SELECT id, job_id, event_type, status, file_size_bytes, created_at
			FROM sync_events
			ORDER BY created_at DESC 
			LIMIT $1
		`

		rows, err = pool.Query(ctx, eventsQuery, limit)
		if err == nil {
			defer rows.Close()

			fmt.Printf("%-8s %-8s %-12s %-10s %-12s %-20s\n",
				"ID", "Job", "Event Type", "Status", "Size", "Created")
			fmt.Println(strings.Repeat("-", 80))

			for rows.Next() {
				var id, jobID int64
				var eventType, status string
				var fileSize int64
				var createdAt time.Time

				if err := rows.Scan(&id, &jobID, &eventType, &status, &fileSize, &createdAt); err == nil {
					fmt.Printf("%-8d %-8d %-12s %-10s %-12s %-20s\n",
						id, jobID, eventType, status, formatBytes(fileSize),
						createdAt.Format("2006-01-02 15:04"))
				}
			}
		}
	}
}

// inspectTable shows detailed information about a specific table
func inspectTable(ctx context.Context, pool *pgxpool.Pool, tableName string, limit int, verbose bool) {
	fmt.Printf("\n=== TABLE: %s ===\n", strings.ToUpper(tableName))

	// Table structure
	fmt.Println("\n--- Table Structure ---")
	structureQuery := `
		SELECT 
			column_name,
			data_type,
			is_nullable,
			column_default
		FROM information_schema.columns 
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`

	rows, err := pool.Query(ctx, structureQuery, tableName)
	if err != nil {
		log.Printf("Failed to get table structure: %v", err)
		return
	}
	defer rows.Close()

	fmt.Printf("%-25s %-20s %-10s %-30s\n", "Column", "Type", "Nullable", "Default")
	fmt.Println(strings.Repeat("-", 90))

	for rows.Next() {
		var columnName, dataType, isNullable, columnDefault *string
		if err := rows.Scan(&columnName, &dataType, &isNullable, &columnDefault); err == nil {
			nullable := "NULL"
			if isNullable != nil && *isNullable == "NO" {
				nullable = "NOT NULL"
			}

			defaultVal := ""
			if columnDefault != nil {
				defaultVal = *columnDefault
			}

			fmt.Printf("%-25s %-20s %-10s %-30s\n",
				*columnName, *dataType, nullable, defaultVal)
		}
	}

	// Table data
	fmt.Printf("\n--- Sample Data (last %d rows) ---\n", limit)

	// Get primary key for ordering
	var pkColumn string
	pool.QueryRow(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass AND i.indisprimary
		LIMIT 1
	`, tableName).Scan(&pkColumn)

	orderBy := "id"
	if pkColumn != "" {
		orderBy = pkColumn
	}

	dataQuery := fmt.Sprintf("SELECT * FROM %s ORDER BY %s DESC LIMIT $1", tableName, orderBy)

	rows, err = pool.Query(ctx, dataQuery, limit)
	if err != nil {
		log.Printf("Failed to get table data: %v", err)
		return
	}
	defer rows.Close()

	// Get column names
	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		columns[i] = string(fd.Name)
	}

	// Print header
	if verbose {
		fmt.Println(strings.Join(columns, " | "))
		fmt.Println(strings.Repeat("-", len(strings.Join(columns, " | "))))
	}

	// Print data
	shownRows := 0
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}

		if verbose {
			// Print all columns
			stringValues := make([]string, len(values))
			for i, val := range values {
				if val == nil {
					stringValues[i] = "NULL"
				} else {
					stringValues[i] = fmt.Sprintf("%v", val)
				}
			}
			fmt.Println(strings.Join(stringValues, " | "))
		} else {
			// Print summary
			shownRows++
		}
	}

	if !verbose {
		fmt.Printf("Total rows shown: %d\n", shownRows)
	}

	// Table statistics
	fmt.Println("\n--- Table Statistics ---")
	statsQuery := fmt.Sprintf(`
		SELECT 
			pg_size_pretty(pg_total_relation_size($1::regclass)) as total_size,
			pg_size_pretty(pg_relation_size($1::regclass)) as table_size,
			pg_size_pretty(pg_total_relation_size($1::regclass) - pg_relation_size($1::regclass)) as index_size,
			(SELECT COUNT(*) FROM %s) as row_count
	`, tableName, tableName)

	var totalSize, tableSize, indexSize string
	var rowsCount int64

	err = pool.QueryRow(ctx, statsQuery).Scan(&totalSize, &tableSize, &indexSize, &rowsCount)
	if err == nil {
		fmt.Printf("Total Size: %s\n", totalSize)
		fmt.Printf("Table Size: %s\n", tableSize)
		fmt.Printf("Index Size: %s\n", indexSize)
		fmt.Printf("Row Count: %d\n", rowsCount)
	}

	// Indexes
	fmt.Println("\n--- Indexes ---")
	indexQuery := `
		SELECT 
			indexname,
			indexdef
		FROM pg_indexes 
		WHERE schemaname = 'public' AND tablename = $1
		ORDER BY indexname
	`

	rows, err = pool.Query(ctx, indexQuery, tableName)
	if err == nil {
		defer rows.Close()

		for rows.Next() {
			var indexName, indexDef string
			if err := rows.Scan(&indexName, &indexDef); err == nil {
				fmt.Printf("%s: %s\n", indexName, indexDef)
			}
		}
	}
}

// formatBytes formats bytes into human readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
