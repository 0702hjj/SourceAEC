// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGitContextCancellationKillsDescendants(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep required")
	}
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "child.pid")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	script := fmt.Sprintf("#!/bin/sh\n%s 60 &\nprintf '%%s' \"$!\" > %s\nwait\n", quote(sleep), quote(pidPath))
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := gitContext(ctx, dir, "status"); finished <- err }()
	var child int
	t.Cleanup(func() {
		cancel()
		if child > 0 {
			_ = syscall.Kill(child, syscall.SIGKILL)
		}
	})
	deadline, cancelDeadline := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelDeadline()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for child == 0 {
		if data, err := os.ReadFile(pidPath); err == nil {
			child, _ = strconv.Atoi(string(data))
		}
		if child != 0 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.Done():
			t.Fatal("Git child did not start")
		}
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancellation accepted as success")
		}
	case <-deadline.Done():
		t.Fatal("Git did not cancel")
	}
	for {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", child))
		if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		end := strings.LastIndexByte(string(data), ')')
		fields := strings.Fields(string(data)[end+1:])
		if len(fields) > 0 && fields[0] == "Z" {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.Done():
			t.Fatal("Git descendant still running")
		}
	}
}
