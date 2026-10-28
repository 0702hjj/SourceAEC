// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

var transactionToken = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Editing holds the shared lock before changing projections. Startup recovery
// takes the exclusive lock, so it cannot restore over a live editing write.
func LockProjectionRecovery(ctx context.Context, tx *sql.Tx, root string, shared bool) error {
	namespace, err := Namespace(root)
	if err != nil {
		return err
	}
	function := "pg_advisory_xact_lock"
	if shared {
		function += "_shared"
	}
	_, err = tx.ExecContext(ctx, "SELECT "+function+"(hashtextextended($1, 0))", namespace+":projection-recovery")
	return err
}

func projectionPath(root, key string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(key))
	if key == "" || filepath.IsAbs(key) || !inside(root, path) || path == root {
		return "", errors.New("invalid projection path")
	}
	for current := path; current != root; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symlink projection path")
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return path, nil
}

// RecordProjection is fsynced before the corresponding projection changes.
// Only paths are recorded: recovery always reads authoritative committed data.
func RecordProjection(root, token, key string) error {
	if !transactionToken.MatchString(token) {
		return errors.New("invalid projection transaction")
	}
	if _, err := projectionPath(root, key); err != nil {
		return err
	}
	path, err := projectionPath(root, ".edit-journal/"+token+".jsonl")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return errors.New("invalid projection journal")
	}
	if err = json.NewEncoder(file).Encode(key); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func ForgetProjections(root, token string) {
	if transactionToken.MatchString(token) {
		_ = os.Remove(filepath.Join(root, ".edit-journal", token+".jsonl"))
	}
}

// RecoverProjectionTransaction runs after rollback, including cancellation
// where database/sql may already have released the original transaction lock.
// Re-reading committed data avoids restoring stale bytes over a newer writer.
func RecoverProjectionTransaction(root, token, model string) error {
	if !transactionToken.MatchString(token) {
		return errors.New("invalid projection transaction")
	}
	path, err := projectionPath(root, ".edit-journal/"+token+".jsonl")
	if err != nil {
		return err
	}
	if _, err = os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	db, err := Database(root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if model != "" {
		if err = LockModel(ctx, tx, root, model); err != nil {
			return err
		}
	}
	if err = LockProjectionRecovery(ctx, tx, root, true); err != nil {
		return err
	}
	if err = recoverJournal(root, path, tx); err != nil && !os.IsNotExist(err) {
		return err
	}
	ForgetProjections(root, token)
	return tx.Commit()
}

func recoverProjections(root string, tx *sql.Tx) error {
	directory, err := projectionPath(root, ".edit-journal")
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".jsonl" || !transactionToken.MatchString(name[:len(name)-len(".jsonl")]) {
			return errors.New("invalid projection journal name")
		}
		path, err := projectionPath(root, ".edit-journal/"+name)
		if err != nil {
			return err
		}
		if err = recoverJournal(root, path, tx); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func recoverJournal(root, path string, tx *sql.Tx) error {
	return recoverJournalScope(context.Background(), root, path, tx, nil)
}

func recoverJournalScope(ctx context.Context, root, path string, tx *sql.Tx, include func(string) bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return errors.New("invalid projection journal")
	}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			// A torn last append precedes its projection write and is safe to drop.
			return nil
		}
		if err != nil {
			return err
		}
		var key string
		if err = json.Unmarshal(line, &key); err != nil {
			return fmt.Errorf("invalid projection journal entry: %w", err)
		}
		if include != nil && !include(key) {
			continue
		}
		target, err := projectionPath(root, key)
		if err != nil {
			return err
		}
		var data []byte
		var mode uint32
		err = tx.QueryRowContext(ctx, "SELECT content,mode FROM artifacts WHERE path=$1", key).Scan(&data, &mode)
		if errors.Is(err, sql.ErrNoRows) {
			err = os.Remove(target)
			if os.IsNotExist(err) {
				err = nil
			}
		} else if err == nil {
			if err = os.MkdirAll(filepath.Dir(target), 0755); err == nil {
				err = os.WriteFile(target, data, os.FileMode(mode))
			}
		}
		if err != nil {
			return err
		}
	}
}
