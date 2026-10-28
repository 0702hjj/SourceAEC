// SPDX-License-Identifier: Apache-2.0
package store

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
)

func TestViewerReaderProcessHelper(t *testing.T) {
	root := os.Getenv("SOURCEAEC_VIEWER_ROOT")
	if root == "" {
		return
	}
	if err := artifact.Open(root); err != nil {
		t.Fatal(err)
	}
	defer artifact.Close(root)
	s := NewStore(root)
	file, err := s.OpenViewer(context.Background(), os.Getenv("SOURCEAEC_VIEWER_ID"), "model.xkt", "")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	fmt.Println("reader-ready")
	if _, err = bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("reader:%s\n", data)
}

func TestViewerOpenDescriptorSurvivesCrossProcessDelete(t *testing.T) {
	s, id, source := conversionFixture(t)
	if _, err := s.FinishConversion(context.Background(), id, source, conversionOutput(t, s, id, "old"), nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestViewerReaderProcessHelper$")
	cmd.Env = append(os.Environ(), "SOURCEAEC_VIEWER_ROOT="+s.DataDir, "SOURCEAEC_VIEWER_ID="+id)
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
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(out)
	ready := false
	for scanner.Scan() {
		if scanner.Text() == "reader-ready" {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("reader did not open committed generation")
	}
	if err = s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err = fmt.Fprintln(in, "resume"); err != nil {
		t.Fatal(err)
	}
	_ = in.Close()
	read := false
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "reader:old" {
			read = true
		}
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if !read {
		t.Fatal("in-flight cross-process reader broke")
	}
}
