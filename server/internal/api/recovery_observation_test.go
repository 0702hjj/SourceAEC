// SPDX-License-Identifier: Apache-2.0
package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStableBuildObservation(t *testing.T) {
	cases := []struct {
		name, before, after, run string
		status                   int
		want                     string
	}{
		{"success", `{"code":0,"data":{"revision":"one"}}`, `{"code":0,"data":{"revision":"one"}}`, `{"code":0,"data":{}}`, 200, ""},
		{"source changed", `{"code":0,"data":{"revision":"one"}}`, `{"code":0,"data":{"revision":"two"}}`, `{"code":0}`, 200, "revision changed"},
		{"overloaded", `{"code":0,"data":{"revision":"one"}}`, "", `{"code":429}`, 429, "run returned 429"},
		{"error envelope", `{"code":0,"data":{"revision":"one"}}`, "", `{"code":422}`, 200, "run returned 200"},
		{"missing run code", `{"code":0,"data":{"revision":"one"}}`, `{"code":0,"data":{"revision":"one"}}`, `{}`, 200, "run returned 200"},
		{"null run envelope", `{"code":0,"data":{"revision":"one"}}`, `{"code":0,"data":{"revision":"one"}}`, `null`, 200, "run returned 200"},
		{"missing source code", `{"data":{"revision":"one"}}`, `{"code":0,"data":{"revision":"one"}}`, `{"code":0}`, 200, "read stable source"},
		{"invalid source", "invalid", "", `{"code":0}`, 200, "read stable source"},
		{"missing revision", `{"code":0,"data":{}}`, "", `{"code":0}`, 200, "read stable source"},
		{"invalid after", `{"code":0,"data":{"revision":"one"}}`, "invalid", `{"code":0}`, 200, "read stable source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.run)
					return
				}
				reads++
				if reads == 1 {
					fmt.Fprint(w, tc.before)
				} else {
					fmt.Fprint(w, tc.after)
				}
			}))
			defer server.Close()
			samples := &httpSamples{client: server.Client(), base: server.URL, kinds: map[string]string{"m_0000000000000001": "dxf"}}
			err := runStableBuild(samples, "m_0000000000000001")
			if tc.want == "" {
				if err != nil || reads != 2 || samples.buildOK != 1 {
					t.Fatalf("success observation: %v reads=%d builds=%d", err, reads, samples.buildOK)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
