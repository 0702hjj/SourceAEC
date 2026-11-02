// SPDX-License-Identifier: Apache-2.0
package workflow

import "context"

func (s *Store) Cancel(ctx context.Context, model, id string) (Run, error) {
	if err := s.requireTransaction(); err != nil {
		return Run{}, err
	}
	if !modelPattern.MatchString(model) || !runPattern.MatchString(id) {
		return Run{}, ErrState
	}
	r, err := scanRun(s.db.QueryRowContext(ctx, "SELECT "+runColumns+" FROM workflow_runs WHERE model_id=$1 AND id=$2 FOR UPDATE", model, id))
	if err != nil {
		return Run{}, err
	}
	if r.State != Pending && r.State != Running {
		return r, nil
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE workflow_runs SET state='canceled',error='',"+clearLease+" WHERE id=$1", id); err != nil {
		return Run{}, err
	}
	if r.State == Running {
		if err = s.endAttempt(ctx, r.Fence(), "canceled", ""); err != nil {
			return Run{}, err
		}
	}
	return s.Get(ctx, model, id)
}

// Explicit resume retains the fixed definition/checkpoint and audit history.
// It does not rebase inputs or reopen a superseded/successful publication.
func (s *Store) Resume(ctx context.Context, model, id string) (Run, error) {
	if err := s.requireTransaction(); err != nil {
		return Run{}, err
	}
	if !modelPattern.MatchString(model) || !runPattern.MatchString(id) {
		return Run{}, ErrState
	}
	r, err := scanRun(s.db.QueryRowContext(ctx, "SELECT "+runColumns+" FROM workflow_runs WHERE model_id=$1 AND id=$2 FOR UPDATE", model, id))
	if err != nil {
		return Run{}, err
	}
	if r.State != Failed && r.State != Canceled {
		return Run{}, ErrState
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE workflow_runs SET state='pending',phase_attempts=0,error='',available_at=clock_timestamp(),`+clearLease+" WHERE id=$1", id); err != nil {
		return Run{}, err
	}
	return s.Get(ctx, model, id)
}
