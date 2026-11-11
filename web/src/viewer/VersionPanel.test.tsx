// SPDX-License-Identifier: Apache-2.0
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchGitState, fetchScript, changeGitBranch } from "@/api/client";
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

it("does not apply a branch mutation to a different model", async () => {
  let finish!: (value: Awaited<ReturnType<typeof changeGitBranch>>) => void;
  vi.mocked(fetchGitState).mockResolvedValue({ branch: "main", branches: ["main"], commits: [{ commit: "a", note: "saved", parents: [] }] });
  vi.mocked(fetchScript).mockResolvedValue({ revision: "revision" } as Awaited<ReturnType<typeof fetchScript>>);
  vi.mocked(changeGitBranch).mockReturnValueOnce(new Promise(resolve => { finish = resolve; }));
  const changed = vi.fn();
  window.addEventListener("sourceaec:script-changed", changed);
  try {
    const view = render(<VersionPanel modelId="m_0000000000000001" />);
    fireEvent.click(screen.getByRole("button", { name: "分支管理" }));
    await screen.findByText(/分支：main/);
    fireEvent.change(screen.getByLabelText("新分支"), { target: { value: "old-model-branch" } });
    fireEvent.click(screen.getByRole("button", { name: "创建并切换" }));
    await waitFor(() => expect(changeGitBranch).toHaveBeenCalled());
    view.rerender(<VersionPanel modelId="m_0000000000000002" />);
    await act(async () => { finish({ branch: "old-model-branch", branches: [], commits: [] }); });
    expect(screen.queryByText(/分支：old-model-branch/)).toBeNull();
    expect(changed).not.toHaveBeenCalled();
  } finally {window.removeEventListener("sourceaec:script-changed", changed); cleanup();}
});

it("does not submit a branch operation after its model's script read becomes obsolete", async () => {
  let finish!: (value: Awaited<ReturnType<typeof fetchScript>>) => void;
  vi.mocked(fetchGitState).mockResolvedValue({ branch: "main", branches: ["main"], commits: [{ commit: "a", note: "saved", parents: [] }] });
  vi.mocked(changeGitBranch).mockClear();
  vi.mocked(fetchScript).mockReturnValueOnce(new Promise(resolve => { finish = resolve; }));
  const view = render(<VersionPanel modelId="m_0000000000000001" />);
  try {
    fireEvent.click(screen.getByRole("button", { name: "分支管理" }));
    await screen.findByText(/分支：main/);
    fireEvent.change(screen.getByLabelText("新分支"), { target: { value: "obsolete" } });
    fireEvent.click(screen.getByRole("button", { name: "创建并切换" }));
    view.rerender(<VersionPanel modelId="m_0000000000000002" />);
    await act(async () => { finish({ revision: "old-revision" } as Awaited<ReturnType<typeof fetchScript>>); });
    expect(changeGitBranch).not.toHaveBeenCalled();
    expect(screen.queryByRole("alert")).toBeNull();
  } finally { cleanup(); }
});
