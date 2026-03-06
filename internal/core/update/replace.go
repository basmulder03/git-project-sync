package update

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ApplyError struct {
	Cause             error
	RollbackPerformed bool
}

func (e ApplyError) Error() string {
	if e.Cause == nil {
		return "update apply failed"
	}
	return e.Cause.Error()
}

func (e ApplyError) Unwrap() error {
	return e.Cause
}

type ApplyResult struct {
	TargetPath string
	Version    string
}

func ReplaceBinaryWithRollback(targetPath, candidatePath string) error {
	if _, err := os.Stat(targetPath); err != nil {
		if os.IsNotExist(err) {
			if renameErr := os.Rename(candidatePath, targetPath); renameErr != nil {
				return ApplyError{Cause: fmt.Errorf("install new binary: %w", renameErr), RollbackPerformed: false}
			}
			return nil
		}
		return ApplyError{Cause: fmt.Errorf("inspect current binary: %w", err), RollbackPerformed: false}
	}

	backupPath := backupPathFor(targetPath)

	if err := os.Rename(targetPath, backupPath); err != nil {
		return ApplyError{Cause: fmt.Errorf("move current binary to backup: %w", err), RollbackPerformed: false}
	}

	if err := os.Rename(candidatePath, targetPath); err != nil {
		rollbackErr := os.Rename(backupPath, targetPath)
		if rollbackErr != nil {
			return ApplyError{Cause: fmt.Errorf("replace binary failed and rollback failed: replace=%w rollback=%v", err, rollbackErr), RollbackPerformed: false}
		}
		return ApplyError{Cause: fmt.Errorf("replace binary failed: %w", err), RollbackPerformed: true}
	}

	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return ApplyError{Cause: fmt.Errorf("cleanup backup after successful replace: %w", err), RollbackPerformed: false}
	}

	return nil
}

func backupPathFor(targetPath string) string {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	return filepath.Join(dir, base+".bak."+time.Now().UTC().Format("20060102150405"))
}
