// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkflowManagedEnvironmentAvoidsSystemPython(t *testing.T) {
	if _, err := os.Stat("/usr/bin/python3"); err != nil {
		t.Skip("system Python fixture unavailable")
	}
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "legacy-cache"}[legacy], func(t *testing.T) {
			root := t.TempDir()
			uv := filepath.Join(root, "uv")
			// Simulate uv's valid default: select the system interpreter when available.
			// Managed selection instead provides a private synthetic interpreter tree.
			tool := `#!/bin/sh
set -eu
for target do :; done
mkdir -p "$target/bin"
case " $* " in
 *" --managed-python "*) printf 'synthetic managed interpreter' > "$target/bin/python" ;;
 *) ln -s /usr/bin/python3 "$target/bin/python" ;;
esac
`
			if err := os.WriteFile(uv, []byte(tool), 0700); err != nil {
				t.Fatal(err)
			}
			s := NewSandbox()
			s.Python = ""
			s.UV = uv
			s.CacheDir = filepath.Join(root, "cache")
			if legacy {
				old := filepath.Join(s.CacheDir, digest([]byte("python3.12:[]")))
				if err := os.MkdirAll(filepath.Join(old, "bin"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/usr/bin/python3", filepath.Join(old, "bin", "python")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(old, ".ready"), []byte("[]"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			script := "# /// script\n# dependencies = []\n# ///\nPARAMS = {}\n"
			seal, err := s.Freeze(context.Background(), script, "ifc")
			if err != nil {
				t.Fatal("default environment selected a system base instead of a sealable managed runtime", err)
			}
			repeated, err := s.Freeze(context.Background(), script, "ifc")
			if err != nil || repeated != seal {
				t.Fatal("managed cache identity not reusable", err)
			}
		})
	}
}
func TestWorkflowConfiguredSystemBaseRemainsRejected(t *testing.T) {
	if _, err := os.Stat("/usr/bin/python3"); err != nil {
		t.Skip("system Python fixture unavailable")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(root, "bin", "python")
	if err := os.Symlink("/usr/bin/python3", python); err != nil {
		t.Fatal(err)
	}
	_, err := auditEnvironment(context.Background(), python)
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 503 {
		t.Fatal("unsealable system base accepted", err)
	}
}

func TestWorkflowRealManagedEnvironmentSeal(t *testing.T) {
	if os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		t.Skip("external runtime integration disabled")
	}
	s := NewSandbox()
	s.Python = ""
	s.CacheDir = t.TempDir()
	source := "# /// script\n# dependencies = []\n# ///\nPARAMS = {}\nimport sys\nopen(sys.argv[1], 'w').write(" + pythonLiteral(string(fixtureIFC("managed-workflow"))) + ")\n"
	seal, err := s.Freeze(context.Background(), source, "ifc")
	if err != nil {
		t.Fatal(err)
	}
	product, err := s.RunFrozen(context.Background(), source, "ifc", seal)
	if err != nil || product.Environment != seal.Digest || string(product.Data) != string(fixtureIFC("managed-workflow")) {
		t.Fatal("managed frozen execution", err)
	}
}
