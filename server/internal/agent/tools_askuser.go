// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// tools_askuser.go：HITL 开放断点工具 ask_user（从 tools.go 拆出，W-0057
// 行数门控）——官方 FollowUpTool 模式对齐（M3）：挂 orchestrator + 子 agent，
// 模型缺信息/需确认时自主调用中断，/answer 经 Agent.Resume 续跑。
package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// --- HITL 开放断点：ask_user（官方 FollowUpTool 模式对齐，M3） -----------------

// AskUserInfo 是中断时呈现给用户的信息（翻译层据此发 question/ask 帧）；
// UserAnswer 由 /answer 经 Agent.Resume 填充。
type AskUserInfo struct {
	Question   string
	UserAnswer string
}

// AskUserState 是中断时保存的工具状态（恢复时读回问题，非目标中断时重新中断）。
type AskUserState struct {
	Question string
}

func init() {
	schema.Register[*AskUserInfo]()
	schema.Register[*AskUserState]()
}

type askUserReq struct {
	Question string `json:"question" jsonschema:"required,description=需要用户确认/补充的问题（自包含——用户只看到这个问题；如：'是否确认以 3 米层高保存？'）"`
}

// askUser 是 ask_user 工具的执行体（对齐官方 FollowUp 三分支：
// 首次中断 / resume 拿回答 / 非目标中断重新挂起）。
func askUser(ctx context.Context, in askUserReq) (string, error) {
	wasInterrupted, _, storedState := tool.GetInterruptState[*AskUserState](ctx)
	if !wasInterrupted {
		info := &AskUserInfo{Question: in.Question}
		return "", tool.StatefulInterrupt(ctx, info, &AskUserState{Question: in.Question})
	}

	isResumeTarget, hasData, data := tool.GetResumeContext[*AskUserInfo](ctx)
	if isResumeTarget && hasData {
		if data.UserAnswer == "" {
			return "", fmt.Errorf("ask_user resumed without a user answer")
		}
		return data.UserAnswer, nil
	}
	if !isResumeTarget {
		// 非目标中断（多断点场景）：重新挂起，保留原问题
		return "", tool.StatefulInterrupt(ctx, &AskUserInfo{Question: storedState.Question}, storedState)
	}
	return "", fmt.Errorf("ask_user resumed without data")
}

// AskUserTool 产出 ask_user 工具（挂 orchestrator + 子 agent，模型缺信息/需确认时自主调用）。
func AskUserTool() tool.InvokableTool {
	return mustTool("ask_user", "向用户提问以获取缺失信息或确认（用户只看到问题文本，你的回答 = 用户原文）。需要设计确认/方案确认/补充信息时调用",
		askUser)
}
