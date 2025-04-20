package migration

import (
	"fmt"
	"os"

	// register drivers
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/golang-migrate/migrate/v4"
	"github.com/zeelrupapara/custom-ai-server/pkg/config"
)

// Up applies all pending “up” migrations from the local migrations/ folder.
func Up() error {
	cfg := config.Load()

	// migrations/ must be at the working directory root
	sourceURL := "file://migrations"
	dbURL := cfg.DBUrl

	m, err := migrate.New(sourceURL, dbURL)
	if err != nil {
		return fmt.Errorf("migration.New: %w", err)
	}
	// Apply all up migrations; ErrNoChange means nothing new
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("m.Up: %w", err)
	}
	return nil
}

// Down rolls back the most recent migration.
func Down() error {
	cfg := config.Load()

	sourceURL := "file://migrations"
	dbURL := cfg.DBUrl

	m, err := migrate.New(sourceURL, dbURL)
	if err != nil {
		return fmt.Errorf("migration.New: %w", err)
	}
	// Steps(-1) undoes one migration
	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("m.Steps(-1): %w", err)
	}
	return nil
}

// Force forces the migration version (useful after a failed migration)
func Force(version int) error {
	cfg := config.Load()

	sourceURL := "file://migrations"
	dbURL := cfg.DBUrl

	m, err := migrate.New(sourceURL, dbURL)
	if err != nil {
		return fmt.Errorf("migration.New: %w", err)
	}
	if err := m.Force(version); err != nil {
		return fmt.Errorf("m.Force(%d): %w", version, err)
	}
	return nil
}

func init() {
	// Verify that the migrations directory exists at startup
	if _, err := os.Stat("migrations"); os.IsNotExist(err) {
		panic("migrations directory not found; please create migrations/*.up.sql files")
	}
}
