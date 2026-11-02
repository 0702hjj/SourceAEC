// SPDX-License-Identifier: Apache-2.0
package workflow_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/store"
	"ifcviewer/server/internal/workflow"
)

func TestWorkflowModelDeletionFencesWorker(t *testing.T) {
	db, root := database(t)
	ctx := context.Background()
	st := store.NewStore(root)
	m, err := st.Create("synthetic.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	in := creation("deletion")
	in.Model = m.ID
	in.Definition.Model = m.ID
	in.Definition.Script = reference(m.ID)
	in.Definition.Inputs = reference(m.ID)
	in.Definition.Environment = reference(m.ID)
	r := create(t, db, in)
	owned := claim(t, db, "worker")
	if err := st.DeleteContext(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.New(db).Get(ctx, m.ID, r.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("model deletion retained workflow", err)
	}
	if err := workflow.New(db).Heartbeat(ctx, owned.Fence(), time.Minute); !errors.Is(err, workflow.ErrLeaseLost) {
		t.Fatal("deleted model worker still owns lease", err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM workflow_attempts WHERE run_id=$1", r.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("attempt audit not deleted", count, err)
	}
}

func TestWorkflowModelDeletionRollbackIsAtomic(t *testing.T) {
	db, root := database(t)
	ctx := context.Background()
	st := store.NewStore(root)
	m, err := st.Create("synthetic.ifc", 1, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	in := creation("rollback-delete")
	in.Model = m.ID
	in.Definition.Model = m.ID
	in.Definition.Script = reference(m.ID)
	in.Definition.Inputs = reference(m.ID)
	in.Definition.Environment = reference(m.ID)
	r := create(t, db, in)
	if _, err := db.Exec(`CREATE FUNCTION reject_workflow_delete() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'synthetic workflow delete rejection'; END $$;
 CREATE TRIGGER reject_workflow_delete BEFORE DELETE ON workflow_definitions FOR EACH ROW EXECUTE FUNCTION reject_workflow_delete()`); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteContext(ctx, m.ID); err == nil {
		t.Fatal("workflow deletion failure ignored")
	}
	if _, err := st.Get(m.ID); err != nil {
		t.Fatal("failed deletion lost model", err)
	}
	if _, err := workflow.New(db).Get(ctx, m.ID, r.ID); err != nil {
		t.Fatal("failed deletion lost workflow", err)
	}
}
