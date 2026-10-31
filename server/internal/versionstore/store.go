// SPDX-License-Identifier: Apache-2.0
// Package versionstore stores immutable source-history references. Geometry and
// Git materialization belong to adapters, outside the normal save path.
package versionstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
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
		return errors.New("source history publication requires a transaction")
	}
	return nil
}

var ErrConflict = errors.New("source history changed")
var ErrProjectionPending = errors.New("source history has pending Git projection; retry")

// Schema shares the caller's business namespace and migration transaction.
func Schema(ctx context.Context, db SQL) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS version_heads (
 model_id TEXT PRIMARY KEY, ref TEXT NOT NULL, branch TEXT NOT NULL,
 last_number BIGINT NOT NULL CHECK(last_number>=0));
 CREATE TABLE IF NOT EXISTS version_events (
 sequence BIGSERIAL PRIMARY KEY, model_id TEXT NOT NULL,
 revision TEXT NOT NULL UNIQUE, event BYTEA NOT NULL, git_commit TEXT);
 CREATE INDEX IF NOT EXISTS version_events_model ON version_events(model_id,sequence);
 CREATE INDEX IF NOT EXISTS version_events_pending ON version_events(model_id,sequence) WHERE git_commit IS NULL;`)
	return err
}

type Head struct {
	Ref, Branch string
	LastNumber  int64
}

func (s *Store) Head(ctx context.Context, model string) (Head, error) {
	var h Head
	err := s.db.QueryRowContext(ctx, "SELECT ref,branch,last_number FROM version_heads WHERE model_id=$1", model).Scan(&h.Ref, &h.Branch, &h.LastNumber)
	return h, err
}
func (s *Store) Initialize(ctx context.Context, model string, h Head) error {
	if !modelPattern.MatchString(model) || h.Branch == "" || h.LastNumber < 0 {
		return errors.New("invalid source history model or head")
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO version_heads(model_id,ref,branch,last_number) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", model, h.Ref, h.Branch, h.LastNumber)
	return err
}
func (s *Store) NextNumber(ctx context.Context, model string) (int64, error) {
	if err := s.requireTransaction(); err != nil {
		return 0, err
	}
	var n int64
	err := s.db.QueryRowContext(ctx, "UPDATE version_heads SET last_number=last_number+1 WHERE model_id=$1 RETURNING last_number", model).Scan(&n)
	return n, err
}

type Event struct {
	Revision   string    `json:"revision"`
	Parent     string    `json:"parent"`
	Branch     string    `json:"branch"`
	Version    string    `json:"version,omitempty"`
	ScriptKey  string    `json:"scriptKey"`
	InputsKey  string    `json:"inputsKey"`
	BuildKey   string    `json:"buildKey"`
	MapKey     string    `json:"mapKey"`
	Note       string    `json:"note"`
	Source     string    `json:"source"`
	AcceptedAt time.Time `json:"acceptedAt"`
}

func NewRevision() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "r_" + hex.EncodeToString(b[:]), nil
}

var revisionPattern = regexp.MustCompile(`^r_[0-9a-f]{32}$`)
var modelPattern = regexp.MustCompile(`^m_[0-9a-f]{16}$`)

func ValidModel(model string) bool { return modelPattern.MatchString(model) }

var versionPattern = regexp.MustCompile(`^v[1-9][0-9]*$`)
var immutableKeyPattern = regexp.MustCompile(`^models/m_[0-9a-f]{16}/(scripts/v[1-9][0-9]*\.(py|inputs\.json|build\.json|map\.json)|history/snapshots/r_[0-9a-f]{32}/(script\.py|inputs\.json|build\.json|map\.json))$`)

func validateEvent(model string, e Event) error {
	if !modelPattern.MatchString(model) || !revisionPattern.MatchString(e.Revision) || (e.Source != "human" && e.Source != "ai") {
		return errors.New("invalid source history identity or provenance")
	}
	if e.Version != "" && !versionPattern.MatchString(e.Version) {
		return errors.New("invalid source history version")
	}
	for _, key := range []string{e.ScriptKey, e.InputsKey, e.BuildKey, e.MapKey} {
		if path.Clean(key) != key || !strings.HasPrefix(key, "models/"+model+"/") || !immutableKeyPattern.MatchString(key) {
			return errors.New("history must reference immutable model snapshots")
		}
	}
	return nil
}

// Append must share the transaction and model lock publishing these artifacts.
func (s *Store) Append(ctx context.Context, model string, e Event) (Event, error) {
	if err := validateEvent(model, e); err != nil {
		return Event{}, err
	}
	if err := s.requireTransaction(); err != nil {
		return Event{}, err
	}
	h, err := s.Head(ctx, model)
	if err != nil {
		return Event{}, err
	}
	e.Parent, e.Branch, e.AcceptedAt = h.Ref, h.Branch, time.Now().UTC()
	data, err := json.Marshal(e)
	if err != nil {
		return Event{}, err
	}
	if _, err = s.db.ExecContext(ctx, "INSERT INTO version_events(model_id,revision,event) VALUES($1,$2,$3)", model, e.Revision, data); err != nil {
		return Event{}, err
	}
	result, err := s.db.ExecContext(ctx, "UPDATE version_heads SET ref=$1 WHERE model_id=$2 AND ref=$3", e.Revision, model, h.Ref)
	if err != nil {
		return Event{}, err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		err = ErrConflict
	}
	return e, err
}

func (s *Store) Pending(ctx context.Context, model string, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT event FROM version_events WHERE model_id=$1 AND git_commit IS NULL ORDER BY sequence LIMIT $2", model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Event
	for rows.Next() {
		var data []byte
		var e Event
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		if err = validateEvent(model, e); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Store) MarkGit(ctx context.Context, model, revision, commit string) error {
	if err := s.requireTransaction(); err != nil {
		return err
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(commit) {
		return errors.New("invalid projected Git commit")
	}
	result, err := s.db.ExecContext(ctx, "UPDATE version_events SET git_commit=$1 WHERE model_id=$2 AND revision=$3 AND git_commit IS NULL", commit, model, revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}

// RecordedVersions prevents a legacy import from duplicating snapshots which
// will be projected from their original provenance records instead.
func (s *Store) RecordedVersions(ctx context.Context, model string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT coalesce(convert_from(event,'UTF8')::jsonb->>'version','') FROM version_events WHERE model_id=$1", model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var version string
		if err = rows.Scan(&version); err != nil {
			return nil, err
		}
		result[version] = true
	}
	return result, rows.Err()
}

// MoveGitHead is only for explicit legacy Git writes, never cache projection.
func (s *Store) MoveGitHead(ctx context.Context, model, branch, commit string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE version_heads SET ref=$1,branch=$2 WHERE model_id=$3", "git:"+commit, branch, model)
	return err
}

func (s *Store) DeleteModel(ctx context.Context, model string) error {
	for _, table := range []string{"version_events", "version_heads"} {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE model_id=$1", model); err != nil {
			return err
		}
	}
	return nil
}
