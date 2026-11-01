// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ifcviewer/server/internal/artifact"
)

func TestInspectionNormalizerBudgets(t *testing.T) {
	for name, n := range map[string]ifcNormalizer{
		"calls":  {calls: 0, scanBytes: 100, outputBytes: 100},
		"scan":   {calls: 100, scanBytes: 1, outputBytes: 100},
		"output": {calls: 100, scanBytes: 100, outputBytes: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := n.normalize("'abc'", map[string]bool{}, 0); err == nil {
				t.Fatal("exhausted budget accepted")
			}
		})
	}
}

func TestInspectionSTEPStructure(t *testing.T) {
	var fields []string
	err := walkSTEP("'a;it''s', (1,(2,3))", ',', func(value string) error {
		fields = append(fields, value)
		return nil
	})
	if err != nil || !reflect.DeepEqual(fields, []string{"'a;it''s'", "(1,(2,3))"}) {
		t.Fatal(fields, err)
	}
	for _, raw := range []string{"'unterminated", "(1", ")", strings.Repeat("(", 129) + strings.Repeat(")", 129)} {
		if err := walkSTEP(raw, ';', func(string) error { return nil }); err == nil {
			t.Fatalf("invalid structure accepted: %q", raw)
		}
	}
}

func TestInspectionSTEPRecordAndFieldBudgets(t *testing.T) {
	data := []byte("ISO-10303-21;DATA;#1=IFCLABEL('a');#2=IFCLABEL('b');ENDSEC;END-ISO-10303-21;")
	if _, err := parseSTEPWithin(data, 2, 2); err != nil {
		t.Fatal("exact budget rejected", err)
	}
	for _, limits := range [][2]int{{1, 2}, {2, 1}} {
		if _, err := parseSTEPWithin(data, limits[0], limits[1]); err == nil {
			t.Fatal("exhausted parser budget accepted", limits)
		}
	}
}

func TestInspectionSTEPInvalidIdentifierDoesNotEchoLargeInput(t *testing.T) {
	data := []byte("ISO-10303-21;#" + strings.Repeat("9", 1<<20) + "=IFCLABEL('x');END-ISO-10303-21;")
	_, err := parseSTEP(data)
	if err == nil || len(err.Error()) > 256 {
		t.Fatal("large invalid identifier was echoed in the error")
	}
}

func TestInspectionDXFRecordBudget(t *testing.T) {
	if _, err := parseDXF([]byte(strings.Repeat("0\nEOF\n", inspectionObjects+1))); err == nil {
		t.Fatal("excessive DXF records accepted")
	}
}

func TestInspectionDXFPolylineBudgetBeforeExpansion(t *testing.T) {
	b := dxfRenderBudget{objects: 1, pairs: 100, bytes: inspectionDXFOutputBytes}
	entity := dxfEntity{Kind: "LWPOLYLINE", Pairs: []dxfPair{{10, "0"}, {10, "1"}}}
	if err := b.reserve(entity); err == nil || b.objects != 1 || b.pairs != 100 {
		t.Fatal("polyline reservation was not atomic", b, err)
	}
}

func TestInspectionDXFChargesParentLookupPerPrimitive(t *testing.T) {
	parent := dxfEntity{Kind: "INSERT", Pairs: make([]dxfPair, 10)}
	child := dxfEntity{Kind: "LWPOLYLINE", Pairs: []dxfPair{{10, "0"}, {10, "1"}, {10, "2"}}}
	b := dxfRenderBudget{objects: 10, pairs: 36, bytes: inspectionDXFOutputBytes}
	// Four child groups plus three parent scans cost 4 + 3*11 = 37.
	if err := b.reserveChild(child, parent); err == nil || b.pairs != 36 || b.objects != 10 {
		t.Fatal("repeated parent scans escaped the budget", b, err)
	}
	b.pairs = 37
	if err := b.reserveChild(child, parent); err != nil || b.pairs != 0 || b.objects != 7 {
		t.Fatal("exact transformed budget rejected", b, err)
	}
}

func TestInspectionDXFTextBudgetBeforeCopy(t *testing.T) {
	b := dxfRenderBudget{objects: 100, pairs: 100, bytes: 2048}
	entity := dxfEntity{Kind: "MTEXT", Pairs: []dxfPair{{1, strings.Repeat("<", 1024)}}}
	if err := b.reserve(entity); err == nil || b.bytes != 2048 || b.pairs != 100 {
		t.Fatal("MTEXT expansion escaped output reservation", b, err)
	}
}

func TestInspectionDXFLayerBudget(t *testing.T) {
	b := dxfRenderBudget{bytes: 2048}
	if err := b.reserveLayer(map[string]any{"name": strings.Repeat("<", 1024), "linetype": "solid"}); err == nil || b.bytes != 2048 {
		t.Fatal("layer text escaped output reservation", b, err)
	}
}

