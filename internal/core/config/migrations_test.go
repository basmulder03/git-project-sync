package config

import (
	"errors"
	"testing"
)

func TestMigrationRegistry_Register(t *testing.T) {
	registry := NewMigrationRegistry()

	migration := Migration{
		Version:     "test_v1",
		Description: "Test migration",
		Apply:       func(cfg *Config) error { return nil },
	}

	registry.Register(migration)

	if len(registry.migrations) != 1 {
		t.Errorf("expected 1 migration, got %d", len(registry.migrations))
	}
}

func TestMigrationRegistry_RunAll_ExecutesInOrder(t *testing.T) {
	registry := NewMigrationRegistry()

	var executionOrder []string

	// Register migrations out of order
	registry.Register(Migration{
		Version:     "20240103",
		Description: "Third migration",
		Apply: func(cfg *Config) error {
			executionOrder = append(executionOrder, "20240103")
			return nil
		},
	})

	registry.Register(Migration{
		Version:     "20240101",
		Description: "First migration",
		Apply: func(cfg *Config) error {
			executionOrder = append(executionOrder, "20240101")
			return nil
		},
	})

	registry.Register(Migration{
		Version:     "20240102",
		Description: "Second migration",
		Apply: func(cfg *Config) error {
			executionOrder = append(executionOrder, "20240102")
			return nil
		},
	})

	cfg := Default()
	if err := registry.RunAll(&cfg); err != nil {
		t.Fatalf("RunAll failed: %v", err)
	}

	// Verify execution order
	expectedOrder := []string{"20240101", "20240102", "20240103"}
	if len(executionOrder) != len(expectedOrder) {
		t.Fatalf("expected %d migrations executed, got %d", len(expectedOrder), len(executionOrder))
	}

	for i, expected := range expectedOrder {
		if executionOrder[i] != expected {
			t.Errorf("migration %d: expected %s, got %s", i, expected, executionOrder[i])
		}
	}
}

func TestMigrationRegistry_RunAll_StopsOnError(t *testing.T) {
	registry := NewMigrationRegistry()

	var executionOrder []string

	registry.Register(Migration{
		Version:     "20240101",
		Description: "First migration",
		Apply: func(cfg *Config) error {
			executionOrder = append(executionOrder, "20240101")
			return nil
		},
	})

	registry.Register(Migration{
		Version:     "20240102",
		Description: "Failing migration",
		Apply: func(cfg *Config) error {
			executionOrder = append(executionOrder, "20240102")
			return errors.New("migration failed")
		},
	})

	registry.Register(Migration{
		Version:     "20240103",
		Description: "Third migration",
		Apply: func(cfg *Config) error {
			executionOrder = append(executionOrder, "20240103")
			return nil
		},
	})

	cfg := Default()
	err := registry.RunAll(&cfg)

	if err == nil {
		t.Fatal("expected error from RunAll, got nil")
	}

	// Should have executed first two migrations
	if len(executionOrder) != 2 {
		t.Errorf("expected 2 migrations executed before error, got %d", len(executionOrder))
	}

	// Third migration should not have been executed
	for _, version := range executionOrder {
		if version == "20240103" {
			t.Error("third migration should not have been executed after error")
		}
	}
}

func TestDefaultMigrations_ContainsStateDatabaseMigration(t *testing.T) {
	registry := DefaultMigrations()

	if len(registry.migrations) == 0 {
		t.Fatal("expected at least one default migration")
	}

	// Check that the state database migration is registered
	found := false
	for _, m := range registry.migrations {
		if m.Version == "20260306_move_state_database" {
			found = true
			if m.Description == "" {
				t.Error("migration should have a description")
			}
			if m.Apply == nil {
				t.Error("migration should have an Apply function")
			}
			break
		}
	}

	if !found {
		t.Error("state database migration not found in default migrations")
	}
}

func TestDefaultMigrations_ContainsStripPATMigration(t *testing.T) {
	registry := DefaultMigrations()

	found := false
	for _, m := range registry.migrations {
		if m.Version == "20260317_strip_pat_from_origins" {
			found = true
			if m.Description == "" {
				t.Error("strip-PAT migration should have a description")
			}
			if m.Apply == nil {
				t.Error("strip-PAT migration should have an Apply function")
			}
			break
		}
	}

	if !found {
		t.Error("strip-PAT-from-origins migration not found in default migrations")
	}
}

func TestRunMigrations_ExecutesAllDefaults(t *testing.T) {
	cfg := Default()

	// Should not error with default config
	if err := RunMigrations(&cfg); err != nil {
		t.Errorf("RunMigrations failed: %v", err)
	}
}
