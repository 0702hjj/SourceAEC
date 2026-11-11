// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type trackedGitRead struct {
	queryer
	payloadReads int
}

func (t *trackedGitRead) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if strings.Contains(query, "SELECT bundle") || strings.Contains(query, "CASE WHEN") {
		t.payloadReads++
	}
	return t.queryer.QueryRowContext(ctx, query, args...)
}
func TestGitSnapshotRejectsBeforeReadingBundle(t *testing.T) {
	r, root := databaseRuntime(t)
	db, err := r.database(Request{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	model := "m_0000000000000001"
	if _, err := db.Exec("INSERT INTO repositories(model_id,bundle,branch) VALUES($1,$2,'main')", model, []byte("too large")); err != nil {
		t.Fatal(err)
	}
	old := gitResources
	gitResources = newGitBudget(1, 4)
	t.Cleanup(func() { gitResources = old })
	tracked := &trackedGitRead{queryer: db}
	snapshot, err := repositorySnapshot(context.Background(), tracked, model)
	if !errors.Is(err, ErrGitCapacity) || len(snapshot.Bundle) != 0 || tracked.payloadReads != 0 {
		t.Fatalf("payload read before admission: reads=%d bytes=%d err=%v", tracked.payloadReads, len(snapshot.Bundle), err)
	}
}
func TestGitJobRetainsBudgetUntilEveryOwnerCloses(t *testing.T) {
	budget := newGitBudget(1, 8)
	first, err := budget.acquire(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	second, err := first.retain()
	if err != nil {
		t.Fatal(err)
	}
	first.close()
	first.close()
	if _, err := first.retain(); err == nil {
		t.Fatal("closed view retained a live job")
	}
	if _, err := budget.acquire(context.Background(), 1); !errors.Is(err, ErrGitCapacity) {
		t.Fatalf("candidate lost its job: %v", err)
	}
	if err := second.grow(8); err != nil {
		t.Fatal(err)
	}
	if err := second.grow(9); !errors.Is(err, ErrGitCapacity) {
		t.Fatal(err)
	}
	second.close()
	second.close()
	next, err := budget.acquire(context.Background(), 8)
	if err != nil {
		t.Fatal(err)
	}
	next.close()
	if _, err := second.retain(); err == nil {
		t.Fatal("resurrected closed reservation")
	}
}

func TestGitPersistRejectsUnbudgetedOutput(t *testing.T) {
	r, root := databaseRuntime(t)
	db, err := r.database(Request{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	c, err := workspaceSnapshot(context.Background(), root, "m_0000000000000001", RepositorySnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.dispose()
	if _, err := commitCandidate(c, gitArgs{Script: "PARAMS = {}"}); err != nil {
		t.Fatal(err)
	}
	c.bundleLease.close()
	c.bundleLease, err = newGitBudget(1, 1).acquire(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	err = persist(db, c)
	var rows int
	if e := db.QueryRow("SELECT count(*) FROM repositories WHERE model_id=$1", c.model).Scan(&rows); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(err, ErrGitCapacity) || rows != 0 {
		t.Fatalf("output bypassed budget: rows=%d err=%v", rows, err)
	}
}
