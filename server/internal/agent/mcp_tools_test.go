// SPDX-License-Identifier: Apache-2.0
package agent

import (
	"context"
	"path/filepath"
	"testing"
)

func TestLoadMCPToolsDisabled(t *testing.T) {
	tools, cleanup, err := LoadMCPTools(context.Background(), MCPToolsConfig{})
	defer cleanup()
	if err != nil || len(tools) != 0 {
		t.Fatal(tools, err)
	}
}
func TestConfiguredMissingMCPBinaryIsReported(t *testing.T) {
	_, cleanup, err := LoadMCPTools(context.Background(), MCPToolsConfig{Binary: filepath.Join(t.TempDir(), "missing")})
	defer cleanup()
	if err == nil {
		t.Fatal("missing configured binary silently ignored")
	}
}
