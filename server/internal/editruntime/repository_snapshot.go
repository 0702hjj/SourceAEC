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
	lease  *gitReference
}

// Close releases this snapshot view. Copies share the same idempotent handle;
// retaining a Git workspace creates its own reference until cleanup completes.
func (s *RepositorySnapshot) Close() { s.lease.close(); s.Bundle = nil }

func repositorySnapshot(ctx context.Context, db queryer, model string) (RepositorySnapshot, error) {
	var size int64
	if err := db.QueryRowContext(ctx, "SELECT octet_length(bundle) FROM repositories WHERE model_id=$1", model).Scan(&size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RepositorySnapshot{}, nil
		}
		return RepositorySnapshot{}, err
	}
	lease, err := gitResources.acquire(ctx, size)
	if err != nil {
		return RepositorySnapshot{}, err
	}
	s := RepositorySnapshot{lease: lease}
	var actual int64
	err = db.QueryRowContext(ctx, `SELECT CASE WHEN octet_length(bundle)<=$2 THEN bundle END,branch,octet_length(bundle)
 FROM repositories WHERE model_id=$1`, model, size).Scan(&s.Bundle, &s.Branch, &actual)
	if err != nil || actual > size || int64(len(s.Bundle)) != actual {
		s.Close()
		if errors.Is(err, sql.ErrNoRows) {
			return RepositorySnapshot{}, nil
		}
		if err != nil {
			return RepositorySnapshot{}, err
		}
		return RepositorySnapshot{}, ErrGitCapacity
	}
	return s, nil
}

// CaptureRepository holds immutable bytes until the next branch preparation.
// Git commands can then run after the read snapshot has released its connection.
func (r *Runtime) CaptureRepository(ctx context.Context, root, token, model string) error {
	s, err := r.ReadRepositorySnapshot(ctx, root, token, model)
	if err == nil {
		if r.repositorySnapshot != nil {
			r.repositorySnapshot.Close()
		}
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
