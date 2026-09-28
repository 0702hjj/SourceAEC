// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_project_detail_test.go：W-0054 项目详情 REST 契约测试——
// GET /api/v1/chat/projects/{id}（envelope + 形状 + 404）。响应形状与
// POST /chat/projects（create）及 agent get_project_models 工具对齐：
// models 项 = {id, kind, name, status}（status 反查真实状态，非快照）。
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ifcviewer/server/internal/store"
)

// projectDetailResp 是 GET /api/v1/chat/projects/{id} 的 data 形状。
type projectDetailResp struct {
	ProjectID string           `json:"projectId"`
	Title     string           `json:"title"`
	Kind      string           `json:"kind"`
	CreatedAt string           `json:"createdAt"`
	Models    []store.ModelRef `json:"models"`
}

// doGetProjectDetail 调 GET /api/v1/chat/projects/{id}（走 mux，验方法路由注册）。
func doGetProjectDetail(t *testing.T, h *ChatHandler, pid string) (*httptest.ResponseRecorder, projectDetailResp) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/projects/"+pid, nil)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	var r projectDetailResp
	if rec.Code == http.StatusOK {
		var e struct {
			Code int          `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Code != 0 {
			t.Fatalf("envelope: %v code=%d body=%s", err, e.Code, rec.Body.String())
		}
		if err := json.Unmarshal(e.Data, &r); err != nil {
			t.Fatal(err)
		}
	}
	return rec, r
}

// mustCreateModelWithStatus 建平台模型并置状态（status 反查语义的夹具）。
func mustCreateModelWithStatus(t *testing.T, st *store.Store, name, kind, status string) *store.Model {
	t.Helper()
	m, err := st.CreateWithKind(name, 4, strings.NewReader("test"), kind)
	if err != nil {
		t.Fatal(err)
	}
	if status != m.Status {
		if err := st.SetStatus(m.ID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

// TestGetProjectDetail 有模型项目：元信息 + 模型列表（真实状态反查，非快照）。
func TestGetProjectDetail(t *testing.T) {
	dataDir := t.TempDir()
	st := store.NewStore(dataDir)
	ps := store.NewProjectStore(dataDir)
	h := newProjectChatHandler(t, st, ps)

	p, err := ps.CreateWithKind("详情项目", "cad->ifc")
	if err != nil {
		t.Fatal(err)
	}
	// 快照状态故意过时（"ready"），真实状态 converting——详情应反查真实值。
	m := mustCreateModelWithStatus(t, st, "骨架.ifc", store.KindIFC, "converting")
	if err := ps.AddModel(p.ID, m.ID, m.Kind, m.Name, "ready"); err != nil {
		t.Fatal(err)
	}

	rec, r := doGetProjectDetail(t, h, p.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if r.ProjectID != p.ID || r.Title != "详情项目" || r.Kind != "cad->ifc" {
		t.Fatalf("元信息 = %+v", r)
	}
	if r.CreatedAt == "" {
		t.Fatalf("createdAt 缺失: %+v", r)
	}
	if len(r.Models) != 1 {
		t.Fatalf("models = %+v, want 1 项", r.Models)
	}
	got := r.Models[0]
	if got.ID != m.ID || got.Kind != store.KindIFC || got.Name != "骨架.ifc" {
		t.Fatalf("model ref = %+v", got)
	}
	if got.Status != "converting" {
		t.Fatalf("status = %q, want converting（真实状态反查，非快照 ready）", got.Status)
	}
}

// TestGetProjectDetailEmpty 空白项目：models = []（空数组，非 null）。
func TestGetProjectDetailEmpty(t *testing.T) {
	dataDir := t.TempDir()
	ps := store.NewProjectStore(dataDir)
	h := newProjectChatHandler(t, store.NewStore(dataDir), ps)
	p, err := ps.CreateWithKind("空白项目", "cad")
	if err != nil {
		t.Fatal(err)
	}

	rec, r := doGetProjectDetail(t, h, p.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(r.Models) != 0 {
		t.Fatalf("models = %+v, want 空", r.Models)
	}
	// 契约形状：空也必须是 [] 而非 null（前端 map 安全）。
	if !strings.Contains(rec.Body.String(), `"models":[]`) {
		t.Fatalf("models 应序列化为空数组: %s", rec.Body.String())
	}
}

// TestGetProjectDetailNotFound 不存在 / 非法 id → 404 envelope（code=40400）。
func TestGetProjectDetailNotFound(t *testing.T) {
	h := newProjectChatHandler(t, store.NewStore(t.TempDir()), store.NewProjectStore(t.TempDir()))
	for _, pid := range []string{"p_0000000000000000", "not-a-pid"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/projects/"+pid, nil)
		rec := httptest.NewRecorder()
		h.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("pid=%s status = %d, want 404", pid, rec.Code)
		}
		var e struct {
			Code int    `json:"code"`
			Data any    `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Code != codeNotFound {
			t.Fatalf("envelope: %v code=%d body=%s（want code=%d）", err, e.Code, rec.Body.String(), codeNotFound)
		}
	}
}
