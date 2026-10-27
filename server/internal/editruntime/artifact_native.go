// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"ifcviewer/server/internal/artifact"
)

// Native artifact calls share the protocol's transaction and durability rules
// without serializing large products through JSON and base64 in-process.
func (r *Runtime) ReadArtifact(ctx context.Context, root, token, key string) ([]byte, error) {
	return r.readArtifact(ctx, root, token, key, true)
}

// ReadStoredArtifact never falls back to a possibly uncommitted projection.
func (r *Runtime) ReadStoredArtifact(ctx context.Context, root, token, key string) ([]byte, error) {
	return r.readArtifact(ctx, root, token, key, false)
}

func (r *Runtime) readArtifact(ctx context.Context, root, token, key string, projectionFallback bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db, err := r.database(Request{Root: root, Tx: token})
	if err != nil {
		return nil, err
	}
	path, err := safePath(root, key)
	if err != nil {
		return nil, err
	}
	var content []byte
	err = db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE path=$1", key).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		if !projectionFallback {
			return nil, os.ErrNotExist
		}
		if t := r.transactions[token]; t != nil && t.readOnly {
			return nil, os.ErrNotExist
		}
		content, err = os.ReadFile(path)
	}
	return content, err
}

func (r *Runtime) PublishArtifacts(ctx context.Context, root, token string, files []ArtifactFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	req := Request{Context: ctx, Root: root, Tx: token}
	db, err := r.database(req)
	if err != nil {
		return err
	}
	if token != "" {
		return r.publish(req, db, files)
	}
	pool, err := artifact.Database(root)
	if err != nil {
		return err
	}
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = r.publish(req, tx, files); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Runtime) TrackProjection(ctx context.Context, root, token, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.track(Request{Context: ctx, Root: root, Tx: token}, key)
}
