// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
)

func TestSourceHistorySaveDoesNotRequireGitAndProjectionFailureIsRetryable(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'saved'}\n"})
	path := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	for range 2 {
		result := invoke(t, s, "POST", "script/save", nil).(map[string]any)
		ref, ok := result["historyRevision"].(string)
		if !ok || ref == "" || result["commit"] != nil {
			t.Fatal("save claimed a Git commit", result)
		}
	}
	if _, err := s.Do(context.Background(), "GET", "/models/"+testModel+"/git", nil); err == nil {
		t.Fatal("Git projection ignored missing executable")
	}
	product, err := artifact.ReadFile(filepath.Join(s.Root, "uploads", testModel+".ifc"))
	if err != nil || string(product) != string(fixtureIFC("saved")) {
		t.Fatal("projection failure changed saved product", err)
	}
	versions := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
	if len(versions) != 2 {
		t.Fatal("Git failure lost saved versions", versions)
	}
	t.Setenv("PATH", path)
	git := invoke(t, s, "GET", "git", nil).(map[string]any)
	if len(git["commits"].([]map[string]any)) != 2 {
		t.Fatal(git)
	}
}

func TestSourceHistoryBatchResumeAndProjectionDoesNotInvalidateExecution(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'saved'}\n"})
	for range 35 {
		invoke(t, s, "POST", "script/save", map[string]any{"source": "ai"})
	}
	r := editruntime.NewEmbedded()
	defer r.Close()
	o := &operation{service: s, runtime: r, model: testModel, ctx: context.Background()}
	func() {
		release, err := o.begin()
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		done, err := r.MaterializeSourceHistory(o.ctx, s.Root, o.tx, testModel)
		if err != nil || done {
			t.Fatal("expected an incomplete first batch", done, err)
		}
		if _, err = o.call("commit", nil); err != nil {
			t.Fatal(err)
		}
		o.tx = ""
	}()
	db, _ := artifact.Database(s.Root)
	var pending int
	if err := db.QueryRow("SELECT count(*) FROM version_events WHERE model_id=$1 AND git_commit IS NULL", testModel).Scan(&pending); err != nil || pending != 3 {
		t.Fatal("batch progress lost", pending, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Do(ctx, "GET", "/models/"+testModel+"/git", nil); err == nil {
		t.Fatal("ignored cancellation")
	}
	p := &pausedExecutor{make(chan struct{}), make(chan struct{})}
	other := New(s.Root, "ifc")
	other.Executor = p
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := other.Do(ctx, "POST", "/models/"+testModel+"/script/run", nil); finished <- err }()
	select {
	case <-p.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	git := invoke(t, s, "GET", "git", nil).(map[string]any)
	if len(git["commits"].([]map[string]any)) != 35 {
		t.Fatal("resume omitted or duplicated commits", git)
	}
	close(p.resume)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal("cache projection invalidated source execution", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
