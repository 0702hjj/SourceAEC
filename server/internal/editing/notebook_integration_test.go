// SPDX-License-Identifier: Apache-2.0
package editing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Synthetic user geometry, not a maintained platform runtime or SDK shim.
const notebookModel = `# /// script
# [tool.simplecadapi]
# id = "part"
# ///
import marimo
app = marimo.App()
with app.setup:
    import simplecadapi as scad
@app.cell
def _():
    width = 20.0
    return (width,)
@app.cell
def _():
    height = 3.0
    return (height,)
@app.cell
def _(width, height):
    body = scad.make_box_rsolid(width, height, 2.0)
    return (body,)
@app.cell
def _(body):
    part = scad.Part(part_id="part", body=body)
    return (part,)
if __name__ == "__main__":
    app.run()
`
const notebookWrapper = `# /// script
# dependencies = []
# [tool.sourceaec.notebook]
# adapter = "simplecadapi-v1"
# notebooks = ["skill-work/part.py"]
# reuse = "declared"
# ///
PARAMS = {}
import os, sys, json, hashlib
from pathlib import Path
import ezdxf
from simplecadapi.runtime import run_notebook
source = Path(os.environ["SOURCEAEC_INPUTS_DIR"]) / "skill-work/part.py"
run = run_notebook(source, cache=True)
from OCP.TopExp import TopExp, TopExp_Explorer
from OCP.TopAbs import TopAbs_EDGE
from OCP.TopoDS import TopoDS
from OCP.BRep import BRep_Tool
doc = ezdxf.new()
edge = TopExp_Explorer(run.product.body.wrapped, TopAbs_EDGE)
while edge.More():
    shape = TopoDS.Edge_s(edge.Current())
    a = BRep_Tool.Pnt_s(TopExp.FirstVertex_s(shape, True))
    b = BRep_Tool.Pnt_s(TopExp.LastVertex_s(shape, True))
    doc.modelspace().add_line((a.X(), a.Y()), (b.X(), b.Y()))
    edge.Next()
doc.modelspace().add_text("volume=" + format(run.product.body.get_volume(), ".12g"))
doc.saveas(sys.argv[1])
report = {"version": 1, "notebooks": [{"path": "skill-work/part.py", "sha256": hashlib.sha256(source.read_bytes()).hexdigest(), "definition": run.definition.content_hash, "cells": [{"key": c.key, "status": c.status} for c in run.cells]}]}
Path(os.environ["SOURCEAEC_NOTEBOOK_REPORT"]).write_text(json.dumps(report))
`

