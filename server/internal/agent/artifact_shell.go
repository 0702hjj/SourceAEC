// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
package agent

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/schema"
	"ifcviewer/server/internal/artifact"
)

type artifactShell struct {
	inner filesystem.StreamingShell
	root  string
}

var workspaceProject = regexp.MustCompile(`^p_[0-9a-f]{16}$`)

func (s *artifactShell) ExecuteStreaming(ctx context.Context, req *filesystem.ExecuteRequest) (*schema.StreamReader[*filesystem.ExecuteResponse], error) {
	stream, err := s.inner.ExecuteStreaming(ctx, req)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(req.Command)
	project := ""
	for i, field := range fields {
		if field == "--project-id" && i+1 < len(fields) {
			project = strings.Trim(fields[i+1], "\"'")
		}
		if strings.HasPrefix(field, "--project-id=") {
			project = strings.Trim(strings.TrimPrefix(field, "--project-id="), "\"'")
		}
	}
	return schema.StreamReaderWithConvert(stream, func(response *filesystem.ExecuteResponse) (*filesystem.ExecuteResponse, error) {
		if response.ExitCode != nil && workspaceProject.MatchString(project) {
			if err := artifact.CaptureTree(filepath.Join(s.root, project)); err != nil {
				return nil, err
			}
		}
		return response, nil
	}), nil
}
