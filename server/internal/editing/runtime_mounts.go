// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A venv may link through a directory alias before reaching the real runtime.
// Mounting only EvalSymlinks(python)'s directory leaves that link dangling in
// bubblewrap. Preserve each link's named runtime root as well as its real root;
// never expose a common parent such as the home or cache directory.
func pythonRuntimeRoots(python string) ([]string, error) {
	paths := []string{python}
	current := python
	seen := map[string]bool{}
	for {
		if seen[current] || len(seen) >= 64 {
			return nil, fmt.Errorf("Python interpreter symlink chain is cyclic or too deep")
		}
		seen[current] = true
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			break
		}
		target, err := os.Readlink(current)
		if err != nil {
			return nil, err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		paths = append(paths, target)
		current = target
	}
	real, err := filepath.EvalSymlinks(python)
	if err != nil {
		return nil, err
	}
	paths = append(paths, real)
	roots := []string{}
	added := map[string]bool{}
	for _, name := range paths {
		root := filepath.Dir(filepath.Dir(name))
		// These system paths are already mounted. In particular, never bind /.
		if root == "/" || root == "/usr" || strings.HasPrefix(root, "/usr/") || added[root] {
			continue
		}
		added[root] = true
		roots = append(roots, root)
	}
	return roots, nil
}