func notebookSandbox(t *testing.T) *Sandbox {
	t.Helper()
	python := os.Getenv("SOURCEAEC_TEST_NOTEBOOK_PYTHON")
	if python == "" {
		t.Skip("SOURCEAEC_TEST_NOTEBOOK_PYTHON with external SimpleCADAPI required")
	}
	s := NewSandbox()
	s.Python = python
	s.NotebookAddressSpace = 2 << 30
	return s
}
func notebookReportOf(t *testing.T, p Product) notebookReport {
	t.Helper()
	var result notebookReport
	if err := json.Unmarshal(p.NotebookReport, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func cachedCells(report notebookReport) int {
	n := 0
	for _, r := range report.Notebooks {
		for _, c := range r.Cells {
			if c.Status == "cached" {
				n++
			}
		}
	}
	return n
}
func TestNotebookRealColdHotAndPartialEquivalent(t *testing.T) {
	s := notebookSandbox(t)
	run := func(source, wrapper string, previous Product) Product {
		t.Helper()
		ctx := context.WithValue(context.Background(), inputContextKey{}, map[string]string{"skill-work/part.py": base64.StdEncoding.EncodeToString([]byte(source))})
		if previous.NotebookCache != nil {
			ctx = context.WithValue(ctx, notebookContextKey{}, *previous.NotebookCache)
		}
		result, err := s.Run(ctx, wrapper, "dxf")
		if err != nil {
			t.Fatal(err)
		}
		if err = validateProduct(result.Data, "dxf"); err != nil {
			t.Fatal(err)
		}
		return result
	}
	cold := run(notebookModel, notebookWrapper, Product{})
	if cold.NotebookCache == nil || cachedCells(notebookReportOf(t, cold)) != 0 {
		t.Fatal("cold notebook did not run")
	}
	hot := run(notebookModel, notebookWrapper, cold)
	if cachedCells(notebookReportOf(t, hot)) == 0 {
		t.Fatal("hot notebook did not restore SDK cells")
	}
	changed := strings.Replace(notebookModel, "width = 20.0", "width = 30.0", 1)
	partial := run(changed, notebookWrapper, hot)
	if cachedCells(notebookReportOf(t, partial)) == 0 {
		t.Fatal("unchanged independent cell was not restored")
	}
	full := run(changed, strings.Replace(notebookWrapper, "cache=True", "cache=False", 1), Product{})
	for _, pair := range [][2]Product{{cold, hot}, {partial, full}} {
		a, err := renderDXF(pair[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		b, err := renderDXF(pair[1].Data)
		if err != nil {
			t.Fatal(err)
		}
		ra, rb := notebookReportOf(t, pair[0]), notebookReportOf(t, pair[1])
		if !reflect.DeepEqual(a, b) || ra.Notebooks[0].Definition != rb.Notebooks[0].Definition {
			t.Fatal("cache changed geometry or SDK definition")
		}
	}
}

func TestNotebookRealConcurrentPrivateCopies(t *testing.T) {
	s := notebookSandbox(t)
	ctx := context.WithValue(context.Background(), inputContextKey{}, map[string]string{"skill-work/part.py": base64.StdEncoding.EncodeToString([]byte(notebookModel))})
	cold, err := s.Run(ctx, notebookWrapper, "dxf")
	if err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, notebookContextKey{}, *cold.NotebookCache)
	type result struct {
		product Product
		err     error
	}
	done := make(chan result, 2)
	for range 2 {
		go func() { p, err := s.Run(ctx, notebookWrapper, "dxf"); done <- result{p, err} }()
	}
	for range 2 {
		select {
		case got := <-done:
			if got.err != nil {
				t.Fatal(got.err)
			}
			if cachedCells(notebookReportOf(t, got.product)) == 0 {
				t.Fatal("concurrent SDK cache was cleared by another attempt")
			}
			if notebookReportOf(t, got.product).Notebooks[0].Definition != notebookReportOf(t, cold).Notebooks[0].Definition {
				t.Fatal("concurrent cache changed definition")
			}
		case <-time.After(90 * time.Second):
			t.Fatal("concurrent notebook execution did not finish")
		}
	}
}

func TestNotebookRealFrozenBudgetAndAdapter(t *testing.T) {
	s := notebookSandbox(t)
	ctx := context.WithValue(context.Background(), inputContextKey{}, map[string]string{"skill-work/part.py": base64.StdEncoding.EncodeToString([]byte(notebookModel))})
	seal, err := s.Freeze(ctx, notebookWrapper, "dxf")
	if err != nil {
		t.Fatal(err)
	}
	product, err := s.RunFrozen(ctx, notebookWrapper, "dxf", seal)
	if err != nil || product.Environment != seal.Digest {
		t.Fatal("frozen notebook environment mismatch", err)
	}
	s.NotebookAddressSpace = 3 << 30
	if _, err = s.RunFrozen(ctx, notebookWrapper, "dxf", seal); err == nil {
		t.Fatal("changed administrator budget reused old frozen definition")
	}
	engine, err := executionEngine(notebookWrapper)
	if err != nil || engine != notebookWorkflowEngine {
		t.Fatal(engine, err)
	}
	if engine, err = executionEngine("PARAMS = {}\n"); err != nil || engine != workflowEngine {
		t.Fatal("legacy whole-script adapter changed", engine, err)
	}
}

func TestNotebookRealModuleAndDeclaredInputInvalidation(t *testing.T) {
	s := notebookSandbox(t)
	for _, kind := range []string{"module", "input"} {
		t.Run(kind, func(t *testing.T) {
			model := notebookModel
			key, old, newValue := "skill-work/dims.py", "WIDTH = 20.0\n", "WIDTH = 30.0\n"
			code := "from dims import WIDTH\n    width = WIDTH"
			if kind == "input" {
				key, old, newValue = "skill-work/data.json", `{"width":20.0}`, `{"width":30.0}`
				code = "import json\n    from pathlib import Path\n    width = json.loads(Path('/inputs/skill-work/data.json').read_text())['width']"
			}
			model = strings.Replace(model, "width = 20.0", code, 1)
			run := func(value, wrapper string, previous Product) Product {
				t.Helper()
				ctx := context.WithValue(context.Background(), inputContextKey{}, map[string]string{"skill-work/part.py": base64.StdEncoding.EncodeToString([]byte(model)), key: base64.StdEncoding.EncodeToString([]byte(value))})
				if previous.NotebookCache != nil {
					ctx = context.WithValue(ctx, notebookContextKey{}, *previous.NotebookCache)
				}
				product, err := s.Run(ctx, wrapper, "dxf")
				if err != nil {
					t.Fatal(err)
				}
				return product
			}
			initial := run(old, notebookWrapper, Product{})
			changed := run(newValue, notebookWrapper, initial)
			if changed.NotebookStatus != "incompatible" || cachedCells(notebookReportOf(t, changed)) != 0 {
				t.Fatal("dependency change reused old cells", changed.NotebookStatus)
			}
			full := run(newValue, strings.Replace(notebookWrapper, "cache=True", "cache=False", 1), Product{})
			a, err := renderDXF(changed.Data)
			if err != nil {
				t.Fatal(err)
			}
			b, err := renderDXF(full.Data)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) || notebookReportOf(t, changed).Notebooks[0].Definition != notebookReportOf(t, full).Notebooks[0].Definition {
				t.Fatal("dependency invalidation differs from full execution")
			}
		})
	}
}
