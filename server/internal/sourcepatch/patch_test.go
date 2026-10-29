// SPDX-License-Identifier: Apache-2.0
package sourcepatch

import (
	"encoding/json"
	"testing"
)

func TestPatchExactRangesAndLimits(t *testing.T) {
	got, err := Apply("a中文z", []Edit{{1, 4, "新"}, {7, 8, "!"}}, 32)
	if err != nil || got != "a新文!" {
		t.Fatal(got, err)
	}
	for _, edits := range [][]Edit{
		nil, {{-1, 0, ""}}, {{3, 2, ""}}, {{0, 99, ""}}, {{1, 2, ""}},
		{{0, 3, ""}, {2, 4, ""}}, {{4, 4, "x"}, {1, 1, "x"}}, {{0, 0, "x"}, {0, 0, "y"}},
		{{0, 0, string([]byte{255})}},
	} {
		if _, err := Apply("a中文z", edits, 32); err == nil {
			t.Fatalf("accepted %+v", edits)
		}
	}
	if _, err := Apply("abc", []Edit{{0, 0, "too large"}}, 4); err == nil {
		t.Fatal("accepted oversized result")
	}
	if _, err := Apply("abc", make([]Edit, 129), 100); err == nil {
		t.Fatal("accepted too many edits")
	}
	got, err = Apply("abc", []Edit{{0, 3, ""}}, 0)
	if err != nil || got != "" {
		t.Fatal(got, err)
	}
}

func TestPatchJSONRequiresExplicitBoundsAndText(t *testing.T) {
	for _, value := range []string{`null`, `{}`, `{"text":"insert"}`, `{"start":0,"end":1}`, `{"start":null,"end":1,"text":""}`, `{"start":0,"end":1,"text":"","unknown":true}`} {
		var e Edit
		if err := json.Unmarshal([]byte(value), &e); err == nil {
			t.Fatal("accepted ambiguous patch", value)
		}
	}
	var e Edit
	if err := json.Unmarshal([]byte(`{"start":0,"end":0,"text":""}`), &e); err != nil {
		t.Fatal(err)
	}
}
