// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"errors"
	"ifcviewer/server/internal/artifact"
	"os"
	"strings"
	"testing"
)

func TestGitPreservesHumanAndAIProvenance(t *testing.T) {
	_, root := databaseRuntime(t)
	db, err := artifact.Database(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := workspaceSnapshot(context.Background(), root, "m_0000000000000001", RepositorySnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(c.directory)
	for _, source := range []string{"ai", "human"} {
		if _, err = commitGit(db, c, gitArgs{Script: "PARAMS = {}", Source: source}); err != nil {
			t.Fatal(err)
		}
		message, err := c.git("log", "-1", "--format=%B")
		if err != nil || !strings.Contains(message, "SourceAEC-Source: "+source) {
			t.Fatal("lost edit provenance", message, err)
		}
	}
}

func TestGitWorkspacePreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, err := workspaceSnapshot(ctx, t.TempDir(), "m_0000000000000001", RepositorySnapshot{})
	if c != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Git command lost caller cancellation: %v", err)
	}
}
