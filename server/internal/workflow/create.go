// SPDX-License-Identifier: Apache-2.0
package workflow

import (
	"context"
	"database/sql"
	"errors"
)

// Create commits only metadata/references. The caller must already hold the
// model lock and atomically insert verified blobs in this same transaction.
// A separate pre-capture reservation is required before reading large inputs.
func (s *Store) Create(ctx context.Context, in Create) (Run, bool, error) {
	if err := s.requireTransaction(); err != nil {
		return Run{}, false, err
	}
	if !modelPattern.MatchString(in.Model) || !labelPattern.MatchString(in.Key) {
		return Run{}, false, ErrState
	}
	intent, intentHash, err := in.Intent.encode()
	if err != nil {
		return Run{}, false, err
	}
	if _, err = s.db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema()||':workflow-admission',0))"); err != nil {
		return Run{}, false, err
	}
	prior, err := s.Lookup(ctx, in.Model, in.Key)
	if err == nil {
		if prior.IntentDigest != intentHash {
			return Run{}, false, ErrConflict
		}
		return prior, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, err
	}
	if in.Definition.Model != in.Model || !hashPattern.MatchString(in.BaseIdentity) {
		return Run{}, false, ErrState
	}
	body, definitionHash, err := in.Definition.Encode()
	if err != nil {
		return Run{}, false, err
	}
	bytes := in.Definition.InputBytes + in.Definition.Script.Bytes + in.Definition.Inputs.Bytes + in.Definition.Environment.Bytes
	var count, used int64
	if count, used, err = s.pendingUsage(ctx); err != nil {
		return Run{}, false, err
	}
	if count >= MaxPending || bytes > MaxPendingBytes-used {
		return Run{}, false, ErrCapacity
	}
	if _, err = s.db.ExecContext(ctx, `INSERT INTO workflow_definitions(model_id,digest,kind,body,input_bytes) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, in.Model, definitionHash, in.Definition.Kind, body, bytes); err != nil {
		return Run{}, false, err
	}
	token, err := newToken()
	if err != nil {
		return Run{}, false, err
	}
	id := "wf_" + token
	_, err = s.db.ExecContext(ctx, `INSERT INTO workflow_runs(id,model_id,definition,kind,action,request_key,intent,intent_digest,base_identity,input_bytes,phase,state)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'geometry','pending')`, id, in.Model, definitionHash, in.Definition.Kind, in.Intent.Action, in.Key, intent, intentHash, in.BaseIdentity, bytes)
	if err != nil {
		return Run{}, false, err
	}
	r, err := s.Get(ctx, in.Model, id)
	return r, true, err
}
