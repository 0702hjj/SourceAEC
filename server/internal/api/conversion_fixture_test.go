// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/store"
)

func testConversionQueue(t *testing.T, st *store.Store, runner convert.Runner, workers int) *convert.Queue {
	t.Helper()
	q := convert.NewQueue(st, runner, workers)
	t.Cleanup(func() {
		q.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if !q.WaitIdle(ctx) {
			t.Error("conversion queue did not stop")
		}
		_ = artifact.Close(st.DataDir)
	})
	return q
}

func writeTestViewer(out string) error {
	if err := os.WriteFile(filepath.Join(out, "model.xkt"), []byte("synthetic-xkt"), 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "metadata.json"), []byte(`{}`), 0600)
}

func publishTestViewer(t *testing.T, st *store.Store, id string) {
	t.Helper()
	source, err := st.ConversionSource(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err = writeTestViewer(out); err != nil {
		t.Fatal(err)
	}
	if accepted, err := st.FinishConversion(context.Background(), id, source, out, nil); err != nil || !accepted {
		t.Fatal(accepted, err)
	}
}

func changeTestSource(t *testing.T, st *store.Store, id, data string) {
	t.Helper()
	if err := artifact.WriteFile(st.IFCPath(id), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
