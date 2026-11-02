// SPDX-License-Identifier: Apache-2.0
package workflow

import (
	"context"
	"encoding/json"
	"errors"
)

const summaryColumns = `id,model_id,definition,kind,action,phase,state,attempts,phase_attempts,created_at,updated_at,error`
const runColumns = summaryColumns + `,request_key,intent_digest,base_identity,intent,
 coalesce(lease_owner,''),coalesce(lease_token,''),lease_until,candidate,result,input_bytes`

type scanner interface{ Scan(...any) error }

func scanRun(row scanner) (Run, error) {
	var r Run
	var intent, candidate, result []byte
	err := row.Scan(&r.ID, &r.Model, &r.Definition, &r.Kind, &r.Action, &r.Phase, &r.State, &r.Attempts, &r.PhaseAttempts,
		&r.CreatedAt, &r.UpdatedAt, &r.Error, &r.Key, &r.IntentDigest, &r.BaseIdentity, &intent, &r.Owner, &r.Token, &r.LeaseUntil, &candidate, &result, &r.InputBytes)
	if err != nil {
		return r, err
	}
	r.Result = result
	if err = json.Unmarshal(intent, &r.Intent); err != nil {
		return r, err
	}
	if candidate != nil {
		r.Candidate = &Ref{}
		err = json.Unmarshal(candidate, r.Candidate)
	}
	return r, err
}

func (s *Store) Get(ctx context.Context, model, id string) (Run, error) {
	if !modelPattern.MatchString(model) || !runPattern.MatchString(id) {
		return Run{}, ErrState
	}
	return scanRun(s.db.QueryRowContext(ctx, "SELECT "+runColumns+" FROM workflow_runs WHERE model_id=$1 AND id=$2", model, id))
}

func (s *Store) Lookup(ctx context.Context, model, key string) (Run, error) {
	if !modelPattern.MatchString(model) || !labelPattern.MatchString(key) {
		return Run{}, ErrState
	}
	return scanRun(s.db.QueryRowContext(ctx, "SELECT "+runColumns+" FROM workflow_runs WHERE model_id=$1 AND request_key=$2", model, key))
}

func (s *Store) Definition(ctx context.Context, model, digest string) (Definition, error) {
	if !modelPattern.MatchString(model) || !hashPattern.MatchString(digest) {
		return Definition{}, ErrState
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, "SELECT body FROM workflow_definitions WHERE model_id=$1 AND digest=$2", model, digest).Scan(&body)
	var d Definition
	if err == nil {
		err = json.Unmarshal(body, &d)
	}
	if err == nil {
		_, actual, e := d.Encode()
		if e != nil || actual != digest || d.Model != model {
			err = errors.New("workflow definition integrity check failed")
		}
	}
	return d, err
}

func (s *Store) List(ctx context.Context, model string, limit int) ([]Summary, error) {
	if !modelPattern.MatchString(model) || limit < 1 || limit > 100 {
		return nil, ErrState
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+summaryColumns+" FROM workflow_runs WHERE model_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2", model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Summary{}
	for rows.Next() {
		var r Summary
		if err := rows.Scan(&r.ID, &r.Model, &r.Definition, &r.Kind, &r.Action, &r.Phase, &r.State, &r.Attempts, &r.PhaseAttempts, &r.CreatedAt, &r.UpdatedAt, &r.Error); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) DeleteModel(ctx context.Context, model string) error {
	if !modelPattern.MatchString(model) {
		return ErrState
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM workflow_definitions WHERE model_id=$1", model)
	return err
}
