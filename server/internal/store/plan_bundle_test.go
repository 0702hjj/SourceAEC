// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"ifcviewer/server/internal/artifact"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanBundleVersionsWithRelativeDataDirectory(t *testing.T) {
	if os.Getenv("VIEWER_PG_DSN") == "" {
		t.Skip("VIEWER_PG_DSN not set")
	}
	t.Chdir(t.TempDir())
	root, err := filepath.Abs("workspace")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close(root) })
	store := NewPlanStore("workspace")
	id := "p_0123456789abcdef"
	for i := 0; i < 2; i++ {
		result, err := store.PutBundle(context.Background(), id, map[string][]byte{"plan.json": []byte(`{"version":1}`), "bim_supplement.json": []byte(`{"version":1}`)})
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && result["plan.json"] != "v2" {
			t.Fatal(result)
		}
	}
	history, err := store.ListHistory(id, "plan.json")
	if err != nil || len(history) != 1 {
		t.Fatal(history, err)
	}
}
