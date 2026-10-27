// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifcviewer/server/internal/metrics"
)

func TestSandboxBudgetSharedAcrossFormats(t *testing.T) {
	available := nativeSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 3)
	t.Cleanup(func() {
		cancel()
		for i := 0; i < 3; i++ {
			select {
			case err := <-done:
				if err == nil {
					t.Error("blocked geometry unexpectedly succeeded")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("geometry did not stop")
			}
		}
		if metrics.ScriptRunning.Value() != 0 {
			t.Error("geometry admission leaked")
		}
		s := nativeSandbox(t)
		source := "# /// script\n# dependencies = []\n# ///\nimport sys\nopen(sys.argv[1], 'w').write(" + pythonLiteral(string(fixtureDXF(1))) + ")\n"
		if _, err := s.Run(context.Background(), source, "dxf"); err != nil {
			t.Errorf("execution did not recover after saturation: %v", err)
		}
	})
	for _, kind := range []string{"ifc", "dxf", "ifc"} {
		s := NewSandbox()
		s.Python = available.Python
		go func() {
			_, err := s.Run(ctx, "# /// script\n# dependencies = []\n# ///\nimport time\ntime.sleep(60)\n", kind)
			done <- err
		}()
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for metrics.ScriptRunning.Value() != 3 {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("geometry jobs did not enter admission")
		}
	}
	s := nativeSandbox(t)
	source := "# /// script\n# dependencies = []\n# ///\nimport sys\nopen(sys.argv[1], 'w').write(" + pythonLiteral(string(fixtureDXF(1))) + ")\n"
	_, err := s.Run(ctx, source, "dxf")
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 429 {
		t.Fatalf("separate service exceeded combined budget: %v", err)
	}
}

func TestSandboxDeniesHostNetworkAndInputWrites(t *testing.T) {
	s := nativeSandbox(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	marker := filepath.Join(t.TempDir(), "host-private")
	if err = os.WriteFile(marker, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	script := "# /// script\n# dependencies = []\n# ///\nimport socket, sys\n" +
		"try:\n socket.create_connection(('127.0.0.1', " + port + "), timeout=0.2)\nexcept OSError:\n pass\nelse:\n raise RuntimeError('host network exposed')\n" +
		"for path in [" + pythonLiteral(marker) + ", '/inputs/plans/synthetic.json']:\n try:\n  open(path, 'w').write('changed')\n except OSError:\n  pass\n else:\n  raise RuntimeError('host or input write allowed')\n" +
		"open(sys.argv[1], 'w').write(" + pythonLiteral(string(fixtureIFC("isolated"))) + ")\n"
	ctx := context.WithValue(context.Background(), inputContextKey{}, map[string]string{"plans/synthetic.json": base64.StdEncoding.EncodeToString([]byte(`{}`))})
	if _, err = s.Run(ctx, script, "ifc"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "unchanged" {
		t.Fatal("host marker changed", err)
	}
}

func TestSandboxPreparationHonorsRunBudget(t *testing.T) {
	s := nativeSandbox(t)
	s.Python = ""
	s.Timeout = 20 * time.Millisecond
	environmentGate <- struct{}{}
	defer func() { <-environmentGate }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.Run(ctx, "# /// script\n# dependencies = []\n# ///\n", "ifc")
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 504 {
		t.Fatalf("preparation ignored execution budget: %v", err)
	}
}

func TestSandboxRejectsNonPositiveBudget(t *testing.T) {
	for _, budget := range []time.Duration{0, -time.Second} {
		s := NewSandbox()
		s.Timeout = budget
		_, err := s.Run(context.Background(), "", "ifc")
		var domain *Error
		if !errors.As(err, &domain) || domain.Status != 503 {
			t.Fatalf("invalid timeout %v accepted: %v", budget, err)
		}
	}
}
