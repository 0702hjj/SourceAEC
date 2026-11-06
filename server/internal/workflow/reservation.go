// SPDX-License-Identifier: Apache-2.0
package workflow

import (
	"context"
	"database/sql"
	"errors"
)

const CaptureBytes int64 = (256 + 3) << 20

type Reservation struct{ Model, Key, Token string }

func (s *Store) admissionLock(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema()||':workflow-admission',0))")
	return err
}
func (s *Store) pendingUsage(ctx context.Context) (int64, int64, error) {
	var count, bytes int64
	err := s.db.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(bytes),0) FROM (
 SELECT input_bytes AS bytes FROM workflow_runs WHERE state IN ('pending','running')
 UNION ALL SELECT bytes FROM workflow_reservations WHERE expires_at>clock_timestamp()) AS pending`).Scan(&count, &bytes)
	return count, bytes, err
}

// Reserve before reading payloads or preparing an environment. A successful
// retry returns the original run; an in-progress capture is retryably busy.
func (s *Store) Reserve(ctx context.Context, model, key string, intent Intent) (Reservation, *Run, error) {
	if err := s.requireTransaction(); err != nil {
		return Reservation{}, nil, err
	}
	if !modelPattern.MatchString(model) || !labelPattern.MatchString(key) {
		return Reservation{}, nil, ErrState
	}
	_, hash, err := intent.encode()
	if err != nil {
		return Reservation{}, nil, err
	}
	if err = s.admissionLock(ctx); err != nil {
		return Reservation{}, nil, err
	}
	prior, err := s.Lookup(ctx, model, key)
	if err == nil {
		if prior.IntentDigest != hash {
			return Reservation{}, nil, ErrConflict
		}
		return Reservation{}, &prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, nil, err
	}
	if _, err = s.db.ExecContext(ctx, "DELETE FROM workflow_reservations WHERE expires_at<=clock_timestamp()"); err != nil {
		return Reservation{}, nil, err
	}
	var existing string
	err = s.db.QueryRowContext(ctx, "SELECT intent_digest FROM workflow_reservations WHERE model_id=$1 AND request_key=$2", model, key).Scan(&existing)
	if err == nil {
		if existing != hash {
			return Reservation{}, nil, ErrConflict
		}
		return Reservation{}, nil, ErrCapacity
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, nil, err
	}
	count, used, err := s.pendingUsage(ctx)
	if err != nil {
		return Reservation{}, nil, err
	}
	if count >= MaxPending || CaptureBytes > MaxPendingBytes-used {
		return Reservation{}, nil, ErrCapacity
	}
	token, err := newToken()
	if err != nil {
		return Reservation{}, nil, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO workflow_reservations(model_id,request_key,token,intent_digest,bytes,expires_at)
 VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '5 minutes')`, model, key, token, hash, CaptureBytes)
	return Reservation{model, key, token}, nil, err
}

func (s *Store) ReleaseReservation(ctx context.Context, r Reservation) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM workflow_reservations WHERE model_id=$1 AND request_key=$2 AND token=$3", r.Model, r.Key, r.Token)
	return err
}

func (s *Store) CreateReserved(ctx context.Context, in Create, r Reservation) (Run, bool, error) {
	if err := s.requireTransaction(); err != nil {
		return Run{}, false, err
	}
	if in.Model != r.Model || in.Key != r.Key {
		return Run{}, false, ErrLeaseLost
	}
	if err := s.admissionLock(ctx); err != nil {
		return Run{}, false, err
	}
	_, intentHash, err := in.Intent.encode()
	if err != nil {
		return Run{}, false, err
	}
	var hash string
	err = s.db.QueryRowContext(ctx, `SELECT intent_digest FROM workflow_reservations WHERE model_id=$1 AND request_key=$2 AND token=$3
 AND expires_at>clock_timestamp() FOR UPDATE`, r.Model, r.Key, r.Token).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, ErrLeaseLost
	}
	if err != nil {
		return Run{}, false, err
	}
	if hash != intentHash {
		return Run{}, false, ErrConflict
	}
	if err = s.ReleaseReservation(ctx, r); err != nil {
		return Run{}, false, err
	}
	return s.Create(ctx, in)
}
