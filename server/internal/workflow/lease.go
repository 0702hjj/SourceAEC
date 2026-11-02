// SPDX-License-Identifier: Apache-2.0
package workflow

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func leaseMillis(ttl time.Duration) (int64, error) {
	if ttl < time.Second || ttl > 5*time.Minute {
		return 0, errors.New("workflow lease must be between one second and five minutes")
	}
	return ttl.Milliseconds(), nil
}

// Claim owns only a short SQL transaction; commit it before acquiring a model
// lock or running geometry. Expired claims get a new token and audit attempt.
func (s *Store) Claim(ctx context.Context, owner string, ttl time.Duration) (Run, bool, error) {
	if err := s.requireTransaction(); err != nil {
		return Run{}, false, err
	}
	if !labelPattern.MatchString(owner) {
		return Run{}, false, ErrState
	}
	millis, err := leaseMillis(ttl)
	if err != nil {
		return Run{}, false, err
	}
	// Skip live publication locks. Exhausted crashed attempts must not remain
	// permanently running, and must never be executed a fourth time silently.
	_, err = s.db.ExecContext(ctx, `WITH expired AS (
 SELECT id FROM workflow_runs WHERE state='running' AND lease_until<=clock_timestamp() AND phase_attempts>=3
 FOR UPDATE SKIP LOCKED LIMIT 64), ended AS (
 UPDATE workflow_runs SET state='failed',error='workflow attempt limit exceeded',
 lease_owner=NULL,lease_token=NULL,lease_until=NULL,updated_at=clock_timestamp()
 WHERE id IN (SELECT id FROM expired) RETURNING id)
 UPDATE workflow_attempts SET state='abandoned',finished_at=clock_timestamp()
 WHERE run_id IN (SELECT id FROM ended) AND state='running'`)
	if err != nil {
		return Run{}, false, err
	}
	r, err := scanRun(s.db.QueryRowContext(ctx, "SELECT "+runColumns+` FROM workflow_runs
 WHERE (state='pending' OR (state='running' AND lease_until<=clock_timestamp()))
 AND available_at<=clock_timestamp() AND phase_attempts<3 ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	if r.State == Running {
		if _, err = s.db.ExecContext(ctx, "UPDATE workflow_attempts SET state='abandoned',finished_at=clock_timestamp() WHERE run_id=$1 AND attempt=$2 AND state='running'", r.ID, r.Attempts); err != nil {
			return Run{}, false, err
		}
	}
	token, err := newToken()
	if err != nil {
		return Run{}, false, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE workflow_runs SET state='running',attempts=attempts+1,phase_attempts=phase_attempts+1,
 lease_owner=$2,lease_token=$3,lease_until=clock_timestamp()+$4::bigint*interval '1 millisecond',updated_at=clock_timestamp()
 WHERE id=$1`, r.ID, owner, token, millis)
	if err != nil {
		return Run{}, false, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO workflow_attempts(run_id,attempt,phase,owner,token,state) VALUES($1,$2,$3,$4,$5,'running')", r.ID, r.Attempts+1, r.Phase, owner, token)
	if err != nil {
		return Run{}, false, err
	}
	claimed, err := s.Get(ctx, r.Model, r.ID)
	return claimed, err == nil, err
}

const fencePredicate = `id=$1 AND model_id=$2 AND lease_owner=$3 AND lease_token=$4 AND attempts=$5 AND state='running' AND lease_until>clock_timestamp()`

func fenceArgs(f Fence) []any { return []any{f.ID, f.Model, f.Owner, f.Token, f.Attempt} }

// Owned locks the run after the caller has acquired the model lock. The
// publication and its final state transition must use this same transaction.
func (s *Store) Owned(ctx context.Context, f Fence) (Run, error) {
	if err := s.requireTransaction(); err != nil {
		return Run{}, err
	}
	if err := validateFence(f); err != nil {
		return Run{}, err
	}
	r, err := scanRun(s.db.QueryRowContext(ctx, "SELECT "+runColumns+" FROM workflow_runs WHERE "+fencePredicate+" FOR UPDATE", fenceArgs(f)...))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrLeaseLost
	}
	return r, err
}

func (s *Store) Heartbeat(ctx context.Context, f Fence, ttl time.Duration) error {
	if err := validateFence(f); err != nil {
		return err
	}
	millis, err := leaseMillis(ttl)
	if err != nil {
		return err
	}
	args := append(fenceArgs(f), millis)
	result, err := s.db.ExecContext(ctx, "UPDATE workflow_runs SET lease_until=clock_timestamp()+$6::bigint*interval '1 millisecond',updated_at=clock_timestamp() WHERE "+fencePredicate, args...)
	return changed(result, err)
}

func changed(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrLeaseLost
	}
	return err
}
