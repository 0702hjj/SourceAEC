// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// tools_locate.go：选中定位/标量改写域工具（从 tools.go 拆出，W-0057 行数门控）——
// get_script_locate（XDATA key → 脚本调用点 line/col/snippet，M3-①）+
// edit_script_call（libcst 标量改写定位到的调用点实参）。
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/cloudwego/eino/components/tool"
)

// locateTools 返回选中定位/标量改写域工具（2 个）：
// edit-call 仅在 edit-service 直连暴露，走 slow client（沙箱 run + staging.push）。
func locateTools(deps ToolDeps) []tool.InvokableTool {
	return []tool.InvokableTool{
		mustTool("get_script_locate", "XDATA key → 脚本调用点定位（line/col/snippet）——选中构件后定位到创建它的脚本位置（M3-①）",
			func(ctx context.Context, in locateReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				return toolRaw(cl.Do(ctx, http.MethodGet, "/models/"+m.ID+"/script/locate?key="+url.QueryEscape(in.Key), nil))
			}),

		mustTool("edit_script_call", "libcst 标量改写定位到的调用点实参（key+argument+value；沙箱 run + staging.push，422/409 零副作用）",
			func(ctx context.Context, in editCallReq) (string, error) {
				m, cl, errText := deps.resolve(ctx, in.ModelID)
				if errText != "" {
					return errText, nil
				}
				body, _ := json.Marshal(map[string]any{
					"key": in.Key, "argument": in.Argument, "value": json.RawMessage(in.Value),
				})
				return toolRaw(cl.DoSlow(ctx, http.MethodPost, "/models/"+m.ID+"/script/edit-call", body))
			}),
	}
}
