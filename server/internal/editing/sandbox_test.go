// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func nativeSandbox(t *testing.T) *Sandbox {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap unavailable")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("external Python runtime unavailable")
	}
	s := NewSandbox()
	s.Python = python
	return s
}
func TestSandboxExecutesRuntimeArtifact(t *testing.T) {
	s := nativeSandbox(t)
	script := "# /// script\n# dependencies = []\n# ///\nPARAMS = {}\nimport sys\nopen(sys.argv[1], 'w').write(" + pythonLiteral(string(fixtureIFC("sandbox"))) + ")\n"
	product, err := s.Run(context.Background(), script, "ifc")
	if err != nil {
		t.Fatal(err)
	}
	if string(product.Data) != string(fixtureIFC("sandbox")) {
		t.Fatal("wrong sandbox product")
	}
}
func TestSandboxTimeoutAndOutputLimits(t *testing.T) {
	for name, script := range map[string]string{"timeout": "while True: pass\n", "output": "while True: print('x'*10000, flush=True)\n"} {
		t.Run(name, func(t *testing.T) {
			s := nativeSandbox(t)
			s.Timeout = 500 * time.Millisecond
			_, err := s.Run(context.Background(), "# /// script\n# dependencies = []\n# ///\nPARAMS = {}\n"+script, "ifc")
			var domain *Error
			if !errors.As(err, &domain) {
				t.Fatal(err)
			}
			want := 504
			if name == "output" {
				want = 422
			}
			if domain.Status != want {
				t.Fatalf("%d %v", domain.Status, err)
			}
		})
	}
}
