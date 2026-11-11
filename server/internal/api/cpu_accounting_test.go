// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"ifcviewer/server/internal/benchobserve"
	"ifcviewer/server/internal/editing"
)

func TestCgroupCPUIncludesIsolatedGeometry(t *testing.T) {
	if os.Getenv("SOURCEAEC_BENCH_CPU_CGROUP") != "1" || os.Getenv("SOURCEAEC_TEST_GEOMETRY") != "1" {
		t.Skip("dedicated cgroup and external geometry interpreter required")
	}
	counter, err := benchobserve.NewCPUCounter()
	if err != nil {
		t.Fatal(err)
	}
	before, err := counter.Read()
	if err != nil {
		t.Fatal(err)
	}
	script := `import math,time,sys,json
start=time.process_time()
x=0.0
while time.process_time()-start < 0.4:
    x+=math.sin(x+0.1)
used=int((time.process_time()-start)*1e9)
with open(sys.argv[1],"w") as f:
    f.write("0\nSECTION\n2\nENTITIES\n0\nLINE\n5\nAA\n8\n0\n10\n"+str(x)+"\n20\n0\n11\n1\n21\n1\n0\nENDSEC\n0\nEOF\n")
with open("/work/out.map.json","w") as f: json.dump({"child_cpu_ns":used},f)
`
	product, err := editing.NewSandbox().Run(context.Background(), script, "dxf")
	if err != nil {
		t.Fatal(err)
	}
	after, err := counter.Read()
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		CPU int64 `json:"child_cpu_ns"`
	}
	if err := json.Unmarshal(product.Map, &report); err != nil {
		t.Fatal(err)
	}
	if report.CPU < 400000000 || after-before+5000000 < report.CPU {
		t.Fatalf("CPU missing: child=%d cgroup=%d", report.CPU, after-before)
	}
	t.Logf("child CPU=%d ns; cgroup total delta=%d ns", report.CPU, after-before)
}
