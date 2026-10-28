// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

const interruptedKey = "models/m_0000000000000001/scripts/v1.py"

func TestInterruptedWriterProcessHelper(t *testing.T) {
	root := os.Getenv("SOURCEAEC_INTERRUPTED_ROOT")
	if root == "" {
		return
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	r := NewEmbedded()
	defer r.Close()
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "lock", root, tx, "m_0000000000000001")
	invoke(t, r, "publish", root, tx, []fileEntry{{interruptedKey, []byte("uncommitted")}})
	path := filepath.Join(root, interruptedKey)
	if err := os.WriteFile(path, []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := r.transactions[tx].tx.QueryRow("SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("writer-ready:%d\n", pid)
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	// Simulate the worst ordering: a canceled process finishes a file rename
	// after another process acquired the SQL lock and committed newer state.
	if err := os.WriteFile(path, []byte("late canceled projection"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _ = r.Handle(Request{Root: root, Tx: tx, Op: "rollback"})
	fmt.Println("writer-restored")
}

func TestInterruptedProcessRollbackPreservesNewerCommit(t *testing.T) {
	r, root := databaseRuntime(t)
	if err := artifact.WriteFile(filepath.Join(root, interruptedKey), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInterruptedWriterProcessHelper$")
	cmd.Env = append(os.Environ(), "SOURCEAEC_INTERRUPTED_ROOT="+root)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(out)
	pid := 0
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "writer-ready:") {
			pid, err = strconv.Atoi(strings.TrimPrefix(scanner.Text(), "writer-ready:"))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if pid == 0 {
		t.Fatal("child did not enter write transaction")
	}
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "lock", root, tx, "m_0000000000000001")
	invoke(t, r, "publish", root, tx, []fileEntry{{interruptedKey, []byte("new committed")}})
	if err = os.WriteFile(filepath.Join(root, interruptedKey), []byte("new committed"), 0600); err != nil {
		t.Fatal(err)
	}
	invoke(t, r, "commit", root, tx, nil)
	if _, err = fmt.Fprintln(in, "resume"); err != nil {
		t.Fatal(err)
	}
	_ = in.Close()
	restored := false
	for scanner.Scan() {
		if scanner.Text() == "writer-restored" {
			restored = true
		}
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("interrupted writer did not recover")
	}
	data, err := os.ReadFile(filepath.Join(root, interruptedKey))
	if err != nil || string(data) != "new committed" {
		t.Fatal("old process restored stale bytes", string(data), err)
	}
	data, err = r.ReadStoredArtifact(ctx, root, "", interruptedKey)
	if err != nil || string(data) != "new committed" {
		t.Fatal("new SQL commit lost", string(data), err)
	}
}
