// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
)

type changedGitRead struct {
	queryer
	before       func()
	payloadReads int
}

func (t *changedGitRead) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if strings.Contains(query, "CASE WHEN") {
		t.payloadReads++
		t.before()
	}
	return t.queryer.QueryRowContext(ctx, query, args...)
}
func TestGitSnapshotLengthRaceDoesNotReadUnchargedBytes(t *testing.T) {
	for _, action := range []string{"grow", "delete", "cancel"} {
		t.Run(action, func(t *testing.T) {
			r, root := databaseRuntime(t)
			db, err := r.database(Request{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			model := "m_0000000000000001"
			if _, err := db.Exec("INSERT INTO repositories(model_id,bundle,branch) VALUES($1,$2,'main')", model, []byte("1234")); err != nil {
				t.Fatal(err)
			}
			old := gitResources
			budget := newGitBudget(1, 4)
			gitResources = budget
			t.Cleanup(func() { gitResources = old })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tracked := &changedGitRead{queryer: db, before: func() {
				var err error
				switch action {
				case "grow":
					_, err = db.Exec("UPDATE repositories SET bundle=$1 WHERE model_id=$2", []byte("123456789"), model)
				case "delete":
					_, err = db.Exec("DELETE FROM repositories WHERE model_id=$1", model)
				case "cancel":
					cancel()
				}
				if err != nil {
					t.Fatal(err)
				}
			}}
			snapshot, err := repositorySnapshot(ctx, tracked, model)
			if len(snapshot.Bundle) != 0 || tracked.payloadReads != 1 {
				t.Fatalf("payload escaped guard: %d %+v", tracked.payloadReads, snapshot)
			}
			if action == "grow" && !errors.Is(err, ErrGitCapacity) || action == "cancel" && !errors.Is(err, context.Canceled) || action == "delete" && err != nil {
				t.Fatal(action, err)
			}
			next, err := budget.acquire(context.Background(), 4)
			if err != nil {
				t.Fatal("reservation leaked", err)
			}
			next.close()
		})
	}
}

func TestGitWorkspaceRetainsCapturedSnapshotReservation(t *testing.T) {
	r, root := databaseRuntime(t)
	gitCall(t, r, root, "commit", "", gitArgs{Script: "PARAMS = {}"})
	db, err := r.database(Request{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	old := gitResources
	budget := newGitBudget(1, 1<<20)
	gitResources = budget
	t.Cleanup(func() { gitResources = old })
	snapshot, err := repositorySnapshot(context.Background(), db, "m_0000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	c, err := workspaceSnapshot(context.Background(), root, "m_0000000000000001", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer c.dispose()
	if other, err := workspaceSnapshot(context.Background(), root, "m_0000000000000001", snapshot); !errors.Is(err, ErrGitCapacity) {
		if other != nil {
			other.dispose()
		}
		t.Fatalf("one snapshot admitted concurrent workspaces: %v", err)
	}
	snapshot.Close()
	if _, err := budget.acquire(context.Background(), 1); !errors.Is(err, ErrGitCapacity) {
		t.Fatal("workspace lost accounting", err)
	}
	if err := c.dispose(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.directory); !os.IsNotExist(err) {
		t.Fatal("workspace retained", err)
	}
	next, err := budget.acquire(context.Background(), 1<<20)
	if err != nil {
		t.Fatal("cleanup leaked reservation", err)
	}
	next.close()
}

func TestGitRuntimeCloseReleasesCapturedSnapshot(t *testing.T) {
	r, root := databaseRuntime(t)
	db, err := r.database(Request{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	model := "m_0000000000000001"
	if _, err := db.Exec("INSERT INTO repositories(model_id,bundle,branch) VALUES($1,$2,'main')", model, []byte("1234")); err != nil {
		t.Fatal(err)
	}
	old := gitResources
	budget := newGitBudget(1, 4)
	gitResources = budget
	t.Cleanup(func() { gitResources = old })
	if err := r.CaptureRepository(context.Background(), root, "", model); err != nil {
		t.Fatal(err)
	}
	r.Close()
	r.Close()
	next, err := budget.acquire(context.Background(), 4)
	if err != nil {
		t.Fatal("runtime retained captured bytes", err)
	}
	next.close()
}

func TestGitCandidateStateReusesItsOnlyWorkReservation(t *testing.T) {
	r, root := databaseRuntime(t)
	gitCall(t, r, root, "commit", "", gitArgs{Script: "PARAMS = {}"})
	old := gitResources
	gitResources = newGitBudget(1, 1<<20)
	t.Cleanup(func() { gitResources = old })
	prepared := gitCall(t, r, root, "prepare", "", gitArgs{Action: "create", Branch: "feature"}).(map[string]string)
	id := prepared["candidate"]
	gitCall(t, r, root, "persist", "", gitArgs{Candidate: id})
	state, err := r.Handle(request("git.candidate_state", root, "", gitArgs{Model: "m_0000000000000001", Candidate: id}))
	if err != nil {
		t.Fatal("candidate state required another Git job", err)
	}
	if state.(map[string]any)["branch"] != "feature" {
		t.Fatal(state)
	}
	gitCall(t, r, root, "discard", "", gitArgs{Candidate: id})
	state = gitCall(t, r, root, "state", "", gitArgs{})
	if state.(map[string]any)["branch"] != "feature" {
		t.Fatal(state)
	}
}
