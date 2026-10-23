// SPDX-License-Identifier: Apache-2.0
package releasepackage

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerReleasePackagesBothArchitectures(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"scripts", "server", "tools"} {
		if e := os.Mkdir(filepath.Join(root, dir), 0755); e != nil {
			t.Fatal(e)
		}
	}
	script, e := os.ReadFile("../../../scripts/build_server_release.sh")
	if e != nil {
		t.Fatal(e)
	}
	scriptPath := filepath.Join(root, "scripts/build_server_release.sh")
	if e = os.WriteFile(scriptPath, script, 0755); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"LICENSE", "NOTICE", "server/server_config.example.json", "scripts/server_release_README.md"} {
		if e = os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0644); e != nil {
			t.Fatal(e)
		}
	}
	fake := `#!/bin/sh
if [ "$1" = version ]; then echo go1.27.3; exit; fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; printf '%s/%s' "$GOOS" "$GOARCH" > "$1"; exit; fi
  shift
done
exit 1
`
	if e = os.WriteFile(filepath.Join(root, "tools/go"), []byte(fake), 0755); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command("bash", scriptPath, "0.1.0")
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "tools")+":"+os.Getenv("PATH"))
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(string(output), e)
	}
	out := filepath.Join(root, "dist/server-v0.1.0")
	sums, e := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if e != nil {
		t.Fatal(e)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		name := "sourceaec_0.1.0_linux_" + arch
		archive, e := os.ReadFile(filepath.Join(out, name+".tar.gz"))
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(string(sums), fmt.Sprintf("%x  %s.tar.gz", sha256.Sum256(archive), name)) {
			t.Fatal("checksum mismatch")
		}
		gz, e := gzip.NewReader(strings.NewReader(string(archive)))
		if e != nil {
			t.Fatal(e)
		}
		defer gz.Close()
		reader := tar.NewReader(gz)
		contents := map[string]string{}
		for {
			header, e := reader.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal(e)
			}
			if header.Typeflag == tar.TypeReg {
				data, e := io.ReadAll(reader)
				if e != nil {
					t.Fatal(e)
				}
				contents[header.Name] = string(data)
			}
		}
		for _, binary := range []string{"sourceaec-server", "sourceaec-runtime", "sourceaec-mcp"} {
			if contents[name+"/bin/"+binary] != "linux/"+arch {
				t.Fatal("missing binary", binary)
			}
			if contents[name+"/bin/"+binary+".buildinfo.txt"] != "go1.27.3\n" {
				t.Fatal("missing build info", binary)
			}
		}
		for _, file := range []string{"LICENSE", "NOTICE", "README.md", "server/server_config.example.json"} {
			if contents[name+"/"+file] != "fixture" {
				t.Fatal("missing", file)
			}
		}
	}
	bad := exec.Command("bash", scriptPath, "../invalid")
	if bad.Run() == nil {
		t.Fatal("invalid version accepted")
	}
}
