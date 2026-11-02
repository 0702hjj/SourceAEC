// SPDX-License-Identifier: Apache-2.0
package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const leaseColumns = `lease_owner=NULL,lease_token=NULL,lease_until=NULL`
const clearLease = leaseColumns + `,updated_at=clock_timestamp()`

func (s *Store) endAttempt(ctx context.Context, f Fence, state, message string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE workflow_attempts SET state=$4,error=$5,finished_at=clock_timestamp()
 WHERE run_id=$1 AND attempt=$2 AND token=$3 AND state='running'`, f.ID, f.Attempt, f.Token, state, message)
	return changed(result, err)
}

func (s *Store) CompleteGeometry(ctx context.Context, f Fence, candidate Ref) error {
	r, err := s.Owned(ctx, f)
	if err != nil {
		return err
	}
	if r.Phase != Geometry {
		return ErrState
	}
	if err := ValidateRef(r.Model, candidate); err != nil {
		return err
	}
	if candidate.Bytes > 1<<20 {
		return errors.New("workflow checkpoint manifest exceeds limit")
	}
	body, _ := json.Marshal(candidate)
	args := append(fenceArgs(f), body)
	result, err := s.db.ExecContext(ctx, `UPDATE workflow_runs SET phase='publication',state='pending',phase_attempts=0,candidate=$6,
 error='',available_at=clock_timestamp(),`+clearLease+" WHERE "+fencePredicate, args...)
	if err := changed(result, err); err != nil {
		return err
	}
	return s.endAttempt(ctx, f, "completed", "")
}

// Call only after publishing the captured candidate under the model lock, in
// that same transaction. A lost acknowledgement can then be resolved by the
// application's existing commit receipt without advancing the version twice.
func (s *Store) CompletePublication(ctx context.Context, f Fence, value json.RawMessage) error {
	r, err := s.Owned(ctx, f)
	if err != nil {
		return err
	}
	if r.Phase != Publication || r.Candidate == nil {
		return ErrState
	}
	trimmed := bytes.TrimSpace(value)
	if len(value) > 1<<20 || !json.Valid(value) || len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("invalid workflow publication result")
	}
	phase, state := Publication, Succeeded
	if r.Kind == "ifc" {
		phase, state = Conversion, Pending
	}
	args := append(fenceArgs(f), value, phase, state)
	result, err := s.db.ExecContext(ctx, `UPDATE workflow_runs SET result=$6,phase=$7,state=$8,phase_attempts=0,
 error='',available_at=clock_timestamp(),`+clearLease+" WHERE "+fencePredicate, args...)
	if err := changed(result, err); err != nil {
		return err
	}
	return s.endAttempt(ctx, f, "completed", "")
}

func (s *Store) CompleteConversion(ctx context.Context, f Fence) error {
	r, err := s.Owned(ctx, f)
	if err != nil {
		return err
	}
	if r.Phase != Conversion || r.Result == nil {
		return ErrState
	}
	return s.finish(ctx, f, Succeeded, "", "completed")
}

func (s *Store) Supersede(ctx context.Context, f Fence) error {
	if _, err := s.Owned(ctx, f); err != nil {
		return err
	}
	return s.finish(ctx, f, Superseded, "", "superseded")
}

func (s *Store) Fail(ctx context.Context, f Fence, message string, retryable bool) error {
	r, err := s.Owned(ctx, f)
	if err != nil {
		return err
	}
	message = boundedError(message)
	if !retryable || r.PhaseAttempts >= MaxPhaseAttempts {
		return s.finish(ctx, f, Failed, message, "failed")
	}
	args := append(fenceArgs(f), message, int64(1<<r.PhaseAttempts))
	result, err := s.db.ExecContext(ctx, `WITH tick AS (SELECT clock_timestamp() AS now)
 UPDATE workflow_runs SET state='pending',error=$6,updated_at=tick.now,
 available_at=tick.now+$7::bigint*interval '1 second',`+leaseColumns+" FROM tick WHERE "+fencePredicate, args...)
	if err := changed(result, err); err != nil {
		return err
	}
	return s.endAttempt(ctx, f, "failed", message)
}

func (s *Store) finish(ctx context.Context, f Fence, state State, message, audit string) error {
	args := append(fenceArgs(f), state, message)
	result, err := s.db.ExecContext(ctx, "UPDATE workflow_runs SET state=$6,error=$7,"+clearLease+" WHERE "+fencePredicate, args...)
	if err := changed(result, err); err != nil {
		return err
	}
	return s.endAttempt(ctx, f, audit, message)
}

func boundedError(message string) string {
	message = strings.ReplaceAll(strings.ToValidUTF8(message, "�"), "\x00", "�")
	if len(message) > 2048 {
		end := 2048
		for !utf8.RuneStart(message[end]) {
			end--
		}
		message = message[:end]
	}
	return message
}
