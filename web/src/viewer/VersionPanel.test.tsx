// SPDX-License-Identifier: Apache-2.0
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchGitState } from "@/api/client";
import { VersionPanel } from "./VersionPanel";

vi.mock("@/api/client", () => ({ fetchGitState: vi.fn(), fetchScript: vi.fn(), changeGitBranch: vi.fn() }));
vi.mock("./store", () => ({ useViewerStore: { getState: () => ({ flagPendingModelReload: vi.fn() }) } }));

describe("on-demand branch history", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.mocked(fetchGitState).mockReset();
    vi.mocked(fetchGitState).mockResolvedValue({ branch: "main", branches: ["main"], commits: [] });
  });

  it("loads only while expanded, including script-change events", async () => {
    render(<VersionPanel modelId="m_0000000000000001" />);
    fireEvent(window, new Event("sourceaec:script-changed"));
    expect(fetchGitState).not.toHaveBeenCalled();
    const toggle = screen.getByRole("button", { name: "分支管理" });
    fireEvent.click(toggle);
    await screen.findByText(/分支：main/);
    expect(fetchGitState).toHaveBeenCalledTimes(1);
    fireEvent(window, new Event("sourceaec:script-changed"));
    await waitFor(() => expect(fetchGitState).toHaveBeenCalledTimes(2));
    fireEvent.click(toggle);
    fireEvent(window, new Event("sourceaec:script-changed"));
    expect(fetchGitState).toHaveBeenCalledTimes(2);
  });

  it("discards a result which arrives after collapsing", async () => {
    let resolve!: (value: Awaited<ReturnType<typeof fetchGitState>>) => void;
    vi.mocked(fetchGitState).mockReturnValueOnce(new Promise((done) => { resolve = done; }));
    render(<VersionPanel modelId="m_0000000000000001" />);
    const toggle = screen.getByRole("button", { name: "分支管理" });
    fireEvent.click(toggle); fireEvent.click(toggle);
    await act(async () => { resolve({ branch: "stale", branches: [], commits: [] }); });
    fireEvent.click(toggle);
    await screen.findByText(/分支：main/);
    expect(screen.queryByText(/stale/)).toBeNull();
  });

  it("shows a retryable load error instead of hiding branch controls", async () => {
    vi.mocked(fetchGitState).mockRejectedValueOnce(new Error("synthetic unavailable"));
    render(<VersionPanel modelId="m_0000000000000001" />);
    fireEvent.click(screen.getByRole("button", { name: "分支管理" }));
    expect((await screen.findByRole("alert")).textContent).toContain("synthetic unavailable");
  });

  it("keeps the latest refresh when responses arrive out of order", async () => {
    let finish!: (value: Awaited<ReturnType<typeof fetchGitState>>) => void;
    vi.mocked(fetchGitState).mockReturnValueOnce(new Promise((resolve) => { finish = resolve; }));
    render(<VersionPanel modelId="m_0000000000000001" />);
    fireEvent.click(screen.getByRole("button", { name: "分支管理" }));
    fireEvent(window, new Event("sourceaec:script-changed"));
    await screen.findByText(/分支：main/);
    await act(async () => { finish({ branch: "stale", branches: [], commits: [] }); });
    expect(screen.queryByText(/stale/)).toBeNull();
  });
});
