// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/change"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/editing"
	"ifcviewer/server/internal/editsvc"
	"ifcviewer/server/internal/issue"
	"ifcviewer/server/internal/override"
	"ifcviewer/server/internal/store"
)

const benchmarkToken = "synthetic-benchmark-token"

// BenchmarkNativeHTTP includes the authenticated gateway, real IFC/DXF sandbox,
// immutable save/Git publication, semantic comparison and real Node conversion.
// It is opt-in and creates only synthetic models in a dedicated test database.
func BenchmarkNativeHTTP(b *testing.B) {
	if os.Getenv("SOURCEAEC_BENCH_GEOMETRY") != "1" {
		b.Skip("set SOURCEAEC_BENCH_GEOMETRY=1 for real geometry")
	}
	if os.Getenv("VIEWER_TEST_PG_DSN") == "" || os.Getenv("VIEWER_TEST_PG_DSN") != os.Getenv("VIEWER_PG_DSN") {
		b.Fatal("set matching explicit dedicated PostgreSQL test DSNs")
	}
	for _, models := range []int{1, 32} {
		b.Run(fmt.Sprintf("models=%d", models), func(b *testing.B) {
			root := b.TempDir()
			if err := artifact.Open(root); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = artifact.Close(root) })
			st := store.NewStore(root)
			converter, err := filepath.Abs("../../../converter/convert.js")
			if err != nil {
				b.Fatal(err)
			}
			runner := &measuredConverter{runner: convert.ExecRunner{NodeBin: "node", Script: converter}}
			q := convert.NewQueue(st, runner, 2)
			b.Cleanup(func() {
				q.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if !q.WaitIdle(ctx) {
					b.Error("conversion did not drain")
				}
			})
			srv := httptest.NewServer(TokenAuth(benchmarkToken)(NewHandler(st, q, issue.NewFileStore(root), change.NewFileStore(root), override.NewFileStore(root), editsvc.NewLocal(root, "ifc"), editsvc.NewLocal(root, "dxf"), 1<<20)))
			b.Cleanup(srv.Close)
			transport := &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 64}
			b.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
			measured := &httpSamples{client: client, base: srv.URL, kinds: map[string]string{}}
			ids := make([]string, models)
			seeds := map[string][]byte{}
			for i := range ids {
				kind := "ifc"
				if i%2 == 1 {
					kind = "dxf"
				}
				if seeds[kind] == nil {
					product, e := editing.NewSandbox().Run(context.Background(), httpGeometryScript(kind), kind)
					if e != nil {
						b.Fatal(e)
					}
					seeds[kind] = product.Data
				}
				model, e := st.CreateWithKind("synthetic."+kind, int64(len(seeds[kind])), strings.NewReader(string(seeds[kind])), kind)
				if e != nil {
					b.Fatal(e)
				}
				ids[i] = model.ID
				measured.kinds[model.ID] = kind
				script := httpGeometryScript(kind)
				// Warm only two environments. All model sources and later requests still
				// pass through the native authenticated HTTP editing route.
				if status, _ := measured.request("PUT", "/api/v1/models/"+model.ID+"/script", map[string]any{"script": script}); status != 200 {
					b.Fatal("staging failed", status)
				}
				if i < 2 {
					if status, body := measured.request("POST", "/api/v1/models/"+model.ID+"/script/run", nil); status != 200 {
						b.Fatalf("warm execution: %d %s", status, body)
					}
				}
			}
			waitConversion(b, q)
			if runner.failed.Load() != 0 {
				b.Fatalf("warm Node conversion failed: %v", runner.lastError.Load())
			}
			measured.validatePreviews(b, st)
			measured.reset()
			db, err := artifact.Database(root)
			if err != nil {
				b.Fatal(err)
			}
			before := db.Stats()
			stages := readStageTotals()
			runs, ns := runner.count.Load(), runner.nanos.Load()
			stopRSS := benchmarkRSS(b)
			cpu := benchmarkCPU()
			var next atomic.Uint64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					n := next.Add(1) - 1
					id := ids[int(n+n/8)%models]
					path := "/api/v1/models/" + id
					switch n % 8 {
					case 0:
						measured.request("GET", path+"/script", nil)
					case 1:
						status, body := measured.request("GET", path+"/script", nil)
						if status != 200 {
							continue
						}
						var result struct {
							Data struct {
								Revision string `json:"revision"`
							} `json:"data"`
						}
						if json.Unmarshal(body, &result) != nil {
							b.Error("invalid gateway envelope")
							continue
						}
						measured.request("PUT", path+"/script", map[string]any{"baseRevision": result.Data.Revision, "params": map[string]any{"name": fmt.Sprintf("synthetic-%d", n), "x": float64(n%10 + 1)}})
					case 2:
						measured.request("POST", path+"/script/run", nil)
					case 3:
						measured.request("POST", path+"/script/save", map[string]any{"source": "ai", "note": "synthetic benchmark"})
					case 4:
						measured.request("GET", path+"/edit/versions", nil)
					case 5:
						measured.request("POST", path+"/edit/diff", map[string]any{"base": "current", "target": "current"})
					case 6:
						measured.request("GET", path, nil)
					case 7:
						measured.request("GET", path+"/download", nil)
					}
				}
			})
			// Include completion of accepted asynchronous conversion in wall throughput.
			waitConversion(b, q)
			b.StopTimer()
			rss := stopRSS()
			after := db.Stats()
			measured.report(b)
			b.ReportMetric(float64(after.WaitCount-before.WaitCount), "pool-waits")
			b.ReportMetric(float64((after.WaitDuration - before.WaitDuration).Nanoseconds()), "pool-wait-ns")
			b.ReportMetric(float64(rss), "sampled-tree-rss-bytes")
			b.ReportMetric(float64(benchmarkCPU()-cpu), "tree-cpu-ns")
			b.ReportMetric(float64(runner.count.Load()-runs), "conversions")
			b.ReportMetric(float64(runner.nanos.Load()-ns), "conversion-ns")
			b.ReportMetric(float64(runner.failed.Load()), "conversion-errors")
			if runner.failed.Load() != 0 {
				b.Errorf("Node conversion failed: %v", runner.lastError.Load())
			}
			reportStages(b, stages)
			if after.InUse != 0 {
				b.Errorf("database leases retained: %d", after.InUse)
			}
			measured.validatePreviews(b, st)
		})
	}
}

