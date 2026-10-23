// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var environmentGate = make(chan struct{}, 1)

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func dependencies(script, kind string) ([]string, error) {
	deps := []string{"ifcopenshell==0.8.3.post2", "numpy==2.2.6"}
	if kind == "dxf" {
		deps = []string{"ezdxf==1.4.3"}
	}
	var block strings.Builder
	active := false
	found := false
	for _, line := range strings.Split(script, "\n") {
		if line == "# /// script" {
			if found {
				return nil, fmt.Errorf("duplicate script dependency block")
			}
			found = true
			active = true
			continue
		}
		if active && line == "# ///" {
			active = false
			break
		}
		if active {
			if !strings.HasPrefix(line, "#") {
				return nil, fmt.Errorf("invalid script dependency block")
			}
			block.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "#"), " "))
			block.WriteByte('\n')
		}
	}
	if active {
		return nil, fmt.Errorf("unterminated script dependency block")
	}
	if found {
		var metadata struct {
			Dependencies []string `toml:"dependencies"`
		}
		if e := toml.Unmarshal([]byte(block.String()), &metadata); e != nil {
			return nil, e
		}
		deps = metadata.Dependencies
	}
	for _, dep := range deps {
		if len(dep) > 200 || strings.ContainsAny(dep, "/@\\\n\r\x00") || strings.HasPrefix(dep, "-") || !strings.Contains(dep, "==") {
			return nil, fmt.Errorf("dependencies require pinned registry package versions")
		}
	}
	return deps, nil
}
func (s *Sandbox) environment(ctx context.Context, script, kind string) (string, string, error) {
	deps, e := dependencies(script, kind)
	if e != nil {
		return "", "", failure(422, e.Error())
	}
	if s.Python != "" {
		python, e := exec.LookPath(s.Python)
		if e != nil {
			return "", "", failure(503, "configured Python interpreter unavailable")
		}
		python, _ = filepath.Abs(python)
		info, e := os.Stat(python)
		if e != nil {
			return "", "", e
		}
		return python, environmentFingerprint(python, fmt.Sprintf("%d:%d:%v", info.Size(), info.ModTime().UnixNano(), deps)), nil
	}
	select {
	case environmentGate <- struct{}{}:
		defer func() { <-environmentGate }()
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	raw, _ := json.Marshal(deps)
	identity := digest(append([]byte("python3.12:"), raw...))
	directory := filepath.Join(s.CacheDir, identity)
	python := filepath.Join(directory, "bin", "python")

	uv, e := exec.LookPath(s.UV)
	if e != nil {
		return "", "", failure(503, "uv or SOURCEAEC_PYTHON is required")
	}
	if e = os.MkdirAll(s.CacheDir, 0700); e != nil {
		return "", "", e
	}
	lock, e := os.OpenFile(filepath.Join(s.CacheDir, identity+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return "", "", e
	}
	defer lock.Close()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			break
		}
		if e != syscall.EWOULDBLOCK {
			return "", "", e
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-ticker.C:
		}
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if _, e := os.Stat(filepath.Join(directory, ".ready")); e == nil {
		return python, environmentFingerprint(python, identity), nil
	}
	// Venv paths cannot be renamed: prepare at the final path and publish a ready marker last.
	if e = os.MkdirAll(directory, 0700); e != nil {
		return "", "", e
	}
	prepareCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := command(prepareCtx, uv, "venv", "--python", "3.12", directory)
	if log, e := execute(prepareCtx, cmd); e != nil {
		return "", "", failure(503, "Python environment creation failed: "+log)
	}
	if len(deps) > 0 {
		args := append([]string{"pip", "install", "--only-binary=:all:", "--python", python, "--"}, deps...)
		if log, e := execute(prepareCtx, command(prepareCtx, uv, args...)); e != nil {
			return "", "", failure(503, "Python dependencies unavailable: "+log)
		}
	}
	if e = os.WriteFile(filepath.Join(directory, ".ready"), raw, 0600); e != nil {
		return "", "", e
	}
	return python, environmentFingerprint(python, identity), nil
}

func environmentFingerprint(python, seed string) string {
	root := filepath.Dir(filepath.Dir(python))
	paths, _ := filepath.Glob(filepath.Join(root, "lib", "python*", "site-packages", "*.dist-info", "METADATA"))
	sort.Strings(paths)
	var input strings.Builder
	input.WriteString(seed)
	for _, path := range paths {
		data, e := os.ReadFile(path)
		if e == nil {
			input.WriteString(filepath.Base(filepath.Dir(path)))
			input.WriteString(digest(data))
		}
	}
	return digest([]byte(input.String()))
}
