// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

import { describe, it, expect, afterEach, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

const api = vi.hoisted(() => ({
  listModels: vi.fn(async () => [] as unknown[]),
  uploadModel: vi.fn(),
  deleteModel: vi.fn(),
  retryModel: vi.fn(),
  createChatProject: vi.fn(),
  fetchModel: vi.fn(),
  downloadUrl: (id: string) => `/api/v1/models/${id}/download`,
  listChatSessions: vi.fn(async () => [] as unknown[]),
  createChatSessionByProject: vi.fn(),
  deleteChatProject: vi.fn(),
  getChatProjectDetail: vi.fn(),
}));
vi.mock("@/api/client", () => api);

import LibraryPage from "./LibraryPage";

const model = (id: string, kind?: string) => ({
  id,
  name: id,
  size: 1,
  status: "ready",
  createdAt: "2026-08-13T00:00:00Z",
  error: "",
  ...(kind !== undefined ? { kind } : {}),
});

function renderPage() {
  return render(
    <MemoryRouter>
      <LibraryPage />
    </MemoryRouter>
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("LibraryPage kind badge", () => {
  it("shows DXF badge for kind=dxf models and IFC for kind-less models", async () => {
    api.listModels.mockResolvedValue([model("m_dxf", "dxf"), model("m_legacy")]);
    renderPage();
    await waitFor(() => expect(screen.getByText("DXF")).toBeTruthy());
    expect(screen.getByText("IFC")).toBeTruthy();
  });

  it("shows IFC badge for explicit kind=ifc models", async () => {
    api.listModels.mockResolvedValue([model("m_ifc", "ifc")]);
    renderPage();
    await waitFor(() => expect(screen.getByText("IFC")).toBeTruthy());
    expect(screen.queryByText("DXF")).toBeNull();
  });
});

// --- W-0054：项目卡片展开模型列表（调 GET /api/v1/chat/projects/{id}）三态 ---

const session = (projectId: string) => ({
  chatSessionId: "c_1",
  opencodeSessionId: "s_1",
  modelId: "",
  projectId,
  title: "P1",
  createdAt: "2026-09-18T00:00:00Z",
});

function renderWithSession(projectId = "p_1") {
  api.listChatSessions.mockResolvedValue([session(projectId)]);
  return renderPage();
}

describe("LibraryPage project models expand", () => {
  it("expands a project to list its models with kind and status", async () => {
    renderWithSession();
    api.getChatProjectDetail.mockResolvedValue({
      projectId: "p_1",
      title: "P1",
      kind: "cad",
      createdAt: "2026-09-18T00:00:00Z",
      models: [
        { id: "m_0000000000000001", kind: "dxf", name: "plan.dxf", status: "ready" },
        { id: "m_0000000000000002", kind: "ifc", name: "bim.ifc", status: "converting" },
      ],
    });
    await waitFor(() => expect(screen.getByText("P1")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "模型" }));
    await waitFor(() => expect(screen.getByText("plan.dxf")).toBeTruthy());
    expect(screen.getByText("bim.ifc")).toBeTruthy();
    // kind 徽标 + 状态标签随模型展示
    expect(screen.getByText("DXF")).toBeTruthy();
    expect(screen.getByText("转换中")).toBeTruthy();
    expect(api.getChatProjectDetail).toHaveBeenCalledWith("p_1");
  });

  it("shows empty tip for a blank project (no models)", async () => {
    renderWithSession();
    api.getChatProjectDetail.mockResolvedValue({
      projectId: "p_1",
      title: "P1",
      kind: "cad",
      models: [],
    });
    await waitFor(() => expect(screen.getByText("P1")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "模型" }));
    await waitFor(() => expect(screen.getByText("暂无模型（空白项目）")).toBeTruthy());
  });

  it("degrades to an error tip when detail loading fails", async () => {
    renderWithSession();
    api.getChatProjectDetail.mockRejectedValue(new Error("backend down"));
    await waitFor(() => expect(screen.getByText("P1")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "模型" }));
    await waitFor(() =>
      expect(screen.getByText(/模型加载失败：backend down/)).toBeTruthy()
    );
  });
});
