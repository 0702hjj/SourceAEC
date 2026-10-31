// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"errors"
	"testing"
)

func TestComparisonByteAdmissionAndFailureRelease(t *testing.T) {
	s, _ := testService(t, "ifc")
	// Leave less than one input available. The query must reject before the
	// product is read or parsed, regardless of the three count-based slots.
	release, ok := comparisonBytes.Try((512 << 20) - 1)
	if !ok {
		t.Fatal("budget already held")
	}
	_, err := s.Do(context.Background(), "POST", "/models/"+testModel+"/diff", jsonBytes(map[string]string{"base": "current", "target": "current"}))
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 429 {
		t.Fatalf("expected byte rejection: %v", err)
	}
	release()
	if _, err = s.Do(context.Background(), "POST", "/models/"+testModel+"/diff", jsonBytes(map[string]string{"base": "current", "target": "v999"})); err == nil {
		t.Fatal("accepted missing version")
	}
	all, ok := comparisonBytes.Try(512 << 20)
	if !ok {
		t.Fatal("failed comparison leaked a byte reservation")
	}
	all()
	if _, err = s.Do(context.Background(), "POST", "/models/"+testModel+"/diff", jsonBytes(map[string]string{"base": "current", "target": "current"})); err != nil {
		t.Fatal(err)
	}
}
