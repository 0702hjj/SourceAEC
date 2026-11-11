// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestInspectionSTEPCommentsAreLexicalWhitespace(t *testing.T) {
	plain := fixtureIFC("wall /* literal */; (a) it's")
	plain = []byte(strings.ReplaceAll(string(plain), "it's", "it''s"))
	commented := strings.ReplaceAll(string(plain), "#1=IFCWALL(", "/* unbalanced ' (); #9=BAD; */ #1 /* = ignored */ = /* ; */ IFCWALL(")
	commented = strings.ReplaceAll(commented, ",$", ",/* ' ) ; */$")
	commented = "/* preamble */" + commented + "/* trailing */"
	a, err := ifcEntities(plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ifcEntities([]byte(commented))
	if err != nil {
		t.Fatal("legal comments changed STEP structure", err)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("comment changed signatures: %s / %s", ja, jb)
	}
	for _, text := range []string{string(plain) + "/*", "ISO-10303-21;/* END-ISO-10303-21;"} {
		if _, err := parseSTEP([]byte(text)); err == nil {
			t.Fatal("unterminated comment accepted")
		}
	}
}

func numericDXF(kind, fields string) []byte {
	return []byte("0\nSECTION\n2\nENTITIES\n0\n" + kind + "\n" + fields + "0\nENDSEC\n0\nEOF\n")
}

func TestInspectionDXFRejectsNonFiniteGeometry(t *testing.T) {
	insert := "0\nSECTION\n2\nBLOCKS\n0\nBLOCK\n2\nB\n0\nLINE\n10\n1e308\n20\n1\n11\n1\n21\n2\n0\nENDBLK\n0\nENDSEC\n0\nSECTION\n2\nENTITIES\n0\nINSERT\n2\nB\n41\n10\n42\n10\n0\nENDSEC\n0\nEOF\n"
	cases := map[string][]byte{
		"insert-overflow": []byte(insert),
		"bounds-overflow": numericDXF("CIRCLE", "10\n1e308\n20\n0\n40\n1e308\n"),
		"bulge-overflow":  numericDXF("LWPOLYLINE", "10\n0\n20\n0\n42\n1e308\n10\n1\n20\n1\n"),
	}
	for _, raw := range []string{"NaN", "Inf", "-Inf", "1e999", "not-a-number"} {
		cases[raw] = numericDXF("LINE", "10\n"+raw+"\n20\n0\n11\n1\n21\n1\n")
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if render, err := renderDXF(data); err == nil || render != nil {
				t.Fatalf("nonfinite or invalid geometry accepted: %v / %v", render, err)
			}
		})
	}
	if _, err := renderDXF(numericDXF("LINE", "10\n1e100\n20\n-1e100\n11\n0\n21\n0\n")); err != nil {
		t.Fatal("large finite coordinates rejected", err)
	}
}

type fixedProductExecutor struct{ data []byte }

func (e fixedProductExecutor) Run(context.Context, string, string) (Product, error) {
	return Product{Data: e.data}, nil
}

func TestInspectionDXFNumericFailureDoesNotPublish(t *testing.T) {
	s, _ := testService(t, "dxf")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'x': 1}\n"})
	invoke(t, s, "POST", "script/save", map[string]any{})
	old := invoke(t, s, "GET", "versions", nil)
	s.Executor = fixedProductExecutor{numericDXF("CIRCLE", "10\n1e308\n20\n0\n40\n1e308\n")}
	_, err := s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", []byte(`{}`))
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 422 {
		t.Fatal("invalid candidate not rejected", err)
	}
	if got := invoke(t, s, "GET", "versions", nil); fmt.Sprint(got) != fmt.Sprint(old) {
		t.Fatal("failed save changed versions", old, got)
	}
}

func TestInspectionJSONFailurePreservesPreviousFile(t *testing.T) {
	files := map[string][]byte{"render.json": []byte(`{"schemaVersion":2}`)}
	for _, value := range []any{map[string]any{"coordinate": math.Inf(1)}, func() {}} {
		if err := addJSONFile(files, "render.json", value); err == nil {
			t.Fatal("encoding failure ignored")
		}
		if string(files["render.json"]) != `{"schemaVersion":2}` {
			t.Fatal("failed encoding replaced previous file")
		}
	}
}
