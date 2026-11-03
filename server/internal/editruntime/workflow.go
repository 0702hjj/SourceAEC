// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"

	"ifcviewer/server/internal/workflow"
)

// WorkflowModel uses the existing model transaction and commit receipt. The
// callback must not retain its store or start another runtime transaction.
func (r *Runtime) WorkflowModel(ctx context.Context, root, token, model string, apply func(*workflow.Store) error) error {
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return err
	}
	tx, ok := db.(*sql.Tx)
	if !ok {
		return errors.New("workflow requires a model write transaction")
	}
	return apply(workflow.New(tx))
}

func (r *Runtime) WorkflowConversionIdentity(ctx context.Context, root, token, model string) (string, error) {
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return "", err
	}
	var product, build []byte
	if err := db.QueryRowContext(ctx, "SELECT content_hash FROM artifacts WHERE path=$1", "uploads/"+model+".ifc").Scan(&product); err != nil {
		return "", err
	}
	if err := db.QueryRowContext(ctx, "SELECT content_hash FROM artifacts WHERE path=$1", "uploads/"+model+".ifc.build.json").Scan(&build); err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(product, build...))
	return hex.EncodeToString(sum[:]), nil
}

func (r *Runtime) WorkflowDefinition(ctx context.Context, root, token, model, digest string) (workflow.Definition, error) {
	db, err := r.database(Request{Context: ctx, Root: root, Tx: token})
	if err != nil {
		return workflow.Definition{}, err
	}
	return workflow.New(db).Definition(ctx, model, digest)
}
func (r *Runtime) StoredArtifactHash(ctx context.Context, root, token, key string) (string, int64, error) {
	db, err := r.database(Request{Context: ctx, Root: root, Tx: token})
	if err != nil {
		return "", 0, err
	}
	var hash string
	var size int64
	err = db.QueryRowContext(ctx, "SELECT encode(content_hash,'hex'),octet_length(content) FROM artifacts WHERE path=$1", key).Scan(&hash, &size)
	if errors.Is(err, sql.ErrNoRows) {
		err = os.ErrNotExist
	}
	return hash, size, err
}

// StoreWorkflowBlob writes database-only checkpoints, never workspace projections.
func (r *Runtime) StoreWorkflowBlob(ctx context.Context, root, token, model string, ref workflow.Ref, data []byte) error {
	if err := workflow.ValidateRef(model, ref); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if ref.Bytes != int64(len(data)) || ref.SHA256 != hex.EncodeToString(sum[:]) {
		return errors.New("workflow blob content mismatch")
	}
	db, err := r.historyDB(ctx, root, token, model)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO artifacts(path,content) VALUES($1,$2) ON CONFLICT(path) DO UPDATE SET content=excluded.content", ref.Key, data)
	return err
}
