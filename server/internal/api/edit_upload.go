// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"io"
	"net/http"

	"ifcviewer/server/internal/artifact"
	"ifcviewer/server/internal/change"
	"ifcviewer/server/internal/editing"
)

// editUpload compares a human-modified document and records its provenance.
// The current model remains unchanged until a separate source edit is saved.
func (h *handler) editUpload(w http.ResponseWriter, r *http.Request) {
	model := h.modelOrErr(w, r.PathValue("id"))
	if model == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeErr(w, 400, codeInvalidType, "invalid upload")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, codeInvalidType, "file required")
		return
	}
	defer file.Close()
	target, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, 400, codeInvalidType, "cannot read upload")
		return
	}
	base, err := artifact.ReadFile(h.st.SourcePath(model))
	if err != nil {
		writeErr(w, 500, codeInternal, err.Error())
		return
	}
	diff, err := editing.Compare(base, target, model.Kind)
	if err != nil {
		writeErr(w, 400, codeInvalidType, err.Error())
		return
	}
	author := r.FormValue("author")
	if author == "" {
		author = "user-upload"
	}
	entries := []*change.Entry{}
	for _, action := range []string{"added", "removed"} {
		for _, id := range diff[action].([]string) {
			entries = append(entries, &change.Entry{EntityID: id, Field: "entity", Operation: action, Author: author, Provenance: change.Provenance{Source: "USER", Origin: model.Kind + "-upload"}})
		}
	}
	for _, entity := range diff["changed"].([]map[string]any) {
		id, _ := entity["guid"].(string)
		if id == "" {
			id, _ = entity["key"].(string)
		}
		for _, field := range entity["changes"].([]map[string]any) {
			old, _ := json.Marshal(field["old"])
			new, _ := json.Marshal(field["new"])
			entries = append(entries, &change.Entry{EntityID: id, Field: field["field"].(string), OldValue: string(old), NewValue: string(new), Operation: "update", Author: author, Provenance: change.Provenance{Source: "USER", Origin: model.Kind + "-upload"}})
		}
	}
	if len(entries) > 0 {
		if err = h.chg.Append(model.ID, entries...); err != nil {
			writeErr(w, 500, codeInternal, err.Error())
			return
		}
	}
	writeJSON(w, map[string]any{"modelId": model.ID, "diff": diff, "events": entries, "summary": map[string]int{"added": len(diff["added"].([]string)), "removed": len(diff["removed"].([]string)), "modified": len(diff["changed"].([]map[string]any))}})
}
