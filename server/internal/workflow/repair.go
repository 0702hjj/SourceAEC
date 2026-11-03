// SPDX-License-Identifier: Apache-2.0
package workflow

import "context"

// Only an unpublished checkpoint can be rebuilt, once automatically.
func (s *Store) RebuildCheckpoint(ctx context.Context, f Fence) error {
	r, err := s.Owned(ctx, f)
	if err != nil {
		return err
	}
	if r.Phase != Publication || r.Result != nil {
		return ErrState
	}
	result, err := s.db.ExecContext(ctx, `UPDATE workflow_runs SET phase='geometry',state='pending',phase_attempts=0,
 candidate=NULL,repair_count=repair_count+1,error='',available_at=clock_timestamp(),`+clearLease+
		" WHERE "+fencePredicate+" AND repair_count<1", fenceArgs(f)...)
	if err := changed(result, err); err != nil {
		return err
	}
	return s.endAttempt(ctx, f, "failed", "checkpoint invalidated; rebuilding geometry")
}
