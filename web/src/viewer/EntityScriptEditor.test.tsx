// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { EntityScriptEditor } from "./EntityScriptEditor";
const api = vi.hoisted(() => ({ locateScript: vi.fn(), editScriptCall: vi.fn() }));
vi.mock("@/api/client", () => api);
afterEach(() => { cleanup(); vi.clearAllMocks(); });
describe("selected entity source editing", () => {
  it("keeps shared parameter bindings and submits the observed revision", async () => {
    api.locateScript.mockResolvedValue({ found: true, designKey: "wall", revision: "revision",
      arguments: [{ argument: "height", value: 3, paramPath: ["floor_height"] }] });
    api.editScriptCall.mockResolvedValue({ staged: 1 });
    render(<EntityScriptEditor modelId="m_1" guid="guid" />);
    fireEvent.click(screen.getByText("编辑构件参数"));
    fireEvent.change(await screen.findByLabelText("height"), { target: { value: "4" } });
    fireEvent.click(screen.getByText("应用并预览"));
    await waitFor(() => expect(api.editScriptCall).toHaveBeenCalledWith("m_1", {
      designKey: "wall", argument: "height", value: 4, baseRevision: "revision", mode: "parameter",
    }));
    expect(await screen.findByText(/修改已生成预览/)).toBeTruthy();
  });
  it("shows conflicts without claiming a preview was published", async () => {
    api.locateScript.mockResolvedValue({ found: true, designKey: "wall", revision: "old",
      arguments: [{ argument: "name", value: "Wall", paramPath: null }] });
    api.editScriptCall.mockRejectedValue(new Error("script revision changed"));
    render(<EntityScriptEditor modelId="m_1" guid="guid" />);
    fireEvent.click(screen.getByText("编辑构件参数"));
    await screen.findByLabelText("name");
    fireEvent.click(screen.getByText("应用并预览"));
    expect(await screen.findByText("script revision changed")).toBeTruthy();
    expect(screen.queryByText(/修改已生成预览/)).toBeNull();
  });
});
