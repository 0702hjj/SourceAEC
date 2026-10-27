// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
)

func TestScriptReadProceedsDuringExecution(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'committed'}\n"})
	startBlockedRun(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, path := range []string{"script", "script/params"} {
		if _, err := s.Do(ctx, "GET", "/models/"+testModel+"/"+path, nil); err != nil {
			t.Fatalf("committed read blocked behind geometry: %v", err)
		}
	}
}

func TestScriptReadCannotSeeUncommittedProjection(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "first-stage", true: "existing-stage"}[existing], func(t *testing.T) {
			s, _ := testService(t, "ifc")
			if existing {
				invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'before'}\n"})
			}
			o := &operation{service: s, runtime: editruntime.NewEmbedded(), model: testModel, ctx: context.Background()}
			defer o.runtime.Close()
			release, err := o.begin()
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			staging, err := o.loadStaging()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = o.stage(staging, jsonBytes(map[string]any{"script": "PARAMS = {'name':'after'}\n"})); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			value, err := s.Do(ctx, "GET", "/models/"+testModel+"/script", nil)
			if existing {
				if err != nil || value.(map[string]any)["script"] != "PARAMS = {'name':'before'}\n" {
					t.Fatal("dirty staging read", value, err)
				}
			} else {
				var domain *Error
				if !errors.As(err, &domain) || domain.Status != 404 {
					t.Fatal("uncommitted first stage visible", value, err)
				}
			}
			if _, err = o.call("commit", nil); err != nil {
				t.Fatal(err)
			}
			value, err = s.Do(ctx, "GET", "/models/"+testModel+"/script", nil)
			if err != nil || value.(map[string]any)["script"] != "PARAMS = {'name':'after'}\n" {
				t.Fatal(value, err)
			}
		})
	}
}

func TestScriptReadFallsBackToCommittedVersionWithoutProjections(t *testing.T) {
	s, _ := testService(t, "ifc")
	source := "PARAMS = {'name':'saved'}\n"
	invoke(t, s, "PUT", "script", map[string]any{"script": source})
	invoke(t, s, "POST", "script/save", nil)
	db, err := artifact.Database(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DELETE FROM artifacts WHERE path=$1", "models/"+testModel+"/script_staging.json"); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(filepath.Join(s.Root, "models", testModel)); err != nil {
		t.Fatal(err)
	}
	value := invoke(t, s, "GET", "script", nil).(map[string]any)
	if value["script"] != source || value["staged"] != 0 {
		t.Fatalf("committed fallback depends on projections: %v", value)
	}
}

func TestScriptSnapshotRetainsOneCommittedRevision(t *testing.T) {
	s, _ := testService(t, "ifc")
	before, after := "PARAMS = {'name':'before'}\n", "PARAMS = {'name':'after'}\n"
	invoke(t, s, "PUT", "script", map[string]any{"script": before})
	o := &operation{service: s, runtime: editruntime.NewEmbedded(), model: testModel, ctx: context.Background(), readOnly: true}
	defer o.runtime.Close()
	release, err := o.begin()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err = o.dispatch("GET", "script", nil); err != nil {
		t.Fatal(err)
	}
	invoke(t, s, "PUT", "script", map[string]any{"script": after})
	value, err := o.dispatch("GET", "script", nil)
	if err != nil || value.(map[string]any)["script"] != before {
		t.Fatal("snapshot mixed committed revisions", value, err)
	}
	params, err := o.dispatch("GET", "script/params", nil)
	if err != nil || params.(map[string]any)["params"].(map[string]any)["name"] != "before" {
		t.Fatal("params escaped snapshot", params, err)
	}
}
