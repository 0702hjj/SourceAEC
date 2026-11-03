// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"database/sql"
	"ifcviewer/server/internal/artifact"
)

// ConversionIdentity avoids loading the product during workflow status polls.
func (s *Store) ConversionIdentity(ctx context.Context, id string) (string, error) {
	if !validID(id) {
		return "", ErrInvalidID
	}
	db, err := artifact.Database(s.DataDir)
	if err != nil {
		return "", err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	return conversionIdentity(ctx, tx, id)
}
