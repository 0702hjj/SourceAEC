// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj

// fs_backend.go：M2-0 打地基——官方 filesystem middleware 的收敛适配（D12）。
//
// 官方 filesystem middleware（adk/middlewares/filesystem）注入文件工具组
// （ls/read_file/write_file/edit_file/glob/grep）+ execute，但 Backend 非空即
// 全挂（无开关单独禁用 write/edit）。领域收敛红线（api_regulation：禁止任意
// 文件写）要求我们做一层薄包装：
//
//   - Backend = fsReadOnlyBackend（包住 local backend）：读方法透传，Write/Edit 拒绝
//   - StreamingShell = local backend（/bin/sh -c）+ ValidateCommand 白名单：
//     打地基最小集 = aiplan / aidxfv3 系列 CLI（skill 捆绑命令），其余拒绝
//
// skill CLI 产出（plan.json/DXF 等）由命令自身写盘（走 /bin/sh -c），
// 不经过 Backend.Write——所以只读包装不影响 skill 脚本执行闭环。
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
)

// skillCommandAllowlist 是 execute 命令白名单（默认 = dist 正式集合的 CLI 入口：
// aiplan / aidxfv3 / aiifc）。可经 SetSkillCommandAllowlist 配置化（第二层：对齐已接入
// skill 集合）；正式 skill 命令面（完整子命令枚举）后续按 machine_contract 细化。
var skillCommandAllowlist = []string{"aiplan", "aidxfv3", "aiifc"}

// SetSkillCommandAllowlist 覆盖 execute 命令白名单（装配时调用，来自 server 配置）。
// 空列表 = 全部拒绝（execute 不可用）。
func SetSkillCommandAllowlist(names []string) {
	if len(names) == 0 {
		skillCommandAllowlist = []string{}
		return
	}
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	skillCommandAllowlist = out
}

// validateSkillCommand 是 local backend 的 ValidateCommand 回调：
// 只放行白名单命令（按第一个 token 精确匹配），其余拒绝（领域收敛单点）。
func validateSkillCommand(cmd string) error {
	c := strings.TrimSpace(cmd)
	if c == "" {
		return fmt.Errorf("命令为空")
	}
	// local backend executes through /bin/sh -c. Checking only the first token
	// would therefore allow shell control operators after an allowed CLI name
	// (for example: "aiplan --help; curl ..."). Reject shell syntax entirely;
	// skill commands use ordinary executable arguments and do not need it.
	if strings.ContainsAny(c, ";|&><$`(){}\\\n\r") {
		return fmt.Errorf("命令包含被禁止的 shell 语法")
	}
	name := strings.Fields(c)[0]
	for _, allow := range skillCommandAllowlist {
		if name == allow {
			return nil
		}
	}
	return fmt.Errorf("命令不在白名单（打地基阶段仅允许 %s）：%s", strings.Join(skillCommandAllowlist, "/"), cmd)
}

// fsReadOnlyBackend 是 filesystem.Backend 的只读包装：
// 读方法（LsInfo/Read/GrepRaw/GlobInfo）透传 local backend；
// Write/Edit **白名单放开到 skill 工作区**（{DATA}/skill-work/ 下允许——agent 产
// design.json 等 LLM 意图中间产物），其它路径拒绝（领域收敛：不写 models/uploads/任意路径）。
type fsReadOnlyBackend struct {
	inner         filesystem.Backend
	skillWorkRoot string   // {DATA}/skill-work 绝对路径（前缀匹配；空 = 全拒绝）
	readRoots     []string // filesystem 读写允许的绝对路径根
}

func (b *fsReadOnlyBackend) confinedPath(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	clean := filepath.Clean(p)
	for _, root := range b.readRoots {
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		root = filepath.Clean(root)
		if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
			continue
		}
		resolved := clean
		probe := clean
		for {
			if real, err := filepath.EvalSymlinks(probe); err == nil {
				resolved = filepath.Join(real, strings.TrimPrefix(clean, probe))
				break
			}
			next := filepath.Dir(probe)
			if next == probe {
				break
			}
			probe = next
		}
		resolved, err := filepath.Abs(resolved)
		if err == nil && (resolved == root || strings.HasPrefix(resolved, root+string(os.PathSeparator))) {
			return true
		}
	}
	return false
}

func (b *fsReadOnlyBackend) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	if !b.confinedPath(req.Path) {
		return nil, fmt.Errorf("filesystem 路径不在允许的 skill/project 工作区")
	}
	return b.inner.LsInfo(ctx, req)
}

func (b *fsReadOnlyBackend) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	if !b.confinedPath(req.FilePath) {
		return nil, fmt.Errorf("filesystem 路径不在允许的 skill/project 工作区")
	}
	return b.inner.Read(ctx, req)
}

func (b *fsReadOnlyBackend) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	if !b.confinedPath(req.Path) {
		return nil, fmt.Errorf("filesystem 路径不在允许的 skill/project 工作区")
	}
	return b.inner.GrepRaw(ctx, req)
}

func (b *fsReadOnlyBackend) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	if !b.confinedPath(req.Path) {
		return nil, fmt.Errorf("filesystem 路径不在允许的 skill/project 工作区")
	}
	return b.inner.GlobInfo(ctx, req)
}

// withinSkillWork 判定写目标是否在 skill 工作区白名单根下（skill-work/{projectID}/...）。
func (b *fsReadOnlyBackend) withinSkillWork(p string) bool {
	if b.skillWorkRoot == "" {
		return false
	}
	// Keep the small unit-test fixture usable when it supplies only the legacy
	// skillWorkRoot field; production construction always supplies readRoots.
	if len(b.readRoots) == 0 {
		clean := filepath.Clean(p)
		return clean == b.skillWorkRoot || strings.HasPrefix(clean, b.skillWorkRoot+string(os.PathSeparator))
	}
	return b.confinedPath(p)
}

// Write 白名单放开：仅 skill 工作区（skill-work/）允许 agent 直接写（design.json 等
// LLM 意图中间产物）；其余路径拒绝（领域收敛红线不破——不写 models/uploads/任意路径）。
func (b *fsReadOnlyBackend) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	if b.withinSkillWork(req.FilePath) {
		return b.inner.Write(ctx, req)
	}
	return fmt.Errorf("领域收敛：filesystem 只允许写 skill 工作区（skill-work/{projectID}/，产 design.json 等中间产物）；目标 %q 不在白名单", req.FilePath)
}

// Edit 拒绝：同上（仅 skill 工作区可改写）。
func (b *fsReadOnlyBackend) Edit(ctx context.Context, req *filesystem.EditRequest) error {
	if b.withinSkillWork(req.FilePath) {
		return b.inner.Edit(ctx, req)
	}
	return fmt.Errorf("领域收敛：filesystem 只允许改 skill 工作区（skill-work/{projectID}/）；目标 %q 不在白名单", req.FilePath)
}

// skillWorkRootFor 计算 filesystem Write/Edit 白名单根（{DATA}/skill-work 绝对路径）。
// dataDir 为空 → 空根（全拒绝写）；相对路径转绝对（与 execute 注入的 VIEWER_DATA_DIR 一致）。
func skillWorkRootFor(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return ""
	}
	return filepath.Join(abs, "skill-work")
}
