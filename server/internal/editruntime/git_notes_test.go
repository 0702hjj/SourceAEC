// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGitNotesLargeInputsDoNotUseCommandArguments(t *testing.T) {
	r, root := databaseRuntime(t)
	payload := strings.Repeat("synthetic", 300000)
	result := gitCall(t, r, root, "commit", "", gitArgs{Script: "PARAMS = {}", Source: "ai", Note: strings.Repeat("synthetic note ", 20000), AcceptedAt: "2026-10-28T12:00:00Z", Inputs: map[string]any{"current.inputs.json": map[string]any{"payload": payload}}}).(map[string]string)
	db, err := r.database(Request{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	s, err := repositorySnapshot(context.Background(), db, "m_0000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	c, err := workspaceSnapshot(context.Background(), root, "m_0000000000000001", s)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(c.directory)
	note, err := c.git("notes", "--ref=build", "show", result["commit"])
	if err != nil || !strings.Contains(note, payload) {
		t.Fatal("large build note lost", err)
	}
	message, err := c.git("show", "-s", "--format=%B%n%aI", result["commit"])
	lines := strings.Split(strings.TrimSpace(message), "\n")
	stamp, parseErr := time.Parse(time.RFC3339, lines[len(lines)-1])
	if err != nil || !strings.Contains(message, "SourceAEC-Source: ai") || !strings.Contains(message, "SourceAEC-Accepted-At: 2026-10-28T12:00:00Z") || parseErr != nil || !stamp.Equal(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("original provenance time lost", message, err)
	}
}
