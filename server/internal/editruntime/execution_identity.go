// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ExecutionIdentity excludes derived viewer/status updates, but includes source,
// staging, immutable history, repository state and the model's ownership/epoch.
// The caller supplies a repeatable-read snapshot or holds the model write lock.
func (r *Runtime) ExecutionIdentity(ctx context.Context, root, token, model string) (string, error) {
	db, err := r.database(Request{Root: root, Tx: token})
	if err != nil {
		return "", err
	}
	rows, err := db.QueryContext(ctx, `SELECT path,encode(content_hash,'hex') FROM artifacts
 WHERE (starts_with(path,$1) AND path<>$2 AND path<>$3) OR starts_with(path,$4)
 ORDER BY path`, "models/"+model+"/", "models/"+model+"/model.json", "models/"+model+"/viewer.json", "uploads/"+model+".")
	if err != nil {
		return "", err
	}
	var state strings.Builder
	for rows.Next() {
		var key, hash string
		if err = rows.Scan(&key, &hash); err != nil {
			rows.Close()
			return "", err
		}
		state.WriteString(key + ":" + hash + "\n")
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	var metadata []byte
	err = db.QueryRowContext(ctx, `SELECT convert_to(jsonb_build_array(convert_from(content,'UTF8')::jsonb->'id',
 convert_from(content,'UTF8')::jsonb->'kind',convert_from(content,'UTF8')::jsonb->'projectId',
 convert_from(content,'UTF8')::jsonb->'createdAt')::text,'UTF8') FROM artifacts WHERE path=$1`, "models/"+model+"/model.json").Scan(&metadata)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	state.Write(metadata)
	var repo, branch string
	err = db.QueryRowContext(ctx, "SELECT encode(sha256(bundle),'hex'),branch FROM repositories WHERE model_id=$1", model).Scan(&repo, &branch)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	state.WriteString(repo + ":" + branch)
	inputs, err := collectInputs(ctx, db, model)
	if err != nil {
		return "", err
	}
	state.Write(canonical(inputs))
	return digestBytes([]byte(state.String())), nil
}
