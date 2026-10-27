// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

// A first-stage read must not discover an uncommitted script through the
// workspace. Enumerate committed scripts in the same snapshot as staging.
func (o *operation) snapshotScriptVersions() ([]version, error) {
	value, err := o.call("list", o.key("scripts/"))
	if err != nil {
		return nil, err
	}
	versions := []version{}
	for _, key := range value.([]string) {
		name := strings.TrimSuffix(path.Base(key), ".py")
		if path.Dir(key) != o.key("scripts") || !strings.HasSuffix(key, ".py") || !versionPattern.MatchString(name) {
			continue
		}
		versions = append(versions, version{Version: name})
	}
	sort.Slice(versions, func(i, j int) bool {
		a, _ := strconv.Atoi(versions[i].Version[1:])
		b, _ := strconv.Atoi(versions[j].Version[1:])
		return a < b
	})
	return versions, nil
}
