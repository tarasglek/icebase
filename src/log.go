package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type Log struct {
	db        *sql.DB
	tableName string
}

func NewLog(tableName string) *Log {
	return &Log{
		tableName: tableName,
	}
}

func (l *Log) getDB() (*sql.DB, error) {
	if l.db != nil {
		return l.db, nil
	}

	// Create storage directory structure
	logDir := filepath.Join("storage", l.tableName, "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create database path
	dbPath := filepath.Join(logDir, "log.db")

	// Initialize main database connection
	db, err := InitializeDuckDB()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Attach log database
	_, err = db.Exec(fmt.Sprintf(`
		ATTACH DATABASE '%s' AS log_db;
		USE log_db;
	`, dbPath))
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to attach log database: %w", err)
	}

	// Create schema if needed
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_log (
			timestamp TIMESTAMP PRIMARY KEY,
			raw_query TEXT NOT NULL
		);
		
		CREATE TABLE IF NOT EXISTS insert_log (
			id UUID PRIMARY KEY,
			partition TEXT NOT NULL DEFAULT '',
			tombstoned_unix_time BIGINT NOT NULL DEFAULT 0,
			size BIGINT NOT NULL DEFAULT 0
		);
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema_log table: %w", err)
	}

	l.db = db
	return l.db, nil
}

func (l *Log) createTable(rawCreateTable string) (int, error) {
	db, err := l.getDB()
	if err != nil {
		return -1, err
	}

	// Insert the raw query
	_, err = db.Exec(`
		INSERT INTO schema_log (timestamp, raw_query)
		VALUES (CURRENT_TIMESTAMP, ?);
	`, rawCreateTable)
	if err != nil {
		return -1, fmt.Errorf("failed to log table creation: %w", err)
	}

	return 0, nil
}

func (l *Log) Close() error {
	if l.db != nil {
		return l.db.Close()
	}
	return nil
}

func (l *Log) RecreateSchema(tx *sql.Tx) error {
	db, err := l.getDB()
	if err != nil {
		return fmt.Errorf("failed to get log database: %w", err)
	}

	// Query schema_log for all create table statements
	rows, err := db.Query(`
		SELECT raw_query 
		FROM schema_log
		ORDER BY timestamp ASC
	`)
	if err != nil {
		return fmt.Errorf("failed to query schema_log: %w", err)
	}
	defer rows.Close()

	// Execute each create table statement in the transaction
	for rows.Next() {
		var createQuery string
		if err := rows.Scan(&createQuery); err != nil {
			return fmt.Errorf("failed to scan schema_log row: %w", err)
		}

		// Execute the create table statement
		if _, err := tx.Exec(createQuery); err != nil {
			return fmt.Errorf("failed to execute schema_log query: %w", err)
		}
	}

	return nil
}

func (l *Log) Insert(tx *sql.Tx, table string, query string) (int, error) {
	// First insert into insert_log to generate UUID
	db, err := l.getDB()
	if err != nil {
		return -1, fmt.Errorf("failed to get log database: %w", err)
	}

	// Insert and get UUID using RETURNING
	var uuidBytes []byte
	err = db.QueryRow(`
		INSERT INTO insert_log (id, partition)
		VALUES (uuidv7(), '')
		RETURNING id;
	`).Scan(&uuidBytes)
	if err != nil {
		return -1, fmt.Errorf("failed to insert into insert_log: %w", err)
	}

	// Convert UUID bytes to string for filename
	uuidStr := uuid.UUID(uuidBytes).String()
	log.Printf("Generated UUIDv7: %s", uuidStr)

	// Create storage directory structure
	dataDir := filepath.Join("storage", table, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return -1, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create parquet file path using UUID from insert_log
	parquetPath := filepath.Join(dataDir, uuidStr+".parquet")
	log.Printf("Parquet file path: %s", parquetPath)

	// Copy table data to parquet file
	copyQuery := fmt.Sprintf(`
		COPY %s TO '%s' (FORMAT PARQUET);
	`, table, parquetPath)

	// Execute COPY TO PARQUET using the transaction
	_, err = tx.Exec(copyQuery)
	if err != nil {
		return -1, fmt.Errorf("failed to copy to parquet with query: %q: %w", copyQuery, err)
	}

	// Get file size
	fileInfo, err := os.Stat(parquetPath)
	if err != nil {
		return -1, fmt.Errorf("failed to get parquet file size: %w", err)
	}

	// Update size in insert_log
	_, err = db.Exec(`
		UPDATE insert_log 
		SET size = ?
		WHERE id = ?;
	`, fileInfo.Size(), uuidStr)
	if err != nil {
		return -1, fmt.Errorf("failed to update insert_log size: %w", err)
	}

	return 0, nil
}

// Recreates the table described in the schema_log table as a view over partitioned parquet files
func (l *Log) RecreateAsView(tx *sql.Tx) error {
	// filels = list of files with tombstone 0 ordered by id desc
	db, err := l.getDB()
	if err != nil {
		return fmt.Errorf("failed to get log database: %w", err)
	}

	// Query schema_log for all create table statements
	rows, err := db.Query(`
		SELECT raw_query
		FROM schema_log
		ORDER BY timestamp ASC
	`)
	if err != nil {
		return fmt.Errorf("failed to query schema_log: %w", err)
	}
	defer rows.Close()

	// Execute each create table statement in the transaction
	for rows.Next() {
		var createQuery string
		if err := rows.Scan(&createQuery); err != nil {
			return fmt.Errorf("failed to scan schema_log row: %w", err)
		}
		// Replace CREATE TABLE with CREATE VIEW and append files to view view read_parquet(filels);
		// log query
		// Execute the create table statement
		if _, err := tx.Exec(createQuery); err != nil {
			return fmt.Errorf("failed to execute schema_log query: %w", err)
		}
		break
	}

	return nil
}