func httpGeometryScript(kind string) string {
	params := "PARAMS = {'name':'synthetic','x':1}\n"
	if kind == "dxf" {
		return params + "import ezdxf, sys\ndoc=ezdxf.new()\ndoc.modelspace().add_line((0,0),(PARAMS['x'],1))\ndoc.saveas(sys.argv[1])\n"
	}
	return params + `import ifcopenshell
import ifcopenshell.api as api
import sys
model = ifcopenshell.file(schema="IFC4")
project = api.run("root.create_entity", model, ifc_class="IfcProject", name="Synthetic")
api.run("unit.assign_unit", model)
context = api.run("context.add_context", model, context_type="Model")
body = api.run("context.add_context", model, context_type="Model", context_identifier="Body", target_view="MODEL_VIEW", parent=context)
site = api.run("root.create_entity", model, ifc_class="IfcSite", name="Synthetic site")
building = api.run("root.create_entity", model, ifc_class="IfcBuilding", name="Synthetic building")
storey = api.run("root.create_entity", model, ifc_class="IfcBuildingStorey", name="Synthetic storey")
api.run("aggregate.assign_object", model, products=[site], relating_object=project)
api.run("aggregate.assign_object", model, products=[building], relating_object=site)
api.run("aggregate.assign_object", model, products=[storey], relating_object=building)
wall = api.run("root.create_entity", model, ifc_class="IfcWall", name=PARAMS["name"])
for entity, guid in zip([project, site, building, storey, wall], ["0aaaaaaaaaaaaaaaaaaaaa", "1aaaaaaaaaaaaaaaaaaaaa", "2aaaaaaaaaaaaaaaaaaaaa", "3aaaaaaaaaaaaaaaaaaaaa", "0bbbbbbbbbbbbbbbbbbbbb"]):
    entity.GlobalId = guid
shape = api.run("geometry.add_wall_representation", model, context=body, length=PARAMS["x"], height=3., thickness=.2)
api.run("geometry.assign_representation", model, product=wall, representation=shape)
api.run("geometry.edit_object_placement", model, product=wall)
api.run("spatial.assign_container", model, products=[wall], relating_structure=storey)
model.write(sys.argv[1])
`
}

type measuredConverter struct {
	runner       convert.Runner
	count, nanos atomic.Int64
	failed       atomic.Int64
	lastError    atomic.Value
}

func (r *measuredConverter) Run(ctx context.Context, input, output string) error {
	start := time.Now()
	err := r.runner.Run(ctx, input, output)
	if err != nil {
		r.failed.Add(1)
		r.lastError.Store(err.Error())
	}
	r.count.Add(1)
	r.nanos.Add(int64(time.Since(start)))
	return err
}

func waitConversion(b *testing.B, q *convert.Queue) {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		pending, _ := q.Depth()
		if pending == 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			b.Fatal("accepted conversions did not finish")
		}
	}
}

func benchmarkRequest(client *http.Client, base, method, path string, body any) (int, []byte) {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(method, base+path, bytes.NewReader(data))
	if err != nil {
		return 0, nil
	}
	req.Header.Set("Authorization", "Bearer "+benchmarkToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		var network net.Error
		if errors.As(err, &network) && network.Timeout() {
			return 504, []byte(err.Error())
		}
		return 0, []byte(err.Error())
	}
	defer resp.Body.Close()
	output, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, []byte(err.Error())
	}
	return resp.StatusCode, output
}

// Cold means a fresh application environment; uv's wheel/interpreter caches
// remain enabled. Warm uses that exact same environment. No public network SLO.
func BenchmarkGeometryEnvironment(b *testing.B) {
	if os.Getenv("SOURCEAEC_BENCH_GEOMETRY") != "1" {
		b.Skip("real geometry benchmark disabled")
	}
	for _, kind := range []string{"ifc", "dxf"} {
		for _, warm := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/warm=%v", kind, warm), func(b *testing.B) {
				sandbox := editing.NewSandbox()
				sandbox.Python = ""
				sandbox.Timeout = 120 * time.Second
				script := httpGeometryScript(kind)
				if warm {
					sandbox.CacheDir = b.TempDir()
					if _, err := sandbox.Run(context.Background(), script, kind); err != nil {
						b.Fatal(err)
					}
				}
				stopRSS := benchmarkRSS(b)
				stages := readStageTotals()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if !warm {
						sandbox.CacheDir = b.TempDir()
					}
					if _, err := sandbox.Run(context.Background(), script, kind); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(stopRSS()), "sampled-tree-rss-bytes")
				reportStages(b, stages)
			})
		}
	}
}
