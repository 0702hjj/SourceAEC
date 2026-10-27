// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestProcessChildrenIncludesWorkerThreadProcesses(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("RSS sampling uses Linux procfs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started, finished := make(chan *exec.Cmd, 1), make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		cmd := exec.CommandContext(ctx, "sleep", "30")
		if err := cmd.Start(); err != nil {
			finished <- err
			return
		}
		started <- cmd
		finished <- cmd.Wait()
	}()
	var cmd *exec.Cmd
	select {
	case cmd = <-started:
	case err := <-finished:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("child did not start")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("child did not terminate")
		}
	})
	if !processChildren(os.Getpid())[cmd.Process.Pid] {
		t.Fatal("RSS sampler missed a subprocess launched by a worker thread")
	}
}
