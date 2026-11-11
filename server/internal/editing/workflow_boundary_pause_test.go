// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"database/sql"
	"fmt"
	"hash/crc32"
	"testing"
	"time"
)

// A trigger waits on a parent-owned advisory lock. pg_locks proves that the
// child reached the exact SQL boundary before SIGKILL; no production hook or
// guessed delay is used. This is installed only in a synthetic test schema.
type workflowBoundaryPause struct {
	db         *sql.DB
	connection *sql.Conn
	table      string
	key        int64
	backend    int
	closed     bool
}

func installWorkflowBoundaryPause(t *testing.T, db *sql.DB, runID, table, timing, condition string) *workflowBoundaryPause {
	t.Helper()
	pause := &workflowBoundaryPause{db: db, table: table, key: int64(crc32.ChecksumIEEE([]byte(runID)) & 0x7fffffff)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	pause.connection, err = db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pause.connection.ExecContext(ctx, "SELECT pg_advisory_lock(170702,$1::integer)", pause.key); err != nil {
		pause.connection.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { pause.stop(t) })
	function := fmt.Sprintf(`CREATE FUNCTION workflow_boundary_pause() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM pg_advisory_xact_lock(170702,%d); RETURN NEW; END $$`, pause.key)
	if _, err = db.ExecContext(ctx, function); err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf("CREATE TRIGGER workflow_boundary_gate %s ON %s FOR EACH ROW WHEN (%s) EXECUTE FUNCTION workflow_boundary_pause()", timing, table, condition)
	if _, err = db.ExecContext(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	return pause
}
func (p *workflowBoundaryPause) await(t *testing.T, child *boundaryChild) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		err := p.db.QueryRowContext(ctx, `SELECT pid FROM pg_locks WHERE locktype='advisory'
 AND classid=170702::oid AND objid=$1::oid AND NOT granted`, p.key).Scan(&p.backend)
		if err == nil {
			return
		}
		if err != sql.ErrNoRows {
			child.kill()
			t.Fatal(err)
		}
		select {
		case ready := <-child.ready:
			if !ready {
				child.kill()
				t.Fatal("worker exited before SQL boundary", child.stderr.String())
			}
		case <-ctx.Done():
			child.kill()
			t.Fatal("SQL boundary deadline")
		case <-ticker.C:
		}
	}
}
func (p *workflowBoundaryPause) stop(t *testing.T) {
	t.Helper()
	if p.closed {
		return
	}
	p.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := p.connection.ExecContext(ctx, "SELECT pg_advisory_unlock(170702,$1::integer)", p.key); err != nil {
		t.Error(err)
	}
	if err := p.connection.Close(); err != nil {
		t.Error(err)
	}
	// A dead client can leave its backend cleaning up briefly. Wait for that
	// actual backend to exit before changing lease state or checking rollback.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for p.backend != 0 {
		var present bool
		if err := p.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1)", p.backend).Scan(&present); err != nil {
			t.Error(err)
			return
		}
		if !present {
			break
		}
		select {
		case <-ctx.Done():
			t.Error("killed worker backend did not exit")
			return
		case <-ticker.C:
		}
	}
	if _, err := p.db.ExecContext(ctx, "DROP TRIGGER IF EXISTS workflow_boundary_gate ON "+p.table); err != nil {
		t.Error(err)
	}
	if _, err := p.db.ExecContext(ctx, "DROP FUNCTION IF EXISTS workflow_boundary_pause()"); err != nil {
		t.Error(err)
	}
}
