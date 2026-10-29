// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"testing"
)

func TestSourcePatchRequiresExactRevisionAndStagesAtomically(t *testing.T) {
	s, _ := testService(t, "ifc")
	source := "PARAMS = {'name':'before'}\n"
	invoke(t, s, "PUT", "script", map[string]any{"script": source})
	patch := map[string]any{"patch": []map[string]any{{"start": 18, "end": 24, "text": "after"}}, "baseRevision": digest([]byte(source))}
	invoke(t, s, "PUT", "script", patch)
	changed := invoke(t, s, "GET", "script", nil).(map[string]any)["script"]
	if changed != "PARAMS = {'name':'after'}\n" {
		t.Fatal(changed)
	}
	_, err := s.Do(context.Background(), "PUT", "/models/"+testModel+"/script", jsonBytes(patch))
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 409 {
		t.Fatalf("stale patch: %v", err)
	}
	delete(patch, "baseRevision")
	_, err = s.Do(context.Background(), "PUT", "/models/"+testModel+"/script", jsonBytes(patch))
	if !errors.As(err, &domain) || domain.Status != 422 {
		t.Fatalf("missing base: %v", err)
	}
	if got := invoke(t, s, "GET", "script", nil).(map[string]any)["script"]; got != changed {
		t.Fatal("failed patch changed staging")
	}
	invoke(t, s, "POST", "script/undo", nil)
	if got := invoke(t, s, "GET", "script", nil).(map[string]any)["script"]; got != source {
		t.Fatal("patch bypassed staging history")
	}
}
