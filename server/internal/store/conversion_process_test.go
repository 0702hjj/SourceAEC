// SPDX-License-Identifier: Apache-2.0
package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

func TestConversionProcessHelper(t *testing.T) {
	root := os.Getenv("SOURCEAEC_CONVERSION_ROOT")
	if root == "" {
		return
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	s := NewStore(root)
	id := os.Getenv("SOURCEAEC_CONVERSION_ID")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	source, err := s.ConversionSource(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.MkdirTemp(s.ModelDir(id), ".process-convert-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(output)
	for _, name := range []string{"model.xkt", "metadata.json"} {
		data := []byte("synthetic-process-output")
		if name == "metadata.json" {
			data = []byte(`{"synthetic":true}`)
		}
		if err = os.WriteFile(filepath.Join(output, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("converter-ready")
	if _, err = bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	var runErr error
	if os.Getenv("SOURCEAEC_CONVERSION_FAIL") == "1" {
		runErr = errors.New("late process conversion failed")
	}
	accepted, err := s.FinishConversion(ctx, id, source, output, runErr)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("conversion-finished:%v\n", accepted)
}

func TestSeparateConverterProcessesPreserveMatchingReady(t *testing.T) {
	s, id, _ := conversionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	type child struct {
		cmd *exec.Cmd
		in  io.WriteCloser
		out *bufio.Scanner
	}
	start := func(fail bool) child {
		t.Helper()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConversionProcessHelper$")
		cmd.Env = append(os.Environ(), "SOURCEAEC_CONVERSION_ROOT="+s.DataDir, "SOURCEAEC_CONVERSION_ID="+id)
		if fail {
			cmd.Env = append(cmd.Env, "SOURCEAEC_CONVERSION_FAIL=1")
		}
		in, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stderr = os.Stderr
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		scanner := bufio.NewScanner(out)
		ready := false
		for scanner.Scan() {
			if scanner.Text() == "converter-ready" {
				ready = true
				break
			}
		}
		if !ready {
			t.Fatal("converter did not capture its source snapshot")
		}
		return child{cmd, in, scanner}
	}
	first, late := start(false), start(true)
	finish := func(c child) {
		t.Helper()
		if _, err := fmt.Fprintln(c.in, "resume"); err != nil {
			t.Fatal(err)
		}
		_ = c.in.Close()
		accepted := false
		for c.out.Scan() {
			if c.out.Text() == "conversion-finished:true" {
				accepted = true
			}
		}
		if err := c.cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		if !accepted {
			t.Fatal("matching conversion did not finish")
		}
	}
	finish(first)
	before, err := s.Get(id)
	if err != nil || before.Status != "ready" {
		t.Fatal(before, err)
	}
	finish(late)
	after, err := s.Get(id)
	if err != nil || after.Status != "ready" || after.Error != "" || after.PreviewGeneration != before.PreviewGeneration {
		t.Fatal("late process overwrote ready generation", after, err)
	}
}
