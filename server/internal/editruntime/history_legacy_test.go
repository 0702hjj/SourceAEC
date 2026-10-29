// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"encoding/json"
	"ifcviewer/server/internal/artifact"
	"testing"
	"time"
)

func TestSourceHistoryLegacyImportKeepsArchivedInputs(t *testing.T) {
	r, root := databaseRuntime(t)
	model := "m_0000000000000001"
	prefix := "models/" + model + "/scripts/v1"
	invoke(t, r, "publish", root, "", []fileEntry{{prefix + ".py", []byte("PARAMS = {}")}, {prefix + ".inputs.json", []byte(`{"plans/p_synthetic/plan.json":"saved input"}`)}, {prefix + ".build.json", []byte(`{"identity":{"environment":"legacy"}}`)}, {prefix + ".map.json", []byte(`{}`)}})
	invoke(t, r, "git.import", root, "", gitArgs{Model: model})
	db, err := r.database(Request{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	var count, empty int
	if err := db.QueryRow("SELECT count(*) FILTER (WHERE model_id=$1),count(*) FILTER (WHERE model_id='') FROM repositories", model).Scan(&count, &empty); err != nil || count != 1 || empty != 0 {
		t.Fatal("legacy bundle stored under wrong model", count, empty, err)
	}
	prepared := gitCall(t, r, root, "prepare", "", gitArgs{Action: "create", Branch: "feature"}).(map[string]string)
	inputs := gitCall(t, r, root, "inputs", "", gitArgs{Candidate: prepared["candidate"]}).(map[string]any)
	if inputs["inputs"].(map[string]any)["plans/p_synthetic/plan.json"] != "saved input" {
		t.Fatal("legacy import lost archived inputs", inputs)
	}
	value, err := archiveJSONValue([]byte(`{"integer":9007199254740993}`))
	if err != nil || value.(map[string]any)["integer"].(json.Number).String() != "9007199254740993" {
		t.Fatal(value, err)
	}
	if _, err := archiveJSONValue([]byte(`{} {}`)); err == nil {
		t.Fatal("accepted trailing archive document")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ReadRepositorySnapshot(ctx, root, "", model); err == nil {
		t.Fatal("snapshot ignored cancellation")
	}
}

func TestSourceHistoryGitStateUsesNoCheckedOutConnection(t *testing.T) {
	r, root := databaseRuntime(t)
	gitCall(t, r, root, "commit", "", gitArgs{Script: "PARAMS = {}", Version: "v1"})
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	token := invoke(t, r, "begin_read", root, "", nil).(string)
	snapshot, err := r.ReadRepositorySnapshot(ctx, root, token, "m_0000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, r, "commit", root, token, nil)
	held, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	state, err := r.GitStateSnapshot(ctx, root, "m_0000000000000001", snapshot)
	if err != nil || state["commit"] == "" {
		t.Fatal("Git state waited for a database connection", state, err)
	}
}
