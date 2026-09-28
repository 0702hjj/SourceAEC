// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// chat_tools_plan.go：chat 工具桥接的方案/工作区域（从 chat_tools.go 拆出，
// W-0057 行数门控）——plan/building 产物读写（PlanStore 版本化）与 skill 工作区
// 落盘桥接（plan→workdir、cad→ifc 上游→workdir）。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// planGetForAgent 读方案产物当前态（经 PlanStore；未配置/缺失 → 文本错误）。
func (h *ChatHandler) planGetForAgent(ctx context.Context, projectID, name string) (string, error) {
	if h.deps.PlanSt == nil {
		return "", fmt.Errorf("方案存储未配置")
	}
	content, err := h.deps.PlanSt.Get(projectID, name)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// skillWorkDirForAgent 返回项目 skill 工作区绝对路径（{DATA}/skill-work/{projectID}，
// 首次调用 MkdirAll）——aidxf 中间产物（derived/missions/deliver）落盘根，projectId
// 隔离多项目不混淆（复用 plans/{projectID} 的 projectId 隔离地基）。
func (h *ChatHandler) skillWorkDirForAgent(ctx context.Context, projectID string) (string, error) {
	if h.deps.DataDir == "" {
		return "", fmt.Errorf("数据目录未配置")
	}
	dir := filepath.Join(h.deps.DataDir, "skill-work", projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("建 skill 工作区: %w", err)
	}
	return dir, nil
}

// planToWorkdirForAgent 把项目 plan 产物（plan.json + bim_supplement.json）从 PlanStore
// 落到 skill 工作区文件，返回 {planPath, bimPath}——aidxfv3 preprocess --plan <文件> 等
// 命令需要文件路径时的桥接（plan 内容 → 工作区文件）。
func (h *ChatHandler) planToWorkdirForAgent(ctx context.Context, projectID string) (map[string]string, error) {
	if h.deps.PlanSt == nil {
		return nil, fmt.Errorf("方案存储未配置")
	}
	dir, err := h.skillWorkDirForAgent(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, name := range []string{"plan.json", "bim_supplement.json"} {
		content, err := h.deps.PlanSt.Get(projectID, name)
		if err != nil {
			return nil, fmt.Errorf("读方案产物 %s: %w", name, err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return nil, fmt.Errorf("写工作区 %s: %w", name, err)
		}
		if name == "plan.json" {
			out["planPath"] = path
		} else {
			out["bimPath"] = path
		}
	}
	return out, nil
}

// planDeliverForAgent 触发 plan 交付（复用 deliverPlan 的 aiplan land 执行逻辑；
// 抽 deliverPlanCore 供 REST handler 与工具共用——单一事实源）。
func (h *ChatHandler) planDeliverForAgent(ctx context.Context, projectID, plan, bimSupplement string) (map[string]any, error) {
	if h.deps.AiplanBin == "" {
		return nil, fmt.Errorf("aiplan 未配置（skill venv 缺失），plan 交付不可用")
	}
	return h.deliverPlanCore(ctx, projectID, []byte(plan), []byte(bimSupplement))
}

// upstreamToWorkdirForAgent 把 ifc 消费的上游产物落到 skill 工作区（cad->ifc 消费上游桥接）：
// building.json + bim_supplement.json（PlanStore → skill-work/{projectID}/）+
// 各 zone DXF（building.json zones[].modelId → uploads/{modelId}.dxf 当前态 →
// skill-work/{projectID}/dxf/<zone>.dxf）。返回 {buildingPath, bimPath, dxfDir, dxfPaths}——
// ifc-agent 跑 aiifc consume-upstream --building/--bim/--dxf-dir 的输入桥接。
func (h *ChatHandler) upstreamToWorkdirForAgent(ctx context.Context, projectID string) (map[string]any, error) {
	if h.deps.PlanSt == nil {
		return nil, fmt.Errorf("方案存储未配置")
	}
	if h.deps.DataDir == "" {
		return nil, fmt.Errorf("数据目录未配置")
	}
	dir, err := h.skillWorkDirForAgent(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"projectId": projectID}
	for _, name := range []string{"building.json", "bim_supplement.json"} {
		content, err := h.deps.PlanSt.Get(projectID, name)
		if err != nil {
			return nil, fmt.Errorf("读上游产物 %s: %w（需先 deliver_building/deliver_plan）", name, err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return nil, fmt.Errorf("写工作区 %s: %w", name, err)
		}
		if name == "building.json" {
			out["buildingPath"] = path
		} else {
			out["bimPath"] = path
		}
	}
	dxfDir := filepath.Join(dir, "dxf")
	if err := os.MkdirAll(dxfDir, 0o755); err != nil {
		return nil, fmt.Errorf("建 dxf 目录: %w", err)
	}
	zones, err := h.buildingZones(projectID)
	if err != nil {
		return nil, err
	}
	dxfPaths := map[string]string{}
	for _, z := range zones {
		zone, _ := z["zone"].(string)
		modelID, _ := z["modelId"].(string)
		if zone == "" || modelID == "" {
			continue
		}
		src := filepath.Join(h.deps.DataDir, "uploads", modelID+".dxf")
		if _, err := os.Stat(src); err != nil {
			return nil, fmt.Errorf("zone %s 的 DXF 缺失（%s——需先 init_model 注册并 run 产 DXF）: %w", zone, src, err)
		}
		dst := filepath.Join(dxfDir, zone+".dxf")
		if err := copyFile(src, dst); err != nil {
			return nil, fmt.Errorf("复制 zone %s DXF: %w", zone, err)
		}
		dxfPaths[zone] = dst
	}
	out["dxfDir"] = dxfDir
	out["dxfPaths"] = dxfPaths
	return out, nil
}

// buildingZones 读 building.json 的 zones[]（zone + modelId 列表）。
func (h *ChatHandler) buildingZones(projectID string) ([]map[string]any, error) {
	content, err := h.deps.PlanSt.Get(projectID, "building.json")
	if err != nil {
		return nil, fmt.Errorf("读 building.json: %w", err)
	}
	var b struct {
		Zones []map[string]any `json:"zones"`
	}
	if err := json.Unmarshal(content, &b); err != nil {
		return nil, fmt.Errorf("building.json 解析: %w", err)
	}
	return b.Zones, nil
}

// buildingDeliverForAgent 交付 building.json（aidxf S4-c：agent 组装 plan 形态整栋楼 +
// zones 记 modelId）→ PlanStore 版本化 plans/{projectID}/building.json。
// 与 deliver_plan 同构但独立（building 不走 aiplan land——agent 组装直接 Put）。
func (h *ChatHandler) buildingDeliverForAgent(ctx context.Context, projectID, building string) (map[string]any, error) {
	if h.deps.PlanSt == nil {
		return nil, fmt.Errorf("方案存储未配置")
	}
	ver, err := h.deps.PlanSt.Put(projectID, "building.json", []byte(building))
	if err != nil {
		return nil, fmt.Errorf("building.json 版本化: %w", err)
	}
	return map[string]any{"projectId": projectID, "buildingVersion": ver}, nil
}
