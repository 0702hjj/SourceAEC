// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"ifcviewer/server/internal/notebookcache"
)

type notebookContextKey struct{}
type notebookReasonKey struct{}
type notebookAttempt struct {
	policy       *notebookPolicy
	root         string
	seal         EnvironmentSeal
	identity     string
	expected     map[string]string
	addressSpace uint64
	status       string
}

func (s *Sandbox) prepareNotebook(ctx context.Context, python, script, inputDir string) (*notebookAttempt, error) {
	policy, err := notebookDeclaration(script)
	if err != nil {
		return nil, failure(422, err.Error())
	}
	if policy == nil {
		return nil, nil
	}
	seal, err := s.auditRuntime(ctx, python, script)
	if err != nil {
		return nil, err
	}
	inputs, _ := ctx.Value(inputContextKey{}).(map[string]string)
	identity, expected, err := policy.compatibility(script, seal.Digest, inputs)
	if err != nil {
		return nil, failure(422, err.Error())
	}
	root, err := os.MkdirTemp("", "sourceaec-notebook-")
	if err != nil {
		return nil, err
	}
	status := "missing"
	if reason, ok := ctx.Value(notebookReasonKey{}).(string); ok {
		status = reason
	}
	if policy.Reuse == "never" {
		status = "disabled"
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(root)
		}
	}()
	previous, available := ctx.Value(notebookContextKey{}).(notebookcache.Snapshot)
	if available && policy.Reuse == "declared" && previous.Identity != identity {
		status = "incompatible"
	}
	if available && policy.Reuse == "declared" && previous.Identity == identity {
		status = "restored"
		if err = notebookcache.Restore(ctx, root, policy.roots(), previous); err != nil {
			status = "corrupt"
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err = os.RemoveAll(root); err != nil {
				return nil, err
			}
			if err = os.Mkdir(root, 0700); err != nil {
				return nil, err
			}
		}
	}
	for _, prefix := range policy.roots() {
		for _, base := range []string{root, inputDir} {
			if err = os.MkdirAll(filepath.Join(base, filepath.FromSlash(prefix)), 0700); err != nil {
				return nil, err
			}
		}
	}
	success = true
	return &notebookAttempt{policy: policy, root: root, seal: seal, identity: identity, expected: expected, addressSpace: s.NotebookAddressSpace, status: status}, nil
}
func (n *notebookAttempt) mounts() []string {
	var args []string
	for _, prefix := range n.policy.roots() {
		args = append(args, "--bind", filepath.Join(n.root, filepath.FromSlash(prefix)), "/inputs/"+prefix)
	}
	return append(args, "--setenv", "SOURCEAEC_NOTEBOOK_REPORT", "/work/notebook-report.json")
}
func (n *notebookAttempt) finish(ctx context.Context, work string, product *Product) error {
	report, err := readProduct(filepath.Join(work, "notebook-report.json"), 1<<20)
	if err != nil {
		return failure(422, "notebook completion report missing or invalid")
	}
	if err = validateNotebookReport(report, n.expected); err != nil {
		return failure(422, err.Error())
	}
	after, err := auditEnvironment(ctx, n.seal.Python)
	if err != nil {
		return err
	}
	after = notebookEnvironment(after, n.addressSpace)
	if after != n.seal {
		return failure(503, "notebook geometry environment changed during execution")
	}
	product.Environment = n.seal.Digest
	product.NotebookReport = report
	product.NotebookStatus = n.status
	if n.policy.Reuse == "declared" {
		value, err := notebookcache.Pack(ctx, n.root, n.policy.roots(), n.identity)
		if err != nil {
			return failure(422, fmt.Sprintf("invalid completed notebook cache: %v", err))
		}
		product.NotebookCache = &value
	}
	return nil
}
