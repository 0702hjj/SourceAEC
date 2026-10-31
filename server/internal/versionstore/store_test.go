// SPDX-License-Identifier: Apache-2.0
package versionstore_test

import (
	"context"
	"os"
	"testing"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/versionstore"
)

func TestSourceHistoryTransactionAndImmutableReferences(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	ctx := context.Background()
	root := t.TempDir()
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	db, _ := artifact.Database(root)
	s := versionstore.New(db)
	model := "m_0000000000000001"
	if err := s.Initialize(ctx, model, versionstore.Head{Branch: "main", Ref: "legacy:v1", LastNumber: 1}); err != nil {
		t.Fatal(err)
	}
	id, _ := versionstore.NewRevision()
	prefix := "models/" + model + "/scripts/v2"
	e := versionstore.Event{Revision: id, Source: "ai", Version: "v2", ScriptKey: prefix + ".py", InputsKey: prefix + ".inputs.json", BuildKey: prefix + ".build.json", MapKey: prefix + ".map.json"}
	if _, err := s.Append(ctx, model, e); err == nil {
		t.Fatal("accepted publication without transaction")
	}
	if _, err := s.NextNumber(ctx, model); err == nil {
		t.Fatal("accepted version allocation without transaction")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	tstore := versionstore.New(tx)
	if n, err := tstore.NextNumber(ctx, model); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	saved, err := tstore.Append(ctx, model, e)
	if err != nil || saved.Parent != "legacy:v1" || saved.Branch != "main" || saved.AcceptedAt.IsZero() {
		t.Fatal(saved, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	head, err := s.Head(ctx, model)
	if err != nil || head.LastNumber != 1 || head.Ref != "legacy:v1" {
		t.Fatal("rollback changed authoritative head", head, err)
	}
	if _, err := s.Append(ctx, model, versionstore.Event{Revision: id, Source: "ai", ScriptKey: "models/" + model + "/current.py"}); err == nil {
		t.Fatal("accepted mutable or incomplete references")
	}
	if pending, err := s.Pending(ctx, model, 1); err != nil || len(pending) != 0 {
		t.Fatal("aborted record survived", pending, err)
	}
}
