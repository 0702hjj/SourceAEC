// SPDX-License-Identifier: Apache-2.0
package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ifcviewer/server/internal/workflow"
)

func TestWorkflowCaptureReservations(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	in := creation("capture")
	var ticket workflow.Reservation
	reserve := func(key string) error {
		return transaction(t, db, func(s *workflow.Store) error {
			var err error
			ticket, _, err = s.Reserve(ctx, model, key, in.Intent)
			return err
		})
	}
	if err := reserve(in.Key); err != nil {
		t.Fatal(err)
	}
	original := ticket
	if err := reserve(in.Key); !errors.Is(err, workflow.ErrCapacity) {
		t.Fatal("parallel capture admitted", err)
	}
	changed := in.Intent
	changed.Note = "changed"
	if err := transaction(t, db, func(s *workflow.Store) error { _, _, err := s.Reserve(ctx, model, in.Key, changed); return err }); !errors.Is(err, workflow.ErrConflict) {
		t.Fatal(err)
	}
	for i := 1; i < 7; i++ {
		if err := reserve(fmt.Sprint("capture-", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := reserve("full"); !errors.Is(err, workflow.ErrCapacity) {
		t.Fatal("capture byte capacity bypassed", err)
	}
	if _, err := db.Exec("UPDATE workflow_reservations SET expires_at=clock_timestamp()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if err := reserve(in.Key); err != nil {
		t.Fatal(err)
	}
	fresh := ticket
	if err := transaction(t, db, func(s *workflow.Store) error { _, _, err := s.CreateReserved(ctx, in, original); return err }); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("expired capture accepted", err)
	}
	var run workflow.Run
	if err := transaction(t, db, func(s *workflow.Store) error {
		var err error
		run, _, err = s.CreateReserved(ctx, in, fresh)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error {
		_, prior, err := s.Reserve(ctx, model, in.Key, in.Intent)
		if err == nil && (prior == nil || prior.ID != run.ID) {
			t.Fatal("duplicate did not return committed run")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := reserve("delete-me"); err != nil {
		t.Fatal(err)
	}
	if err := transaction(t, db, func(s *workflow.Store) error { return s.DeleteModel(ctx, model) }); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM workflow_reservations").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestWorkflowResumeHonorsPendingCapacity(t *testing.T) {
	for _, mode := range []string{"count", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			db, _ := database(t)
			ctx := context.Background()
			input := creation("resume-capacity")
			count := workflow.MaxPending
			if mode == "bytes" {
				input.Definition.InputBytes = 256 << 20
				count = 7
			}
			canceled := create(t, db, input)
			if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Cancel(ctx, model, canceled.ID); return err }); err != nil {
				t.Fatal(err)
			}
			var first workflow.Run
			for i := 0; i < count; i++ {
				input.Key = fmt.Sprint("pending-", i)
				row := create(t, db, input)
				if i == 0 {
					first = row
				}
			}
			if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Resume(ctx, model, canceled.ID); return err }); !errors.Is(err, workflow.ErrCapacity) {
				t.Fatal("resume bypassed pending capacity", err)
			}
			if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Cancel(ctx, model, first.ID); return err }); err != nil {
				t.Fatal(err)
			}
			if err := transaction(t, db, func(s *workflow.Store) error { _, err := s.Resume(ctx, model, canceled.ID); return err }); err != nil {
				t.Fatal("released capacity not reusable", err)
			}
		})
	}
}
