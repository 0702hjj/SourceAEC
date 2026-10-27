// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/store"
)

func testQueue(t *testing.T, st *store.Store, runner Runner, workers int) *Queue {
	t.Helper()
	q := NewQueue(st, runner, workers)
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

func viewerFixture(out string) error {
	if err := os.WriteFile(filepath.Join(out, "model.xkt"), []byte("synthetic-xkt"), 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "metadata.json"), []byte(`{}`), 0600)
}
