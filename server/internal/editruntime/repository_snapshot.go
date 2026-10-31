// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"errors"
)

type RepositorySnapshot struct {
	Bundle []byte
	Branch string
}

func repositorySnapshot(ctx context.Context, db queryer, model string) (RepositorySnapshot, error) {
	var s RepositorySnapshot
	err := db.QueryRowContext(ctx, "SELECT bundle,branch FROM repositories WHERE model_id=$1", model).Scan(&s.Bundle, &s.Branch)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return s, err
}

// CaptureRepository holds immutable bytes until the next branch preparation.
// Git commands can then run after the read snapshot has released its connection.
func (r *Runtime) CaptureRepository(ctx context.Context, root, token, model string) error {
	s, err := r.ReadRepositorySnapshot(ctx, root, token, model)
	if err == nil {
		r.repositorySnapshot = &s
	}
	return err
}

func (r *Runtime) ReadRepositorySnapshot(ctx context.Context, root, token, model string) (RepositorySnapshot, error) {
	db, err := r.database(Request{Root: root, Tx: token})
	if err != nil {
		return RepositorySnapshot{}, err
	}
	if err = historyProjectionReady(ctx, db, model); err != nil {
		return RepositorySnapshot{}, err
	}
	return repositorySnapshot(ctx, db, model)
}
