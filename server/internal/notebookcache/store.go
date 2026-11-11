// SPDX-License-Identifier: Apache-2.0
// Package notebookcache stores disposable, model-scoped notebook generations.
// It never interprets SDK pickle data or schedules model work.
package notebookcache

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
)

const MaxBytes = 64 << 20
const MaxFiles = 4096
const Policy = "simplecadapi-v1"

type SQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Store struct{ db SQL }

func New(db SQL) *Store { return &Store{db: db} }

func Schema(ctx context.Context, db SQL) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notebook_generations (
 model_id TEXT NOT NULL, generation TEXT NOT NULL, identity TEXT NOT NULL,
 payload BYTEA NOT NULL CHECK(octet_length(payload)<=67108864),
 PRIMARY KEY(model_id,generation));
 CREATE TABLE IF NOT EXISTS notebook_heads (
 model_id TEXT PRIMARY KEY, generation TEXT NOT NULL,
 FOREIGN KEY(model_id,generation) REFERENCES notebook_generations(model_id,generation));`)
	return err
}

var modelPattern = regexp.MustCompile(`^m_[0-9a-f]{16}$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func Hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

var ErrCorrupt = errors.New("corrupt notebook cache generation")

type Snapshot struct {
	Identity, Generation string
	Payload              []byte
}

// A single statement captures the pointer and immutable bytes consistently.
// Schema and write-side bounds prevent reading an oversized cache first.
func (s *Store) Load(ctx context.Context, model string) (Snapshot, error) {
	if !modelPattern.MatchString(model) {
		return Snapshot{}, errors.New("invalid notebook cache model")
	}
	var result Snapshot
	err := s.db.QueryRowContext(ctx, `SELECT g.identity,g.generation,g.payload FROM notebook_heads h
 JOIN notebook_generations g USING(model_id,generation) WHERE h.model_id=$1`, model).Scan(&result.Identity, &result.Generation, &result.Payload)
	if err != nil {
		return Snapshot{}, err
	}
	if len(result.Payload) > MaxBytes || !hashPattern.MatchString(result.Identity) || Hash(result.Payload) != result.Generation {
		return Snapshot{}, ErrCorrupt
	}
	return result, nil
}

// Publish must join the existing fenced model/checkpoint transaction. Bytes
// have already been packed/validated before entering this transaction.
func (s *Store) Publish(ctx context.Context, model string, value Snapshot) error {
	if _, ok := s.db.(*sql.Tx); !ok {
		return errors.New("notebook cache publication requires a transaction")
	}
	if !modelPattern.MatchString(model) || !hashPattern.MatchString(value.Identity) || len(value.Payload) > MaxBytes || Hash(value.Payload) != value.Generation {
		return errors.New("invalid notebook cache generation")
	}
	if err := validateSnapshotIdentity(ctx, value); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO notebook_generations(model_id,generation,identity,payload)
 VALUES($1,$2,$3,$4) ON CONFLICT(model_id,generation) DO UPDATE SET payload=excluded.payload,identity=excluded.identity
 WHERE encode(sha256(notebook_generations.payload),'hex')<>notebook_generations.generation OR notebook_generations.identity<>excluded.identity`, model, value.Generation, value.Identity, value.Payload); err != nil {
		return err
	}
	var identity string
	if err := s.db.QueryRowContext(ctx, "SELECT identity FROM notebook_generations WHERE model_id=$1 AND generation=$2", model, value.Generation).Scan(&identity); err != nil {
		return err
	}
	if identity != value.Identity {
		return errors.New("notebook cache generation identity is immutable")
	}
	// Concurrent attempts are serialized by the caller's model lock. No writer
	// mutates another generation; readers materialize a private copy before use.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO notebook_heads(model_id,generation) VALUES($1,$2)
 ON CONFLICT(model_id) DO UPDATE SET generation=excluded.generation`, model, value.Generation); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM notebook_generations WHERE model_id=$1 AND generation<>$2`, model, value.Generation)
	return err
}
func (s *Store) DeleteModel(ctx context.Context, model string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM notebook_heads WHERE model_id=$1", model); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM notebook_generations WHERE model_id=$1", model)
	return err
}
