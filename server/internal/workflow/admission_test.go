// SPDX-License-Identifier: Apache-2.0
package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ifcviewer/server/internal/workflow"
)

func TestWorkflowPendingAdmissionAndIdempotentRetry(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	first := create(t, db, creation("first"))
	for i := 1; i < workflow.MaxPending; i++ {
		create(t, db, creation(fmt.Sprintf("request-%d", i)))
	}
	err := transaction(t, db, func(s *workflow.Store) error { _, _, err := s.Create(ctx, creation("overflow")); return err })
	if !errors.Is(err, workflow.ErrCapacity) {
		t.Fatal("count admission not enforced", err)
	}
	err = transaction(t, db, func(s *workflow.Store) error {
		r, created, err := s.Create(ctx, creation("first"))
		if err == nil && (created || r.ID != first.ID) {
			t.Fatal("full capacity broke retry")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Cancel(ctx, model, first.ID); return err }); err != nil {
		t.Fatal(err)
	}
	create(t, db, creation("replacement"))
	rows, err := workflow.New(db).List(ctx, model, 2)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
}

func TestWorkflowInputByteAdmission(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	in := creation("bytes")
	in.Definition.InputBytes = 256 << 20
	var first workflow.Run
	// Seven maximum input sets plus manifests fit; eight exceed 2 GiB.
	for i := 0; i < 7; i++ {
		in.Key = fmt.Sprintf("bytes-%d", i)
		r := create(t, db, in)
		if i == 0 {
			first = r
		}
	}
	in.Key = "byte-overflow"
	if err := transaction(t, db, func(s *workflow.Store) error { _, _, err := s.Create(ctx, in); return err }); !errors.Is(err, workflow.ErrCapacity) {
		t.Fatal("byte admission not enforced", err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Cancel(ctx, model, first.ID); return err }); err != nil {
		t.Fatal(err)
	}
	create(t, db, in)
}

func TestWorkflowDefinitionAndRunRollbackTogether(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, created, err := workflow.New(tx).Create(ctx, creation("rollback"))
	if err != nil || !created {
		t.Fatal(r, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var definitions, runs int
	if err := db.QueryRow("SELECT count(*) FROM workflow_definitions").Scan(&definitions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM workflow_runs").Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if definitions != 0 || runs != 0 {
		t.Fatal("rolled-back creation retained rows", definitions, runs)
	}
}
