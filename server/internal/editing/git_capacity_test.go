// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"testing"
	"time"

	"ifcviewer/server/internal/editruntime"
)

func TestGitCapacityReturns429AndRecoversAfterSnapshotRelease(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'synthetic'}\n"})
	invoke(t, s, "POST", "script/save", nil)
	invoke(t, s, "GET", "git", nil)
	r := editruntime.NewEmbedded()
	defer r.Close()
	var snapshots []editruntime.RepositorySnapshot
	t.Cleanup(func() {
		for i := range snapshots {
			snapshots[i].Close()
		}
	})
	for range 3 {
		snapshot, err := r.ReadRepositorySnapshot(context.Background(), s.Root, "", testModel)
		if err != nil {
			t.Fatal(err)
		}
		snapshots = append(snapshots, snapshot)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := s.Do(ctx, "GET", "/models/"+testModel+"/git", nil)
	var response *Error
	if !errors.As(err, &response) || response.Status != 429 {
		t.Fatalf("Git capacity not surfaced as 429: %v", err)
	}
	op := &operation{service: s, runtime: r, model: testModel, ctx: ctx}
	_, err = op.gitMutation(&staging{Cursor: -1}, "git/branches", jsonBytes(map[string]any{"branch": "feature"}))
	if !errors.As(err, &response) || response.Status != 429 {
		t.Fatalf("branch preparation masked capacity: %v", err)
	}
	for i := range snapshots {
		snapshots[i].Close()
	}
	state := invoke(t, s, "GET", "git", nil).(map[string]any)
	if state["commit"] == "" {
		t.Fatal("Git state did not recover", state)
	}
}
