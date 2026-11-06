// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type EnvironmentSeal struct {
	Version int    `json:"version"`
	Python  string `json:"python"`
	Digest  string `json:"digest"`
}

type PinnedExecutor interface {
	Executor
	Freeze(context.Context, string, string) (EnvironmentSeal, error)
	RunFrozen(context.Context, string, string, EnvironmentSeal) (Product, error)
}

func (s *Sandbox) Freeze(ctx context.Context, script, kind string) (EnvironmentSeal, error) {
	python, _, err := s.environment(ctx, script, kind)
	if err != nil {
		return EnvironmentSeal{}, err
	}
	return auditEnvironment(ctx, python)
}

func (s *Sandbox) RunFrozen(ctx context.Context, script, kind string, seal EnvironmentSeal) (Product, error) {
	before, err := s.Freeze(ctx, script, kind)
	if err != nil {
		return Product{}, err
	}
	if before != seal || seal.Version != 1 {
		return Product{}, failure(503, "frozen geometry environment changed")
	}
	pinned := *s
	pinned.Python = seal.Python
	product, err := pinned.Run(ctx, script, kind)
	if err != nil {
		return Product{}, err
	}
	after, err := auditEnvironment(ctx, seal.Python)
	if err != nil {
		return Product{}, err
	}
	if after != seal {
		return Product{}, failure(503, "geometry environment changed during execution")
	}
	product.Environment = seal.Digest
	return product, nil
}

// Seal installed content, not just version metadata. This is conservative
// dependency drift detection, not an OS snapshot or deterministic execution.
func auditEnvironment(ctx context.Context, python string) (EnvironmentSeal, error) {
	real, err := filepath.EvalSymlinks(python)
	if err != nil {
		return EnvironmentSeal{}, err
	}
	root := filepath.Dir(filepath.Dir(python))
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return EnvironmentSeal{}, err
	}
	base := filepath.Dir(filepath.Dir(real))
	if root == "/usr" || root == "/" || base == "/usr" || base == "/" {
		return EnvironmentSeal{}, failure(503, "durable geometry requires an isolated Python environment")
	}
	roots := []string{root}
	if base != root {
		roots = append(roots, base)
	}
	sort.Strings(roots)
	h := sha256.New()
	fmt.Fprintf(h, "sourceaec.environment.v1\x00%s\x00", python)
	files := 0
	var total int64
	buffer := make([]byte, 64<<10)
	for _, directory := range roots {
		err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			files++
			if files > 100000 {
				return fmt.Errorf("geometry environment file limit exceeded")
			}
			resolved := path
			if entry.Type()&os.ModeSymlink != 0 {
				var err error
				resolved, err = filepath.EvalSymlinks(path)
				if err != nil {
					return err
				}
				allowed := false
				for _, r := range roots {
					if resolved == r || strings.HasPrefix(resolved, r+string(os.PathSeparator)) {
						allowed = true
					}
				}
				if !allowed {
					return fmt.Errorf("geometry environment symlink escapes sealed roots")
				}
				info, err := os.Stat(resolved)
				if err != nil {
					return err
				}
				if info.IsDir() {
					fmt.Fprintf(h, "link:%s=>%s\x00", path, resolved)
					return nil
				}
			}
			file, err := os.Open(resolved)
			if err != nil {
				return err
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("geometry environment contains nonregular file")
			}
			total += info.Size()
			if total > 16<<30 {
				return fmt.Errorf("geometry environment exceeds seal limit")
			}
			fmt.Fprintf(h, "%s\x00%d\x00%d\x00", path, info.Mode().Perm(), info.Size())
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				n, e := file.Read(buffer)
				if n > 0 {
					_, _ = h.Write(buffer[:n])
				}
				if e == io.EOF {
					break
				}
				if e != nil {
					return e
				}
			}
			after, err := file.Stat()
			if err != nil {
				return err
			}
			if info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
				return fmt.Errorf("geometry environment changed while sealing")
			}
			return nil
		})
		if err != nil {
			return EnvironmentSeal{}, err
		}
	}
	return EnvironmentSeal{1, python, hex.EncodeToString(h.Sum(nil))}, nil
}
