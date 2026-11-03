// SPDX-License-Identifier: Apache-2.0
package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ifcviewer/server/internal/workflow"
)

func TestWorkflowHeartbeatDoesNotWaitForPublicationLock(t *testing.T) {
	db, _ := database(t)
	create(t, db, creation("heartbeat-lock"))
	run := claim(t, db, "owner")
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = workflow.New(tx).Owned(context.Background(), run.Fence()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = workflow.New(db).Heartbeat(ctx, run.Fence(), time.Minute); !errors.Is(err, workflow.ErrLeaseBusy) {
		t.Fatal("heartbeat blocked behind its own publication transaction", err)
	}
	// The owning transaction can extend its own lease for bounded large writes.
	if err = workflow.New(tx).Heartbeat(context.Background(), run.Fence(), 3*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = workflow.New(db).Heartbeat(context.Background(), run.Fence(), time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE workflow_runs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", run.ID); err != nil {
		t.Fatal(err)
	}
	if err = workflow.New(db).Heartbeat(context.Background(), run.Fence(), time.Minute); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("expired unlocked owner renewed", err)
	}
}
