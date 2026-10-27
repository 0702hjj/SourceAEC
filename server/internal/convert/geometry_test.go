// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/store"
)

func TestRealConversionPublishesCompleteGeneration(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_CONVERTER") != "1" {
		t.Skip("set SOURCEAEC_TEST_CONVERTER=1 with converter dependencies installed")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	converter, err := filepath.Abs("../../../converter")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(converter, "test/fixtures/wall-with-opening-and-window.ifc"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewStore(t.TempDir())
	model, err := st.Create("synthetic.ifc", int64(len(data)), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	q := testQueue(t, st, ExecRunner{NodeBin: node, Script: filepath.Join(converter, "convert.js")}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q.Start(ctx)
	if !q.Enqueue(model.ID) {
		t.Fatal("conversion was not admitted")
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := st.Get(model.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == "failed" {
			t.Fatal(current.Error)
		}
		if current.Status == "ready" {
			for _, name := range []string{"model.xkt", "metadata.json"} {
				path, err := st.ViewerPath(ctx, model.ID, name, current.PreviewGeneration)
				if err != nil {
					t.Fatal(err)
				}
				content, err := os.ReadFile(path)
				if err != nil || len(content) == 0 {
					t.Fatal("empty converted artifact", err)
				}
				if name == "metadata.json" && !json.Valid(content) {
					t.Fatal("invalid converted metadata")
				}
			}
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("conversion timed out")
		}
	}
}
