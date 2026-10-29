// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"strings"
	"testing"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
	"ifcviewer/server/internal/store"
)

func TestSourceHistoryMigratesLegacyBranchesAndPreservesMerge(t *testing.T) {
	s, _ := testService(t, "ifc")
	ctx := context.Background()
	r := editruntime.NewEmbedded()
	defer r.Close()
	legacy := "PARAMS = {'name':'legacy'}\n" + strings.Repeat("\n", 10) + "OTHER = 1\n"
	args := map[string]any{"model": testModel, "script": legacy, "version": "v1", "source": "human"}
	first, err := r.Handle(editruntime.Request{Root: s.Root, Op: "git.commit", Args: jsonBytes(args)})
	if err != nil {
		t.Fatal(err)
	}
	prefix := "models/" + testModel + "/scripts/v1"
	if err = r.PublishArtifacts(ctx, s.Root, "", []editruntime.ArtifactFile{{Path: prefix + ".py", Content: []byte(legacy)}, {Path: prefix + ".product", Content: fixtureIFC("legacy")}}); err != nil {
		t.Fatal(err)
	}
	old := invoke(t, s, "GET", "git", nil).(map[string]any)
	if old["commit"] != first.(map[string]string)["commit"] {
		t.Fatal("migration changed old commit")
	}
	revision := digest([]byte(legacy))
	invoke(t, s, "POST", "git/branches", map[string]any{"branch": "feature", "baseRevision": revision})
	feature := strings.Replace(legacy, "legacy", "feature", 1)
	invoke(t, s, "PUT", "script", map[string]any{"script": feature})
	if result := invoke(t, s, "POST", "script/save", nil).(map[string]any); result["version"] != "v2" {
		t.Fatal(result)
	}
	featureGit := invoke(t, s, "GET", "git", nil).(map[string]any)
	invoke(t, s, "POST", "git/checkout", map[string]any{"branch": "main", "baseRevision": digest([]byte(feature))})
	main := strings.Replace(legacy, "OTHER = 1", "OTHER = 2", 1)
	invoke(t, s, "PUT", "script", map[string]any{"script": main})
	if result := invoke(t, s, "POST", "script/save", nil).(map[string]any); result["version"] != "v3" {
		t.Fatal(result)
	}
	mainGit := invoke(t, s, "GET", "git", nil).(map[string]any)
	merged := invoke(t, s, "POST", "git/merge", map[string]any{"branch": "feature", "baseRevision": digest([]byte(main))}).(map[string]any)
	parents := merged["commits"].([]map[string]any)[0]["parents"].([]string)
	if len(parents) != 2 || parents[0] != mainGit["commit"] || parents[1] != featureGit["commit"] {
		t.Fatal("lost branch DAG", merged)
	}
	current := invoke(t, s, "GET", "script", nil).(map[string]any)["script"].(string)
	if !strings.Contains(current, "'feature'") || !strings.Contains(current, "OTHER = 2") {
		t.Fatal("merge did not rebuild combined source", current)
	}
	if err := store.NewStore(s.Root).Delete(testModel); err != nil {
		t.Fatal(err)
	}
	db, _ := artifact.Database(s.Root)
	var n int
	if err := db.QueryRow("SELECT (SELECT count(*) FROM version_heads)+(SELECT count(*) FROM version_events)").Scan(&n); err != nil || n != 0 {
		t.Fatal("deletion retained history references", n, err)
	}
}
