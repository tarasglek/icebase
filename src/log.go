package main

import (
	"database/sql"
	"fmt"

	"github.com/marcboeker/go-duckdb"
)


type Log struct {
	db *sql.DB
}

func NewLog() (*Log, error) {
	// Create new database connection
	db, err := InitializeDB()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database for logging: %w", err)
	}

	return &Log{
		db: db,
	}, nil
}

func (l *Log) createTable(rawCreateTable string) (int, error) {
	// Create schema_log table if it doesn't exist
	_, err := l.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_log (
			timestamp TIMESTAMP PRIMARY KEY,
			raw_query TEXT NOT NULL
		);
	`)
	if err != nil {
		return -1, fmt.Errorf("failed to create schema_log table: %w", err)
	}

	// Insert the raw query
	_, err = l.db.Exec(`
		INSERT INTO schema_log (timestamp, raw_query)
		VALUES (CURRENT_TIMESTAMP, ?);
	`, rawCreateTable)
	if err != nil {
		return -2, fmt.Errorf("failed to log table creation: %w", err)
	}

	return 0, nil
}

func (l *Log) Close() error {
	if l.db != nil {
		return l.db.Close()
	}
	return nil
}
