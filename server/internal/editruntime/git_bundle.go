// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func gitCommand(parent context.Context, repo string, args ...string) (*exec.Cmd, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	argv := append([]string{"-C", repo, "-c", "user.name=SourceAEC", "-c", "user.email=sourceaec@localhost", "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	return cmd, cancel
}

type bundleWriter struct {
	target io.Writer
	lease  *gitReference
	cancel context.CancelFunc
	size   int64
	err    error
}

func (w *bundleWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if err := w.lease.grow(w.size + int64(len(p))); err != nil {
		w.err = err
		w.cancel()
		return 0, err
	}
	n, err := w.target.Write(p)
	w.size += int64(n)
	if err != nil {
		w.err = err
		w.cancel()
	}
	return n, err
}

type gitDiagnostics struct{ data []byte }

func (d *gitDiagnostics) Write(p []byte) (int, error) {
	n := len(p)
	d.data = append(d.data, p...)
	if len(d.data) > 2048 {
		d.data = d.data[len(d.data)-2048:]
	}
	return n, nil
}

// Stream before storing. Both the temporary bundle and its later BYTEA read
// stay covered by the candidate's largest input/output source-byte charge.
func writeGitBundle(c *candidate, filename string) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd, cancel := gitCommand(c.ctx, c.repo, "bundle", "create", "-", "--all")
	defer cancel()
	output := &bundleWriter{target: file, lease: c.bundleLease, cancel: cancel}
	diagnostics := &gitDiagnostics{}
	cmd.Stdout = output
	cmd.Stderr = diagnostics
	if err = cmd.Run(); err != nil {
		if output.err != nil {
			return output.err
		}
		return fmt.Errorf("Git bundle failed: %w: %s", err, strings.TrimSpace(string(diagnostics.data)))
	}
	return file.Close()
}
