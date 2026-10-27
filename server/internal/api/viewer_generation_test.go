// SPDX-License-Identifier: Apache-2.0
package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestViewerAssetsRequireCurrentGeneration(t *testing.T) {
	srv, st := setup(t)
	m, err := st.Create("synthetic.ifc", 3, strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	publishTestViewer(t, st, m.ID)
	before, err := st.Get(m.ID)
	if err != nil || before.PreviewGeneration == "" {
		t.Fatal(before, err)
	}
	check := func(file, generation string, want int) {
		t.Helper()
		resp, err := http.Get(srv.URL + "/v1/models/" + m.ID + "/" + file + "?generation=" + generation)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s generation=%q: status=%d want=%d", file, generation, resp.StatusCode, want)
		}
	}
	for _, file := range []string{"model.xkt", "metadata.json"} {
		check(file, before.PreviewGeneration, http.StatusOK)
		check(file, strings.Repeat("a", 32), http.StatusNotFound)
	}
	changeTestSource(t, st, m.ID, "new")
	for _, file := range []string{"model.xkt", "metadata.json"} {
		check(file, before.PreviewGeneration, http.StatusNotFound)
		check(file, "", http.StatusNotFound)
	}
	publishTestViewer(t, st, m.ID)
	after, err := st.Get(m.ID)
	if err != nil || after.PreviewGeneration == before.PreviewGeneration {
		t.Fatal(after, err)
	}
	for _, file := range []string{"model.xkt", "metadata.json"} {
		check(file, before.PreviewGeneration, http.StatusNotFound)
		check(file, after.PreviewGeneration, http.StatusOK)
	}
}
