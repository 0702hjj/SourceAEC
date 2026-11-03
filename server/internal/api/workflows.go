// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"ifcviewer/server/internal/editing"
)

func writeWorkflowErr(w http.ResponseWriter, err error) {
	var domain *editing.Error
	if errors.As(err, &domain) {
		writeErr(w, domain.Status, domain.Status*100, domain.Detail)
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeErr(w, 504, 50400, "workflow operation timed out")
		return
	}
	writeErr(w, 500, codeInternal, "workflow operation failed")
}

func (h *handler) workflowService(w http.ResponseWriter, r *http.Request) *editing.Workflows {
	if h.modelOrErr(w, r.PathValue("id")) == nil {
		return nil
	}
	if h.workflows == nil {
		writeWorkflowErr(w, &editing.Error{Status: 503, Detail: "durable workflows unavailable"})
		return nil
	}
	return h.workflows
}
func (h *handler) createWorkflow(w http.ResponseWriter, r *http.Request) {
	service := h.workflowService(w, r)
	if service == nil {
		return
	}
	var in editing.WorkflowRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeWorkflowErr(w, &editing.Error{Status: 422, Detail: "invalid workflow request"})
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeWorkflowErr(w, &editing.Error{Status: 422, Detail: "request must contain one JSON value"})
		return
	}
	result, created, err := service.Submit(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeWorkflowErr(w, err)
		return
	}
	if created {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
	}
	writeJSON(w, result)
}
func (h *handler) getWorkflow(w http.ResponseWriter, r *http.Request) {
	service := h.workflowService(w, r)
	if service == nil {
		return
	}
	result, err := service.Get(r.Context(), r.PathValue("id"), r.PathValue("workflowId"))
	if err != nil {
		writeWorkflowErr(w, err)
		return
	}
	writeJSON(w, result)
}
func (h *handler) listWorkflows(w http.ResponseWriter, r *http.Request) {
	service := h.workflowService(w, r)
	if service == nil {
		return
	}
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 100 {
			writeWorkflowErr(w, &editing.Error{Status: 422, Detail: "workflow limit must be 1..100"})
			return
		}
		limit = n
	}
	result, err := service.List(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		writeWorkflowErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"workflows": result})
}
func (h *handler) controlWorkflow(w http.ResponseWriter, r *http.Request) {
	service := h.workflowService(w, r)
	if service == nil {
		return
	}
	result, err := service.Control(r.Context(), r.PathValue("id"), r.PathValue("workflowId"), r.PathValue("action"))
	if err != nil {
		writeWorkflowErr(w, err)
		return
	}
	writeJSON(w, result)
}
