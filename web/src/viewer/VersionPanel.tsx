// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import { useEffect, useState } from "react";
import { fetchGitState, fetchScript, changeGitBranch } from "@/api/client";
import type { GitState } from "@/api/types";
import { useViewerStore } from "./store";

export function VersionPanel({ modelId }: { modelId: string }) {
  const [state, setState] = useState<GitState | null>(null);
  const [branch, setBranch] = useState("");
  const [target, setTarget] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState(false);
  useEffect(() => {
    if (!expanded) return;
    let active = true;
    let generation = 0;
    setState(null); setError("");
    const refresh = () => {
      const request = ++generation;
      void fetchGitState(modelId).then((value) => {
        if (active && request === generation) { setState(value); setError(""); }
      }).catch((e: Error) => { if (active && request === generation) setError(e.message); });
    };
    refresh();
    window.addEventListener("sourceaec:script-changed", refresh);
    return () => { active = false; window.removeEventListener("sourceaec:script-changed", refresh); };
  }, [expanded, modelId]);
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
  return <section aria-label="分支管理">
    <button aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>分支管理</button>
    {expanded && <>
    {!state && !error && <p>正在读取分支…</p>}
    {state && <>
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
    </>}
    {error && <p role="alert">{error}</p>}
    </>}
  </section>;
}
