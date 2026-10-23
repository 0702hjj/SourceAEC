// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package editruntime

import (
	"encoding/base64"
	"encoding/json"
	"ifcviewer/server/internal/artifact"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func request(op, root, tx string, args any) Request {
	return Request{Op: op, Root: root, Tx: tx, Args: canonical(args)}
}
func invoke(t *testing.T, r *Runtime, op, root, tx string, args any) any {
	t.Helper()
	value, err := r.Handle(request(op, root, tx, args))
	if err != nil {
		t.Fatal(op, err)
	}
	return value
}
func databaseRuntime(t *testing.T) (*Runtime, string) {
	t.Helper()
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	r := New()
	t.Cleanup(r.Close)
	root := t.TempDir()
	t.Cleanup(func() { artifact.Close(root) })
	invoke(t, r, "initialize", root, "", nil)
	return r, root
}
func TestCheckpointRollbackAndCommit(t *testing.T) {
	r, root := databaseRuntime(t)
	key := "models/m_0000000000000001/scripts/v1.py"
	path := filepath.Join(root, key)
	invoke(t, r, "publish", root, "", []fileEntry{{key, []byte("old")}})
	invoke(t, r, "initialize", root, "", nil)
	tx := invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "publish", root, tx, []fileEntry{{key, []byte("new")}, {"uploads/model.ifc", []byte("new IFC")}})
	if err := os.WriteFile(path, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	product := filepath.Join(root, "uploads/model.ifc")
	os.MkdirAll(filepath.Dir(product), 0755)
	os.WriteFile(product, []byte("new IFC"), 0644)
	invoke(t, r, "rollback", root, tx, nil)
	if got := string(invoke(t, r, "read", root, "", key).([]byte)); got != "old" {
		t.Fatal(got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "old" {
		t.Fatal(string(data))
	}
	if _, err := os.Stat(product); !os.IsNotExist(err) {
		t.Fatal("new projection survived rollback", err)
	}
	tx = invoke(t, r, "begin", root, "", nil).(string)
	invoke(t, r, "publish", root, tx, []fileEntry{{key, []byte("committed")}})
	invoke(t, r, "commit", root, tx, nil)
	os.Remove(path)
	invoke(t, r, "initialize", root, "", nil)
	data, _ = os.ReadFile(path)
	if string(data) != "committed" {
		t.Fatal(string(data))
	}
}
func TestInvalidBatchIsAtomicAndSymlinksRejected(t *testing.T) {
	r, root := databaseRuntime(t)
	_, err := r.Handle(request("publish", root, "", []fileEntry{{"models/valid.py", []byte("x")}, {"../escape", []byte("bad")}}))
	if err == nil {
		t.Fatal("accepted traversal")
	}
	db, _ := r.database(request("", root, "", nil))
	var count int
	db.QueryRow("SELECT count(*) FROM artifacts").Scan(&count)
	if count != 0 {
		t.Fatal("partial batch committed")
	}
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "link"))
	_, err = r.Handle(request("publish", root, "", []fileEntry{{"link/file.py", []byte("bad")}}))
	if err == nil {
		t.Fatal("accepted symlink")
	}
}
func TestDurabilityPolicyAndBuildInputs(t *testing.T) {
	r := New()
	defer r.Close()
	for _, path := range []string{"models/x/model.xkt", "models/x/versions/v1.ifc", "models/x/ifc_cache/v1.ifc", "models/x/scripts/a.tmp", "models/x/.git/config"} {
		if invoke(t, r, "durable", "", "", path).(bool) {
			t.Fatal(path)
		}
	}
	root := t.TempDir()
	inputs := map[string]string{"plans/p_example/plan.json": base64.StdEncoding.EncodeToString([]byte("{}"))}
	target, err := projectInputs(root, inputs)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "plans/p_example/plan.json"))
	if string(data) != "{}" {
		t.Fatal(string(data))
	}
	_, err = projectInputs(t.TempDir(), map[string]string{"../escape": "eA=="})
	if err == nil || !strings.Contains(err.Error(), "invalid build input path") {
		t.Fatal(err)
	}
}
func TestStdioProtocol(t *testing.T) {
	input := strings.NewReader("{bad json}\n" + string(canonical(request("ping", "", "", nil))) + "\n")
	var out strings.Builder
	if err := Serve(input, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatal(out.String())
	}
	var response Response
	if err := json.Unmarshal([]byte(lines[1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != "" {
		t.Fatal(response.Error)
	}
}

func TestLateCaptureRestoresDatabaseOriginal(t *testing.T) {
	r, root := databaseRuntime(t)
	key := "models/m_0000000000000001/current.build.json"
	invoke(t, r, "publish", root, "", []fileEntry{{key, []byte("original")}})
	invoke(t, r, "initialize", root, "", nil)
	tx := invoke(t, r, "begin", root, "", nil).(string)
	// Streaming tools can overwrite projections before asking Go to capture them.
	if err := os.WriteFile(filepath.Join(root, key), []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, r, "publish", root, tx, []fileEntry{{key, []byte("replacement")}})
	invoke(t, r, "rollback", root, tx, nil)
	data, _ := os.ReadFile(filepath.Join(root, key))
	if string(data) != "original" {
		t.Fatal(string(data))
	}
}
