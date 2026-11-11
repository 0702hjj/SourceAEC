// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"ifcviewer/server/internal/benchobserve"
	"ifcviewer/server/internal/convert"
	"ifcviewer/server/internal/store"
)

// Producers have joined before this phase. Recovery has separate metrics and
// is excluded from mixed-load throughput. It must not rescue failed requests
// by hiding them in the successful-build count of the preceding timed phase.
func verifyStableBuilds(b *testing.B, source *httpSamples, ids []string, q *convert.Queue, st *store.Store) {
	b.Helper()
	roots, err := benchobserve.Roots()
	if err != nil {
		b.Fatal(err)
	}
	observe := benchobserve.Start(roots, 250*time.Millisecond)
	defer func() { observe() }()
	samples := &httpSamples{client: source.client, base: source.base, kinds: source.kinds}
	readCPU := benchmarkCPUReader(b)
	cpu := readCPU()
	began := time.Now()
	for _, id := range ids {
		if err := runStableBuild(samples, id); err != nil {
			b.Errorf("stable %s build: %v", source.kinds[id], err)
		}
	}
	waitConversion(b, q)
	samples.validatePreviews(b, st)
	b.ReportMetric(float64(time.Since(began).Nanoseconds()), "recovery-wall-ns")
	b.ReportMetric(float64(readCPU()-cpu), "recovery-tree-cpu-ns")
	summary := observe()
	b.ReportMetric(float64(summary.Peak.PSS), "recovery-sampled-pss-bytes")
	b.ReportMetric(float64(summary.Peak.FDs), "recovery-sampled-fds")
	b.ReportMetric(float64(summary.Errors), "recovery-observation-errors")
	b.ReportMetric(float64(samples.buildOK), "recovery-build-ok")
	if samples.buildOK != len(ids) {
		b.Errorf("stable builds: got %d, want %d", samples.buildOK, len(ids))
	}
	for _, kind := range []string{"ifc", "dxf"} {
		values := samples.buildLatencies[kind]
		b.ReportMetric(float64(len(values)), "recovery-"+kind+"-build-ok")
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		for _, p := range []int{50, 95, 99} {
			if len(values) > 0 {
				b.ReportMetric(float64(values[(len(values)-1)*p/100]), fmt.Sprintf("recovery-%s-p%d-ns", kind, p))
			}
		}
	}
}

func runStableBuild(samples *httpSamples, id string) error {
	path := "/api/v1/models/" + id + "/script"
	before, err := stableRevision(samples, path)
	if err != nil {
		return err
	}
	status, body := samples.request("POST", path+"/run", nil)
	var response struct {
		Code *int `json:"code"`
	}
	if status != 200 || json.Unmarshal(body, &response) != nil || (response.Code == nil || *response.Code != 0) {
		return fmt.Errorf("run returned %d: %s", status, body)
	}
	after, err := stableRevision(samples, path)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("source revision changed after producers stopped")
	}
	return nil
}

func stableRevision(samples *httpSamples, path string) (string, error) {
	status, body := samples.request("GET", path, nil)
	var response struct {
		Code *int `json:"code"`
		Data struct {
			Revision string `json:"revision"`
		} `json:"data"`
	}
	if status != 200 || json.Unmarshal(body, &response) != nil || (response.Code == nil || *response.Code != 0) || response.Data.Revision == "" {
		return "", fmt.Errorf("read stable source: %d %s", status, body)
	}
	return response.Data.Revision, nil
}
