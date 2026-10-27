// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

type processPausedExecutor struct{}

func (processPausedExecutor) Run(ctx context.Context, script, kind string) (Product, error) {
	fmt.Println("execution-ready")
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		return Product{}, err
	}
	return (&testExecutor{}).Run(ctx, script, kind)
}
func TestDetachedProcessHelper(t *testing.T) {
	root := os.Getenv("SOURCEAEC_DETACHED_ROOT")
	if root == "" {
		return
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	s := New(root, "ifc")
	s.Executor = processPausedExecutor{}
	_, err := s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", nil)
	if err != nil {
		fmt.Println("outcome:" + err.Error())
	} else {
		fmt.Println("outcome:success")
	}
}
func TestDetachedExecutionRejectsCrossProcessStage(t *testing.T) {
	s, _ := testService(t, "ifc")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'old'}\n"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDetachedProcessHelper$")
	cmd.Env = append(os.Environ(), "SOURCEAEC_DETACHED_ROOT="+s.Root)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(stdout)
	ready := false
	for scanner.Scan() {
		if scanner.Text() == "execution-ready" {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("child never started execution")
	}
	// This is a different OS process with its own pool and admission registry.
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'name':'new'}\n"})
	if _, err = fmt.Fprintln(stdin, "resume"); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	conflict := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "outcome:model or build inputs changed") {
			conflict = true
		}
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if !conflict {
		t.Fatal("cross-process stale execution published")
	}
	if versions := invoke(t, s, "GET", "versions", nil).(map[string]any)["versions"].([]version); len(versions) != 0 {
		t.Fatal(versions)
	}
}
