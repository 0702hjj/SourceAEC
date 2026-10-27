// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"bytes"
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
)

// Restore repairs projections without touching a live editing transaction.
func Restore(ctx context.Context, root string) error {
	db, err := Database(root)
	if err != nil {
		return err
	}
	return restoreWorkspace(ctx, root, db)
}

func restoreWorkspace(ctx context.Context, root string, db *sql.DB) error {
	recovery, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer recovery.Rollback()
	if err = LockProjectionRecovery(ctx, recovery, root, false); err != nil {
		return err
	}
	if err = recoverProjections(root, recovery); err != nil {
		return err
	}
	rows, err := recovery.QueryContext(ctx, "SELECT path,content,mode FROM artifacts")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var data []byte
		var mode uint32
		if err = rows.Scan(&key, &data, &mode); err != nil {
			return err
		}
		path, err := projectionPath(root, key)
		if err != nil {
			return err
		}
		if old, e := os.ReadFile(path); e == nil && bytes.Equal(old, data) {
			continue
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err = os.WriteFile(path, data, fs.FileMode(mode)); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return recovery.Commit()
}
