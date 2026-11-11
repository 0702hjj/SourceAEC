// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"fmt"
	"ifcviewer/server/internal/notebookcache"
)

func notebookEnvironment(seal EnvironmentSeal, addressSpace uint64) EnvironmentSeal {
	seal.Digest = digest([]byte(fmt.Sprintf("%s:as=%d:%s", notebookcache.Policy, addressSpace, seal.Digest)))
	return seal
}
func (s *Sandbox) auditRuntime(ctx context.Context, python, script string) (EnvironmentSeal, error) {
	seal, err := auditEnvironment(ctx, python)
	if err != nil {
		return seal, err
	}
	policy, err := notebookDeclaration(script)
	if err != nil {
		return EnvironmentSeal{}, failure(422, err.Error())
	}
	if policy == nil {
		return seal, nil
	}
	if s.NotebookAddressSpace < 64<<20 || s.NotebookAddressSpace > 8<<30 {
		return EnvironmentSeal{}, failure(503, "invalid administrator notebook address-space budget")
	}
	return notebookEnvironment(seal, s.NotebookAddressSpace), nil
}

const notebookWorkflowEngine = "sourceaec.notebook.v1"

func executionEngine(script string) (string, error) {
	policy, err := notebookDeclaration(script)
	if err != nil {
		return "", failure(422, err.Error())
	}
	if policy != nil {
		return notebookWorkflowEngine, nil
	}
	return workflowEngine, nil
}
