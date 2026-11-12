// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"ifcviewer/server/internal/workflow"
	"path/filepath"
	"testing"

	"ifcviewer/server/internal/artifact"
)

func TestSourceHistoryNativeConflictIs409AndAtomic(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'before'}\n"})
	invoke(t, s, "POST", "script/save", nil)
	invoke(t, s, "PUT", "script", map[string]any{"params": map[string]any{"name": "after"}})
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE FUNCTION reject_head_cas() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.ref <> OLD.ref THEN RETURN NULL; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_head_cas BEFORE UPDATE ON version_heads FOR EACH ROW EXECUTE FUNCTION reject_head_cas()`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", nil)
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 409 {
		t.Fatal("native history conflict not translated", err)
	}
	var versions, events int
	if err = db.QueryRow(`SELECT last_number FROM version_heads WHERE model_id=$1`, testModel).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM version_events WHERE model_id=$1`, testModel).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if versions != 1 || events != 1 {
		t.Fatal("conflicting publication changed history", versions, events)
	}
	if _, err = db.Exec(`DROP TRIGGER reject_head_cas ON version_heads; DROP FUNCTION reject_head_cas()`); err != nil {
		t.Fatal(err)
	}
	saved := invoke(t, s, "POST", "script/save", nil).(map[string]any)
	if saved["version"] != "v2" {
		t.Fatal("failed save consumed version", saved)
	}
}

func TestWorkflowHistoryConflictRollsBackBeforeSuperseding(t *testing.T) {
	w, id, _, request := workflowFixture(t, "dxf")
	ctx := context.Background()
	run, _, err := w.Submit(ctx, id, request)
	if err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	before, err := artifact.ReadFile(filepath.Join(w.root, "uploads", id+".dxf"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.db.Exec(`CREATE FUNCTION reject_head_cas() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.ref <> OLD.ref THEN RETURN NULL; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_head_cas BEFORE UPDATE ON version_heads FOR EACH ROW EXECUTE FUNCTION reject_head_cas()`); err != nil {
		t.Fatal(err)
	}
	workflowStep(t, w)
	v, err := w.Get(ctx, id, run.ID)
	if err != nil || v.State != workflow.Superseded {
		t.Fatal(v, err)
	}
	after, err := artifact.ReadFile(filepath.Join(w.root, "uploads", id+".dxf"))
	var count, number int
	if err != nil || string(before) != string(after) {
		t.Fatal("superseded publication changed product", err)
	}
	if err = w.db.QueryRow(`SELECT count(*) FROM version_events WHERE model_id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err = w.db.QueryRow(`SELECT last_number FROM version_heads WHERE model_id=$1`, id).Scan(&number); err != nil {
		t.Fatal(err)
	}
	if count != 0 || number != 0 {
		t.Fatal("superseded publication committed partial history", count, number)
	}
}
