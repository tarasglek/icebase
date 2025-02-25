package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	_ "github.com/marcboeker/go-duckdb"
	"github.com/rs/zerolog/log"
)

type ExtensionInfo struct {
	Extension    string `json:"extension"`
	ExtensionURL string `json:"extension_url"`
	Path         string `json:"path"`
}

// ProcessExtensions handles loading or downloading DuckDB extensions
func ProcessExtensions(db *sql.DB, install bool) error {
	extensions := []string{"httpfs", "s3", "delta"}
	freeBefore := uint64(0)
	if install {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %w", err)
		}
		extDir := filepath.Join(homeDir, ".duckdb", "extensions")
		if err := os.MkdirAll(extDir, 0755); err != nil {
			return fmt.Errorf("failed to create extension directory %s: %w", extDir, err)
		}
		freeBefore, err = GetFreeDiskSpace(extDir)
		if err != nil {
			return fmt.Errorf("failed to get free disk space before installing extensions: %w", err)
		}

		defer func() {
			freeAfter, err := GetFreeDiskSpace(extDir)
			if err != nil {
				log.Err(err).Msgf("failed to get free disk space after installing extensions")
			} else {
				diff := freeAfter - freeBefore
				log.Info().Strs("extensions", extensions).Msgf("Extension installed, free disk space changed by %d bytes", diff)
			}
		}()
	}

	for _, ext := range extensions {
		if install {
			if _, err := db.Exec(fmt.Sprintf("INSTALL %s;", ext)); err != nil {
				return fmt.Errorf("failed to install extension %s: %w", ext, err)
			}
		}
		if _, err := db.Exec(fmt.Sprintf("LOAD %s;", ext)); err != nil {
			return fmt.Errorf("failed to load extension %s: %w", ext, err)

		}
	}
	log.Info().Msg("DuckDB extensions loaded successfully")
	return nil
}

//go:embed duckdb-uuidv7/uuidv7.sql
var uuid_v7_macro string

//go:embed delta_stats.sql
var delta_stats string

// loadMacros loads all required DuckDB macros
func loadMacros(db *sql.DB) error {
	// Load uuid_v7_macro
	if _, err := db.Exec(uuid_v7_macro); err != nil {
		return fmt.Errorf("failed to load UUIDv7 macro: %w", err)
	}
	// Load delta_stats
	if _, err := db.Exec(delta_stats); err != nil {
		return fmt.Errorf("failed to load delta_stats macro: %w", err)
	}
	return nil
}

func GetFreeDiskSpace(dir string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

// InitializeDuckDB loads JSON extension and registers UUIDv7 UDFs
func InitializeDuckDB() (*sql.DB, error) {
	// Ensure ~/.duckdb/extensions directory exists
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	freeDisk, err := GetFreeDiskSpace(homeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk space for %s: %w", homeDir, err)
	}
	log.Info().Msgf("Available disk space in %s: %d bytes", homeDir, freeDisk)

	// Open database connection
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Load JSON extension
	if _, err := db.Exec("LOAD json;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load JSON extension: %w", err)
	}

	// Load all macros
	if err := loadMacros(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// ResetMemoryDB resets the in-memory database state
// by attaching a new memory database and detaching all others
func ResetMemoryDB(db *sql.DB) error {
	// Get database name from character sets
	var dbName string
	if err := db.QueryRow(`
			SELECT default_collate_catalog
			FROM information_schema.character_sets
			LIMIT 1;
	`).Scan(&dbName); err != nil {
		return fmt.Errorf("failed to get database name: %w", err)
	}

	// now run ATTACH 'memory' as <dbName_tmp>; USE <dbName_tmp>; detach <dbName>; ATTACH 'memory' as <dbName>; USE <dbName>;
	_, err := db.Exec(fmt.Sprintf(
		`ATTACH ':memory:' AS %s_tmp; 
		 USE %s_tmp; 
		 DETACH %s; 
		 ATTACH ':memory:' AS %s; 
		 USE %s`,
		dbName, // %s_tmp
		dbName, // %s_tmp
		dbName, // %s
		dbName, // %s
		dbName, // %s
	))

	if err != nil {
		return fmt.Errorf("failed to reset memory database: %w", err)
	}

	rows, err := db.Query("SELECT name FROM pragma_database_list WHERE name != ?", dbName)
	if err != nil {
		return fmt.Errorf("failed to list databases: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("failed to scan database name: %w", err)
		}
		log.Debug().Msgf("DETACH %s", name)
		if _, err := db.Exec("DETACH " + name); err != nil {
			return fmt.Errorf("failed to detach %s: %w", name, err)
		}
	}

	// Load all macros
	if err := loadMacros(db); err != nil {
		return err
	}

	return nil
}
