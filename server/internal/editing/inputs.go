// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type inputContextKey struct{}

func (o *operation) inputs() (map[string]string, error) {
	if o.inputOverride != nil {
		return o.inputOverride, nil
	}
	if o.replayVersion != "" {
		data, e := o.read(o.key("scripts/" + o.replayVersion + ".inputs.json"))
		if e == nil {
			var inputs map[string]string
			e = json.Unmarshal(data, &inputs)
			return inputs, e
		}
		if !os.IsNotExist(e) {
			return nil, e
		}
	}
	value, e := o.call("build.collect", map[string]any{"model": o.model})
	if e != nil {
		return nil, e
	}
	return value.(map[string]string), nil
}
func projectInputs(ctx context.Context) (string, error) {
	files, _ := ctx.Value(inputContextKey{}).(map[string]string)
	root, e := os.MkdirTemp("", "sourceaec-inputs-")
	if e != nil {
		return "", e
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(root)
		}
	}()
	var total int
	for key, encoded := range files {
		clean := filepath.ToSlash(filepath.Clean(key))
		if key != clean || filepath.IsAbs(key) || strings.Contains(key, "\\") || (!strings.HasPrefix(key, "plans/") && !strings.HasPrefix(key, "skill-work/")) || strings.HasPrefix(key, "../") {
			return "", fmt.Errorf("invalid build input path")
		}
		data, e := base64.StdEncoding.Strict().DecodeString(encoded)
		if e != nil {
			return "", e
		}
		total += len(data)
		if len(data) > 64<<20 || total > productLimit {
			return "", fmt.Errorf("build inputs exceed size limit")
		}
		path := filepath.Join(root, filepath.FromSlash(key))
		if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			return "", e
		}
		if e = os.WriteFile(path, data, 0444); e != nil {
			return "", e
		}
	}
	success = true
	return root, nil
}
