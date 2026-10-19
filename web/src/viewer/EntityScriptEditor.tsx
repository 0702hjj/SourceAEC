// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import { useEffect, useState } from "react";
import { editScriptCall, locateScript } from "@/api/client";
import type { ScriptLocateResult } from "@/api/types";
import { useViewerStore } from "./store";

export function EntityScriptEditor({ modelId, guid }: { modelId: string; guid: string }) {
  const [located, setLocated] = useState<ScriptLocateResult | null>(null);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [modes, setModes] = useState<Record<string, "parameter" | "override">>({});
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  useEffect(() => { setLocated(null); setMessage(""); }, [modelId, guid]);

  const load = async () => {
    setBusy(true); setMessage("");
    try {
      const result = await locateScript(modelId, guid);
      setLocated(result);
      setDrafts(Object.fromEntries((result.arguments ?? []).map((a) => [a.argument, String(a.value)])));
      setModes(Object.fromEntries((result.arguments ?? []).map((a) => [a.argument, a.paramPath ? "parameter" : "override"])));
      if (!result.arguments?.length) setMessage("该构件没有可直接编辑的参数，请定位脚本修改");
    } catch (e) { setMessage((e as Error).message); }
    finally { setBusy(false); }
  };

  const apply = async (argument: NonNullable<ScriptLocateResult["arguments"]>[number]) => {
    if (!located?.designKey || !located.revision) return;
    setBusy(true); setMessage("");
    try {
      const raw = drafts[argument.argument];
      let value: string | number | boolean = raw;
      if (typeof argument.value === "number") {
        value = Number(raw);
        if (!raw.trim() || !Number.isFinite(value)) throw new Error("请输入有效数字");
      } else if (typeof argument.value === "boolean") value = raw === "true";
      await editScriptCall(modelId, {
        designKey: located.designKey, argument: argument.argument, value,
        baseRevision: located.revision, mode: modes[argument.argument],
      });
      useViewerStore.getState().flagPendingModelReload();
      window.dispatchEvent(new Event("sourceaec:script-changed"));
      setLocated(null);
      setMessage("修改已生成预览；请在脚本参数面板保存版本");
    } catch (e) { setMessage((e as Error).message); }
    finally { setBusy(false); }
  };

  return <section className="entity-script-editor">
    <button disabled={busy} onClick={load}>编辑构件参数</button>
    {located?.arguments?.map((argument) => <div key={argument.argument}>
      <label>{argument.argument}
        {typeof argument.value === "boolean" ? <select aria-label={argument.argument}
          value={drafts[argument.argument]} onChange={(e) => setDrafts({ ...drafts, [argument.argument]: e.target.value })}>
          <option value="true">true</option><option value="false">false</option>
        </select> : <input aria-label={argument.argument} value={drafts[argument.argument]}
          onChange={(e) => setDrafts({ ...drafts, [argument.argument]: e.target.value })} />}
      </label>
      {argument.paramPath && <label>修改范围
        <select value={modes[argument.argument]} onChange={(e) => setModes({ ...modes,
          [argument.argument]: e.target.value as "parameter" | "override" })}>
          <option value="parameter">共享参数 {argument.paramPath.join(".")}</option>
          <option value="override">仅覆盖此调用</option>
        </select>
      </label>}
      <button disabled={busy} onClick={() => apply(argument)}>应用并预览</button>
    </div>)}
    {message && <p role="status">{message}</p>}
  </section>;
}
