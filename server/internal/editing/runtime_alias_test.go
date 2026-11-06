// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSandboxExecutesInterpreterThroughRuntimeDirectoryAlias(t *testing.T) {
	s := nativeSandbox(t)
	real, err := filepath.EvalSymlinks(s.Python)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	private := filepath.Join(root, "private-sibling")
	if err = os.WriteFile(private, []byte("must stay outside sandbox"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "runtime-alias")
	if err = os.Symlink(filepath.Dir(filepath.Dir(real)), alias); err != nil {
		t.Fatal(err)
	}
	venv := filepath.Join(root, "environment")
	if err = os.MkdirAll(filepath.Join(venv, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	s.Python = filepath.Join(venv, "bin", "python")
	if err = os.Symlink(filepath.Join(alias, "bin", filepath.Base(real)), s.Python); err != nil {
		t.Fatal(err)
	}
	script := "# /// script\n# dependencies = []\n# ///\nPARAMS = {}\nimport os, sys\nassert not os.path.exists(" + pythonLiteral(private) + ")\nopen(sys.argv[1], 'w').write(" + pythonLiteral(string(fixtureIFC("runtime-alias"))) + ")\n"
	product, err := s.Run(context.Background(), script, "ifc")
	if err != nil || string(product.Data) != string(fixtureIFC("runtime-alias")) {
		t.Fatal("host interpreter exists but its runtime alias is invisible inside the sandbox", err)
	}
}

func TestPythonRuntimeRootsPreservesRelativeLinkChain(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"environment", "alias", "runtime"} {
		if err := os.MkdirAll(filepath.Join(root, name, "bin"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(root, "runtime", "bin", "python")
	if err := os.WriteFile(real, []byte("synthetic interpreter"), 0700); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(root, "environment", "bin", "python")
	if err := os.Symlink("../../alias/bin/python", python); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../runtime/bin/python", filepath.Join(root, "alias", "bin", "python")); err != nil {
		t.Fatal(err)
	}
	got, err := pythonRuntimeRoots(python)
	want := []string{filepath.Join(root, "environment"), filepath.Join(root, "alias"), filepath.Join(root, "runtime")}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("roots = %v, %v; want %v", got, err, want)
	}
}

func TestPythonRuntimeRootsRejectsInvalidLinks(t *testing.T) {
	for _, target := range []string{"python", "missing"} {
		t.Run(target, func(t *testing.T) {
			python := filepath.Join(t.TempDir(), "python")
			if err := os.Symlink(target, python); err != nil {
				t.Fatal(err)
			}
			if roots, err := pythonRuntimeRoots(python); err == nil || roots != nil {
				t.Fatalf("invalid runtime accepted: %v, %v", roots, err)
			}
		})
	}
}
