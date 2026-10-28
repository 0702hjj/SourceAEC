// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ifcviewer/server/internal/artifact"
)

// A receipt commits with the state. A lost COMMIT acknowledgement can be
// resolved without replaying user code or mistaking success for rollback.
func (r *Runtime) commit(t *transaction) error {
	defer t.tx.Rollback()
	if !t.readOnly {
		if _, err := t.tx.Exec("DELETE FROM transaction_receipts WHERE created_at < clock_timestamp() - interval '10 minutes'"); err != nil {
			return err
		}
		if _, err := t.tx.Exec("INSERT INTO transaction_receipts(token) VALUES($1)", t.token); err != nil {
			return err
		}
	}
	commit := r.commitTx
	if commit == nil {
		commit = func(tx *sql.Tx) error { return tx.Commit() }
	}
	err := commit(t.tx)
	if err == nil {
		return nil
	}
	_ = t.tx.Rollback()
	if !t.readOnly {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		db, e := artifact.Database(t.root)
		if e != nil {
			return fmt.Errorf("commit outcome unknown: %w", err)
		}
		// Taking the model lock also waits for a disconnected backend's transaction
		// to finish before checking whether its receipt became visible.
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			return fmt.Errorf("commit outcome unknown: %w", err)
		}
		defer tx.Rollback()
		if t.model != "" {
			e = artifact.LockModel(ctx, tx, t.root, t.model)
		}
		if e != nil {
			return fmt.Errorf("commit outcome unknown: %w", err)
		}
		var committed bool
		e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM transaction_receipts WHERE token=$1)", t.token).Scan(&committed)
		if e != nil {
			return fmt.Errorf("commit outcome unknown: %w", err)
		}
		if committed {
			return nil
		}
	}
	return err
}
