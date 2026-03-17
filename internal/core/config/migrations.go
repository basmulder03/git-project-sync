package config

import (
	"fmt"
	"sort"
)

// Migration represents a single migration operation
type Migration struct {
	// Version is a unique identifier for this migration (e.g., "20240101_move_database")
	Version string

	// Description provides a human-readable description of what this migration does
	Description string

	// Apply executes the migration
	Apply func(cfg *Config) error
}

// MigrationRegistry manages all registered migrations
type MigrationRegistry struct {
	migrations []Migration
}

// NewMigrationRegistry creates a new migration registry
func NewMigrationRegistry() *MigrationRegistry {
	return &MigrationRegistry{
		migrations: []Migration{},
	}
}

// Register adds a migration to the registry
func (r *MigrationRegistry) Register(m Migration) {
	r.migrations = append(r.migrations, m)
}

// RunAll executes all registered migrations in order
func (r *MigrationRegistry) RunAll(cfg *Config) error {
	// Sort migrations by version to ensure consistent ordering
	sort.Slice(r.migrations, func(i, j int) bool {
		return r.migrations[i].Version < r.migrations[j].Version
	})

	for _, migration := range r.migrations {
		if err := migration.Apply(cfg); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration.Version, err)
		}
	}

	return nil
}

// DefaultMigrations returns the default migration registry with all built-in migrations
func DefaultMigrations() *MigrationRegistry {
	registry := NewMigrationRegistry()

	// Register migration: Move state database to AppData
	registry.Register(Migration{
		Version:     "20260306_move_state_database",
		Description: "Move state database from workspace root to user data directory",
		Apply:       migrateStateDatabaseToAppData,
	})

	// Register migration: Strip embedded PAT tokens from git remote origins
	registry.Register(Migration{
		Version:     "20260317_strip_pat_from_origins",
		Description: "Remove embedded PAT tokens from git remote origin URLs in all workspace repos",
		Apply:       migrateStripPATFromOrigins,
	})

	return registry
}

// RunMigrations runs all default migrations on the given config
func RunMigrations(cfg *Config) error {
	registry := DefaultMigrations()
	return registry.RunAll(cfg)
}
