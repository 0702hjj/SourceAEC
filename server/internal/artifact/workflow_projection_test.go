// SPDX-License-Identifier: Apache-2.0
package artifact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowBlobsStayDatabaseOnlyOnRestore(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	root := t.TempDir()
	if err := Open(root); err != nil {
		t.Fatal(err)
	}
	defer Close(root)
	db, err := Database(root)
	if err != nil {
		t.Fatal(err)
	}
	key := "models/m_0123456789abcdef/workflows/blobs/" + strings.Repeat("a", 64)
	if _, err = db.Exec("INSERT INTO artifacts(path,content) VALUES($1,$2)", key, []byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	if err = Restore(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, key)); !os.IsNotExist(err) {
		t.Fatal("durable workflow blob projected onto disk", err)
	}
	var value []byte
	if err = db.QueryRow("SELECT content FROM artifacts WHERE path=$1", key).Scan(&value); err != nil || string(value) != "synthetic" {
		t.Fatal(string(value), err)
	}
}
