// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestNotebookPolicyCompatibilityAndIsolation(t *testing.T) {
	policy, err := notebookDeclaration(notebookWrapper)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]string{"skill-work/part.py": base64.StdEncoding.EncodeToString([]byte("cell one")), "skill-work/data.json": base64.StdEncoding.EncodeToString([]byte("input one"))}
	first, _, err := policy.compatibility(notebookWrapper, "seal", inputs)
	if err != nil {
		t.Fatal(err)
	}
	inputs["skill-work/part.py"] = base64.StdEncoding.EncodeToString([]byte("cell two"))
	partial, _, _ := policy.compatibility(notebookWrapper, "seal", inputs)
	if partial != first {
		t.Fatal("notebook code edits cannot reach SDK invalidation")
	}
	inputs["skill-work/data.json"] = base64.StdEncoding.EncodeToString([]byte("input two"))
	changed, _, _ := policy.compatibility(notebookWrapper, "seal", inputs)
	environment, _, _ := policy.compatibility(notebookWrapper, "another seal", inputs)
	if changed == first || environment == changed {
		t.Fatal("input/environment drift did not invalidate cache")
	}
	inputs["skill-work/__marimo__/uploaded.pkl"] = base64.StdEncoding.EncodeToString([]byte("untrusted"))
	if _, _, err := policy.compatibility(notebookWrapper, "seal", inputs); err == nil {
		t.Fatal("uploaded cache accepted")
	}
	for _, path := range []string{"../part.py", "skill-work/../part.py", "/absolute.py", "skill-work/a\\part.py", "skill-work/__marimo__/part.py"} {
		text := strings.Replace(notebookWrapper, "skill-work/part.py", path, 1)
		if _, err := notebookDeclaration(text); err == nil {
			t.Fatal("unsafe declaration accepted", path)
		}
	}
}
func TestNotebookReportRequiresCompleteBoundIdentities(t *testing.T) {
	hash := strings.Repeat("a", 64)
	good := fmt.Sprintf(`{"version":1,"notebooks":[{"path":"skill-work/part.py","sha256":"%s","definition":"sha256:%s","cells":[{"key":"c1","status":"ran"}]}]}`, hash, hash)
	expected := map[string]string{"skill-work/part.py": hash}
	if err := validateNotebookReport([]byte(good), expected); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{strings.Replace(good, "ran", "pending", 1), strings.Replace(good, "c1", "", 1), strings.Replace(good, "sha256:", "", 1), strings.Replace(good, `"version":1`, `"version":2`, 1), good + `{}`, `{"version":1,"notebooks":[]}`} {
		if err := validateNotebookReport([]byte(raw), expected); err == nil {
			t.Fatal("invalid completion accepted", raw)
		}
	}
}
func TestNotebookAddressSpaceDefaultsAndInvalidConfiguration(t *testing.T) {
	t.Setenv("SOURCEAEC_NOTEBOOK_AS_BYTES", "")
	if got := NewSandbox().NotebookAddressSpace; got != 1<<30 {
		t.Fatal("default isolation budget changed", got)
	}
	t.Setenv("SOURCEAEC_NOTEBOOK_AS_BYTES", "not-a-budget")
	if got := NewSandbox().NotebookAddressSpace; got != 0 {
		t.Fatal("invalid administrator setting silently fell back", got)
	}
}
