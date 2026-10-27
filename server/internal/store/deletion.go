// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"ifcviewer/server/internal/artifact"
	"os"
	"path/filepath"
)

func (s *Store) DeleteContext(ctx context.Context, id string) error {
	if !validID(id) {
		return ErrInvalidID
	}
	release, err := artifact.AcquireModel(ctx, s.DataDir, id)
	if err != nil {
		return err
	}
	defer release()
	db, err := artifact.Database(s.DataDir)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = artifact.LockModel(ctx, tx, s.DataDir, id); err != nil {
		return err
	}
	if err = artifact.LockProjectionRecovery(ctx, tx, s.DataDir, true); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM artifacts WHERE starts_with(path,$1) OR starts_with(path,$2)", "models/"+id+"/", "uploads/"+id+".")
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM repositories WHERE model_id=$1", id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Missing database rows are authoritative even if process death interrupts
	// best-effort removal of disposable files. IDs are never reused.
	for _, suffix := range []string{".ifc", ".dxf", ".ifc.build.json", ".dxf.build.json"} {
		if err = os.Remove(filepath.Join(s.DataDir, "uploads", id+suffix)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.RemoveAll(s.ModelDir(id))
}
