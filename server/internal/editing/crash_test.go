// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/editruntime"
)

func TestEditingCrashHelper(t *testing.T) {
	root := os.Getenv("SOURCEAEC_CRASH_TEST_ROOT")
	if root == "" {
		return
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	s := New(root, os.Getenv("SOURCEAEC_CRASH_TEST_KIND"))
	s.Executor = &testExecutor{}
	o := &operation{service: s, runtime: editruntime.NewEmbedded(), model: testModel, ctx: context.Background()}
	release, err := o.begin()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	staging, err := o.loadStaging()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.run(staging, "script/save", nil); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SOURCEAEC_CRASH_TEST_COMMIT") == "1" {
		if _, err = o.call("commit", nil); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("publication-ready")
	// The parent kills this process, bypassing all rollback/Close defers.
	select {}
}

func TestEditingProcessCrashRestoresCommittedState(t *testing.T) {
	for _, scenario := range []struct {
		kind      string
		committed bool
	}{{"ifc", false}, {"ifc", true}, {"dxf", false}, {"dxf", true}} {
		kind, committed := scenario.kind, scenario.committed
		t.Run(fmt.Sprintf("%s/committed=%v", kind, committed), func(t *testing.T) {
			s, _ := testService(t, kind)
			invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'before', 'x':1}\n"})
			invoke(t, s, "POST", "script/save", nil)
			invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'after', 'x':2}\n"})
			if err := artifact.Close(s.Root); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEditingCrashHelper$")
			cmd.Env = append(os.Environ(), "SOURCEAEC_CRASH_TEST_ROOT="+s.Root, "SOURCEAEC_CRASH_TEST_KIND="+kind)
			if committed {
				cmd.Env = append(cmd.Env, "SOURCEAEC_CRASH_TEST_COMMIT=1")
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(output)
			ready := false
			for scanner.Scan() {
				if scanner.Text() == "publication-ready" {
					ready = true
					break
				}
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			if !ready {
				t.Fatalf("child did not reach publication: %v", ctx.Err())
			}
			if err = artifact.Open(s.Root); err != nil {
				t.Fatal(err)
			}
			versions := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version)
			want, name := 1, "before"
			if committed {
				want, name = 2, "after"
			}
			if len(versions) != want {
				t.Fatalf("crash left phantom or missing version: %+v", versions)
			}
			product, err := os.ReadFile(filepath.Join(s.Root, "uploads", testModel+"."+kind))
			expected := fixtureIFC(name)
			if kind == "dxf" {
				expected = fixtureDXF(float64(want))
			}
			if err != nil || string(product) != string(expected) {
				t.Fatalf("projection differs from committed product: %q %v", product, err)
			}
			state := invoke(t, s, "GET", "git", nil).(map[string]any)
			if len(state["commits"].([]map[string]any)) != want {
				t.Fatalf("Git history differs from committed versions: %v", state)
			}
			staging := invoke(t, s, "GET", "script", nil).(map[string]any)
			if staging["staged"] != 2-want {
				t.Fatalf("staging differs from committed state: %v", staging)
			}
		})
	}
}