func TestInspectionIFCInverseReferenceBudget(t *testing.T) {
	id := "0aaaaaaaaaaaaaaaaaaaaa"
	records := map[string]stepRecord{
		"#1": {kind: "IFCWALL", fields: []string{"'" + id + "'"}},
		"#2": {kind: "IFCPROPERTYSET", fields: []string{"$", "$", "'Pset'", "$", "(" + strings.Repeat("#missing,", 1499) + "#missing)"}},
		"#3": {kind: "IFCRELDEFINESBYPROPERTIES", fields: []string{"$", "$", "$", "$", "(" + strings.Repeat("#1,", 1499) + "#1)", "#2"}},
	}
	n := ifcNormalizer{records: records, calls: 100000, scanBytes: inspectionScanBytes, outputBytes: inspectionOutputBytes}
	if err := attachIFCProperties(records, map[string]map[string]any{id: {}}, &n); err == nil {
		t.Fatal("dangling inverse references bypassed normalization budget")
	}
}

func TestInspectionIFCPropertyKeyBudget(t *testing.T) {
	id := "0aaaaaaaaaaaaaaaaaaaaa"
	records := map[string]stepRecord{
		"#1": {kind: "IFCWALL", fields: []string{"'" + id + "'"}},
		"#2": {kind: "IFCPROPERTYSET", fields: []string{"$", "$", "'Pset'", "$", "(#4)"}},
		"#3": {kind: "IFCRELDEFINESBYPROPERTIES", fields: []string{"$", "$", "$", "$", "(#1)", "#2"}},
		"#4": {kind: "IFCPROPERTYSINGLEVALUE", fields: []string{"'" + strings.Repeat("x", 100) + "'", "$", "42"}},
	}
	n := ifcNormalizer{records: records, calls: 100000, scanBytes: inspectionScanBytes, outputBytes: 50}
	if err := attachIFCProperties(records, map[string]map[string]any{id: {}}, &n); err == nil {
		t.Fatal("expanded property key escaped output budget")
	}
}

func TestInspectionDXFSignatureBudget(t *testing.T) {
	if _, err := dxfEntitiesWithin(fixtureDXF(1), 1); err == nil {
		t.Fatal("unfunded signature accepted")
	}
	if _, err := dxfEntitiesWithin(fixtureDXF(1), 1<<20); err != nil {
		t.Fatal("small signature rejected", err)
	}
}

func TestInspectionDXFBlockExpansionLimit(t *testing.T) {
	data := inspectionBlockDXF()
	if _, err := renderDXF(data); err == nil {
		t.Fatal("unbounded block expansion accepted")
	}
}

func inspectionBlockDXF() []byte {
	line := "0\nLINE\n10\n0\n20\n0\n11\n1\n21\n1\n"
	return []byte("0\nSECTION\n2\nBLOCKS\n0\nBLOCK\n2\nshared\n" +
		strings.Repeat(line, 400) + "0\nENDBLK\n0\nENDSEC\n0\nSECTION\n2\nENTITIES\n" +
		strings.Repeat("0\nINSERT\n2\nshared\n", 300) + "0\nENDSEC\n0\nEOF\n")
}

type inspectionExecutor struct{ data []byte }

func (e inspectionExecutor) Run(context.Context, string, string) (Product, error) {
	return Product{Data: e.data, Environment: "fixture"}, nil
}

func TestInspectionDXFRejectDoesNotPublish(t *testing.T) {
	s, _ := testService(t, "dxf")
	invoke(t, s, "PUT", "script", map[string]any{"script": "PARAMS = {'x':1}\n"})
	path := filepath.Join(s.Root, "uploads", testModel+".dxf")
	before, err := artifact.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Executor = inspectionExecutor{data: inspectionBlockDXF()}
	_, err = s.Do(context.Background(), "POST", "/models/"+testModel+"/script/save", nil)
	var domain *Error
	if !errors.As(err, &domain) || domain.Status != 422 {
		t.Fatal("expected 422", err)
	}
	after, err := artifact.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected expansion published product", err)
	}
}

func TestInspectionIFCExpandedOutputLimit(t *testing.T) {
	data := []byte("ISO-10303-21;DATA;#1=IFCWALL('0aaaaaaaaaaaaaaaaaaaaa',$," +
		strings.Repeat("#2,", 34) + "#2);#2=IFCLABEL('" + strings.Repeat("x", 2<<20) +
		"');ENDSEC;END-ISO-10303-21;")
	if _, err := ifcEntities(data); err == nil {
		t.Fatal("unbounded IFC signature expansion accepted")
	}
}

func TestInspectionIFCRootBudget(t *testing.T) {
	var source strings.Builder
	source.WriteString("ISO-10303-21;DATA;")
	for i := 0; i <= inspectionObjects; i++ {
		fmt.Fprintf(&source, "#%d=IFCWALL('%022d');", i+1, i)
	}
	source.WriteString("ENDSEC;END-ISO-10303-21;")
	if _, err := ifcEntities([]byte(source.String())); err == nil {
		t.Fatal("root-only records bypassed normalization call budget")
	}
}
