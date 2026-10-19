// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import { useCallback, useEffect, useState } from "react";
import { fetchGitState, fetchScript, changeGitBranch } from "@/api/client";
import type { GitState } from "@/api/types";
import { useViewerStore } from "./store";

export function VersionPanel({ modelId }: { modelId: string }) {
  const [state, setState] = useState<GitState | null>(null);
  const [branch, setBranch] = useState("");
  const [target, setTarget] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const refresh = useCallback(() => Promise.resolve().then(() => fetchGitState(modelId))
    .then(setState).catch(() => setState(null)), [modelId]);
  useEffect(() => {
    refresh();
    window.addEventListener("sourceaec:script-changed", refresh);
    return () => window.removeEventListener("sourceaec:script-changed", refresh);
  }, [refresh]);
  const change = async (action: "branches" | "checkout" | "merge", name: string) => {
    setBusy(true); setError("");
    try {
      const script = await fetchScript(modelId);
      if (!script.revision) throw new Error("请刷新脚本后重试");
      setState(await changeGitBranch(modelId, action, name, script.revision));
      useViewerStore.getState().flagPendingModelReload();
      window.dispatchEvent(new Event("sourceaec:script-changed"));
    } catch (e) { setError((e as Error).message); }
    finally { setBusy(false); }
  };
  if (!state) return null;
  return <section aria-label="Git 版本管理">
    <p>分支：{state.branch} {state.commit?.slice(0, 8)}</p>
    {state.commits.length === 0 ? <p>保存首个版本后可创建分支</p> : <>
      <label>新分支<input value={branch} onChange={(e) => setBranch(e.target.value)} /></label>
      <button disabled={busy || !branch} onClick={() => change("branches", branch)}>创建并切换</button>
      <label>目标分支<select value={target} onChange={(e) => setTarget(e.target.value)}>
        <option value="">选择分支</option>
        {state.branches.filter((b) => b !== state.branch).map((b) => <option key={b}>{b}</option>)}
      </select></label>
      <button disabled={busy || !target} onClick={() => change("checkout", target)}>切换分支</button>
      <button disabled={busy || !target} onClick={() => change("merge", target)}>合并到当前分支</button>
    </>}
    {error && <p role="alert">{error}</p>}
  </section>;
}
