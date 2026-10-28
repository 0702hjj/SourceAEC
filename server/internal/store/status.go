// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"ifcviewer/server/internal/artifact"
)

func (s *Store) SetStatusContext(ctx context.Context, id, status, detail string) error {
	return s.setStatusContext(ctx, id, status, detail, false)
}

// Conversion admission/rejection preserves a complete preview of current source.
func (s *Store) SetConversionStatusContext(ctx context.Context, id, status, detail string) error {
	return s.setStatusContext(ctx, id, status, detail, true)
}

func (s *Store) setStatusContext(ctx context.Context, id, status, detail string, preservePreview bool) error {
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
	key := "models/" + id + "/model.json"
	var raw []byte
	if err = tx.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", key).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var model Model
	if err = json.Unmarshal(raw, &model); err != nil {
		return err
	}
	model.Status, model.Error = status, detail
	if preservePreview && status != "ready" {
		if _, e := s.viewerPath(ctx, tx, id, "model.xkt"); e == nil {
			if _, e = s.viewerPath(ctx, tx, id, "metadata.json"); e == nil {
				model.Status, model.Error = "ready", ""
			}
		}
	}
	raw, err = json.Marshal(model)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE artifacts SET content=$2 WHERE path=$1", key, raw); err != nil {
		return err
	}
	return tx.Commit()
}
