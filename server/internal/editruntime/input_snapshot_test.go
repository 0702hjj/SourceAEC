// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

const inputModel = "m_0000000000000001"

func inputFixture(t *testing.T) (*Runtime, string) {
	t.Helper()
	r, root := databaseRuntime(t)
	for key, content := range map[string]string{
		"models/" + inputModel + "/model.json": `{"projectId":"p_inputs"}`,
		"plans/p_inputs/plan.json":             `{"name":"committed"}`,
		"skill-work/p_inputs/drawing.dxf":      "committed drawing",
		"plans/p_inputs_other/plan.json":       "another project",
	} {
		if err := artifact.WriteFile(filepath.Join(root, key), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return r, root
}

func TestBuildCollectionIgnoresMissingAndDirtyProjections(t *testing.T) {
	r, root := inputFixture(t)
	if err := os.RemoveAll(filepath.Join(root, "plans")); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "skill-work/p_inputs/drawing.dxf"), "uncommitted drawing")
	inputs := invoke(t, r, "build.collect", root, "", buildArgs{Model: inputModel}).(map[string]string)
	if inputs["plans/p_inputs/plan.json"] != base64.StdEncoding.EncodeToString([]byte(`{"name":"committed"}`)) || inputs["skill-work/p_inputs/drawing.dxf"] != base64.StdEncoding.EncodeToString([]byte("committed drawing")) {
		t.Fatalf("collection trusted file projections: %v", inputs)
	}
}

func TestBuildCollectionCannotSeeAnotherTransactionsProject(t *testing.T) {
	r, root := inputFixture(t)
	tx := invoke(t, r, "begin", root, "", nil).(string)
	key := "models/" + inputModel + "/model.json"
	files := []fileEntry{{key, []byte(`{"projectId":"p_uncommitted"}`)}, {"plans/p_uncommitted/plan.json", []byte("uncommitted")}}
	invoke(t, r, "publish", root, tx, files)
	for _, file := range files {
		put(t, filepath.Join(root, file.Path), string(file.Content))
	}
	inputs := invoke(t, r, "build.collect", root, "", buildArgs{Model: inputModel}).(map[string]string)
	if len(inputs) != 2 || inputs["plans/p_inputs/plan.json"] == "" {
		t.Fatalf("uncommitted project/input leaked through projections: %v", inputs)
	}
	invoke(t, r, "rollback", root, tx, nil)
}

func TestBuildCollectionUsesCallerSnapshotWithoutAnotherConnection(t *testing.T) {
	r, root := inputFixture(t)
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	tx := invoke(t, r, "begin_read", root, "", nil).(string)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req := request("build.collect", root, tx, buildArgs{Model: inputModel})
	req.Context = ctx
	value, err := r.Handle(req)
	if err != nil || len(value.(map[string]string)) != 2 {
		t.Fatal("collection required a second connection", value, err)
	}
	invoke(t, r, "commit", root, tx, nil)
}

func TestBuildCollectionHonorsCanceledContext(t *testing.T) {
	r, root := inputFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := request("build.collect", root, "", buildArgs{Model: inputModel})
	req.Context = ctx
	if _, err := r.Handle(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("collection ignored cancellation: %v", err)
	}
}

func TestBuildCollectionRetainsCallerSnapshot(t *testing.T) {
	r, root := inputFixture(t)
	tx := invoke(t, r, "begin_read", root, "", nil).(string)
	before := invoke(t, r, "build.collect", root, tx, buildArgs{Model: inputModel}).(map[string]string)
	writer := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "publish", root, writer, []fileEntry{
		{"models/" + inputModel + "/model.json", []byte(`{"projectId":"p_new"}`)},
		{"plans/p_new/plan.json", []byte("new committed input")},
	})
	invoke(t, r, "commit", root, writer, nil)
	inside := invoke(t, r, "build.collect", root, tx, buildArgs{Model: inputModel}).(map[string]string)
	outside := invoke(t, r, "build.collect", root, "", buildArgs{Model: inputModel}).(map[string]string)
	if inside["plans/p_inputs/plan.json"] != before["plans/p_inputs/plan.json"] || outside["plans/p_new/plan.json"] != base64.StdEncoding.EncodeToString([]byte("new committed input")) || len(outside) != 1 {
		t.Fatal("collection escaped caller snapshot", before, inside, outside)
	}
	invoke(t, r, "commit", root, tx, nil)
}

func TestBuildInputValidationPreservesIsolationAndLimits(t *testing.T) {
	for _, key := range []string{"plans/p_other/plan.json", "plans/p_inputs/../p_other/plan.json", "plans/p_inputs/back\\slash.json"} {
		if _, err := validateInputKey(key, "p_inputs"); err == nil {
			t.Fatalf("unsafe input accepted: %s", key)
		}
	}
	for _, key := range []string{"plans/p_inputs/plan_history/v1.json", "skill-work/p_inputs/ignored.bin"} {
		if include, err := validateInputKey(key, "p_inputs"); err != nil || include {
			t.Fatalf("excluded input accepted: %s %v", key, err)
		}
	}
	if err := validateInputSize(buildInputLimit, buildInputsLimit); err != nil {
		t.Fatal("exact limits rejected", err)
	}
	for _, limits := range [][2]int64{{buildInputLimit + 1, buildInputLimit + 1}, {1, buildInputsLimit + 1}} {
		if err := validateInputSize(limits[0], limits[1]); err == nil {
			t.Fatalf("oversized inputs accepted: %v", limits)
		}
	}
}

func TestBuildCollectionRejectsInvalidOwnershipMetadata(t *testing.T) {
	for _, metadata := range []string{"not-json", `{"projectId":42}`} {
		r, root := inputFixture(t)
		if err := artifact.WriteFile(filepath.Join(root, "models", inputModel, "model.json"), []byte(metadata), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Handle(request("build.collect", root, "", buildArgs{Model: inputModel})); err == nil {
			t.Fatal("invalid ownership metadata was silently accepted")
		}
	}
}
