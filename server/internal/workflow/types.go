// SPDX-License-Identifier: Apache-2.0
// Package workflow stores durable script-workflow state. It does not execute
// geometry, own a queue, or publish model versions.
package workflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

type SQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Store struct{ db SQL }

func New(db SQL) *Store { return &Store{db: db} }
func (s *Store) requireTransaction() error {
	if _, ok := s.db.(*sql.Tx); !ok {
		return errors.New("workflow transition requires a transaction")
	}
	return nil
}

var (
	ErrConflict  = errors.New("workflow idempotency key has a different intent")
	ErrCapacity  = errors.New("workflow pending capacity exhausted")
	ErrLeaseLost = errors.New("workflow lease no longer owned")
	ErrState     = errors.New("invalid workflow transition")
	modelPattern = regexp.MustCompile(`^m_[0-9a-f]{16}$`)
	hashPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	runPattern   = regexp.MustCompile(`^wf_[0-9a-f]{32}$`)
	labelPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	tokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

const (
	MaxPending             = 64
	MaxPendingBytes  int64 = 2 << 30
	MaxPhaseAttempts       = 3
)

type Phase string

const (
	Geometry    Phase = "geometry"
	Publication Phase = "publication"
	Conversion  Phase = "conversion"
)

type State string

const (
	Pending    State = "pending"
	Running    State = "running"
	Succeeded  State = "succeeded"
	Failed     State = "failed"
	Canceled   State = "canceled"
	Superseded State = "superseded"
)

type Ref struct {
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Definitions contain only immutable references. The application must verify
// referenced bytes and environment manifests in the transaction creating them.
type Definition struct {
	Schema            int    `json:"schema"`
	Model             string `json:"model"`
	Kind              string `json:"kind"`
	Engine            string `json:"engine"`
	EnvironmentDigest string `json:"environmentDigest"`
	Script            Ref    `json:"script"`
	Inputs            Ref    `json:"inputs"`
	Environment       Ref    `json:"environment"`
	InputBytes        int64  `json:"inputBytes"`
}

type Intent struct {
	Action       string `json:"action"`
	BaseRevision string `json:"baseRevision"`
	Source       string `json:"source"`
	Note         string `json:"note"`
}

type Create struct {
	Model, Key, BaseIdentity string
	Definition               Definition
	Intent                   Intent
}

type Summary struct {
	ID, Model, Definition, Kind, Action string
	Phase                               Phase
	State                               State
	Attempts, PhaseAttempts             int
	CreatedAt, UpdatedAt                time.Time
	Error                               string
}

type Run struct {
	Summary
	Key, IntentDigest, BaseIdentity string
	Intent                          Intent
	Owner, Token                    string
	LeaseUntil                      sql.NullTime
	Candidate                       *Ref
	Result                          json.RawMessage
	InputBytes                      int64
}

type Fence struct {
	ID, Model, Owner, Token string
	Attempt                 int
}

func (r Run) Fence() Fence { return Fence{r.ID, r.Model, r.Owner, r.Token, r.Attempts} }

func newToken() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func hash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func validateFence(f Fence) error {
	if !runPattern.MatchString(f.ID) || !modelPattern.MatchString(f.Model) || !labelPattern.MatchString(f.Owner) || !tokenPattern.MatchString(f.Token) || f.Attempt < 1 {
		return ErrLeaseLost
	}
	return nil
}
