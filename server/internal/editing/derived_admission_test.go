// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/metrics"
	"ifcviewer/server/internal/store"
)

func TestDerivedWaitersDoNotOccupyEditingPool(t *testing.T) {
	for _, operation := range []string{"status", "conversion", "delete"} {
		t.Run(operation, func(t *testing.T) {
			s, _ := testService(t, "ifc")
			if err := artifact.WriteFile(filepath.Join(s.Root, "models", testModel, "model.json"), jsonBytes(store.Model{ID: testModel, Kind: "ifc", Status: "converting"}), 0644); err != nil {
				t.Fatal(err)
			}
			st := store.NewStore(s.Root)
			source, err := st.ConversionSource(context.Background(), testModel)
			if err != nil {
				t.Fatal(err)
			}
			invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'base'}\n"})
			const other = "m_0000000000000002"
			if err = artifact.WriteFile(filepath.Join(s.Root, "uploads", other+".ifc"), fixtureIFC("other"), 0644); err != nil {
				t.Fatal(err)
			}
			db, err := artifact.Database(s.Root)
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(2)
			startBlockedWrite(t, s)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			done := make(chan error, 1)
			go func() {
				var err error
				if operation == "status" {
					err = st.SetStatusContext(ctx, testModel, "failed", "synthetic")
				} else if operation == "delete" {
					err = st.DeleteContext(ctx, testModel)
				} else {
					_, err = st.FinishConversion(ctx, testModel, source, "", errors.New("synthetic"))
				}
				done <- err
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Errorf("derived wait did not return cancellation: %v", err)
					}
				case <-time.After(time.Second):
					t.Error("derived waiter did not cancel")
				}
			})
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for db.Stats().InUse < 2 && metrics.ModelWaiting.Value() == 0 {
				select {
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal("derived write did not reach lock admission")
				}
			}
			writeCtx, stop := context.WithTimeout(ctx, 200*time.Millisecond)
			defer stop()
			if _, err = s.Do(writeCtx, "PUT", "/models/"+other+"/script", jsonBytes(map[string]any{"script": "PARAMS = {}\n"})); err != nil {
				t.Fatalf("derived waiter exhausted pool for unrelated model: %v", err)
			}
		})
	}
}
