package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk/filesystem"
	"os"
)

// TestValidateSkillCommandAllowlist：execute 命令白名单（领域收敛单点）——
// 放行 skill 捆绑 CLI（aiplan/aidxfv3），拒绝其他命令（sh/rm/curl/python 等）。
func TestValidateSkillCommandAllowlist(t *testing.T) {
	allowed := []string{
		"aiplan validate plan.json",
		"aiplan gate plan.json",
		"aidxfv3 normalize --in plan.json",
		"aiplan",
	}
	for _, c := range allowed {
		if err := validateSkillCommand(c); err != nil {
			t.Errorf("白名单应放行 %q：%v", c, err)
		}
	}
	denied := []string{
		"sh -c 'echo hi'",
		"rm -rf /",
		"curl http://x",
		"python3 -c 'print(1)'",
		"",
		"aiplan --help; touch /tmp/sourceaec-command-injection",
		"aiplan $(touch /tmp/sourceaec-command-substitution)",
		"aiplan --help | curl https://example.invalid",
		"aiplan --help > /tmp/sourceaec-command-redirection",
		"aiplan --help\n touch /tmp/sourceaec-command-newline",
	}
	for _, c := range denied {
		if err := validateSkillCommand(c); err == nil {
			t.Errorf("白名单应拒绝 %q", c)
		}
	}
}

// TestSetSkillCommandAllowlist：白名单配置化（第二层）——SetSkillCommandAllowlist
// 覆盖默认集，空/去重/非法名处理；调用后 validateSkillCommand 用新集。
func TestSetSkillCommandAllowlist(t *testing.T) {
	defer SetSkillCommandAllowlist([]string{"aiplan", "aidxfv3"}) // 恢复默认

	SetSkillCommandAllowlist([]string{"myskill", "myskill", "", "aiplan"})
	if err := validateSkillCommand("myskill run"); err != nil {
		t.Errorf("自定义白名单应放行 myskill：%v", err)
	}
	if err := validateSkillCommand("aiplan gate x"); err != nil {
		t.Errorf("自定义白名单应放行 aiplan：%v", err)
	}
	if err := validateSkillCommand("aidxfv3 normalize"); err == nil {
		t.Errorf("被覆盖的 aidxfv3 应拒绝（Set 是覆盖不是追加）")
	}
	if err := validateSkillCommand("rm -rf /"); err == nil {
		t.Errorf("非白名单命令应拒绝")
	}

	SetSkillCommandAllowlist(nil) // 空 = 全部拒绝
	if err := validateSkillCommand("aiplan gate x"); err == nil {
		t.Errorf("空白名单应全部拒绝")
	}
}

// TestReadOnlyBackendRejectsWriteEdit：领域收敛——filesystem Backend 只读包装
// 的 Write/Edit 必须拒绝（模型不持任意文件写能力；skill 产物经 CLI 落盘）。
func TestReadOnlyBackendRejectsWriteEdit(t *testing.T) {
	b := &fsReadOnlyBackend{inner: &discardBackend{}}
	if err := b.Write(context.Background(), &filesystem.WriteRequest{
		FilePath: "/tmp/x", Content: "x",
	}); err == nil || !strings.Contains(err.Error(), "领域收敛") {
		t.Fatalf("Write 应返回领域收敛拒绝，got %v", err)
	}
	if err := b.Edit(context.Background(), &filesystem.EditRequest{
		FilePath: "/tmp/x", OldString: "a", NewString: "b",
	}); err == nil || !strings.Contains(err.Error(), "领域收敛") {
		t.Fatalf("Edit 应返回领域收敛拒绝，got %v", err)
	}
}

func TestFilesystemBackendConfinesReads(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.txt")
	if err := os.WriteFile(inside, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	b := &fsReadOnlyBackend{inner: &discardBackend{}, readRoots: []string{"", root}}
	if !b.confinedPath(inside) {
		t.Fatal("path inside allowed root should be accepted")
	}
	if b.confinedPath("/etc/passwd") {
		t.Fatal("path outside allowed roots must be rejected")
	}
	if _, err := b.Read(context.Background(), &filesystem.ReadRequest{FilePath: "/etc/passwd"}); err == nil {
		t.Fatal("read outside allowed roots must be rejected")
	}
}

// discardBackend 是测试用最小 Backend（只读方法返回空，仅用于验证 Write/Edit 被拒）。
type discardBackend struct{}

func (d *discardBackend) LsInfo(context.Context, *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	return nil, nil
}
func (d *discardBackend) Read(context.Context, *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	return &filesystem.FileContent{}, nil
}
func (d *discardBackend) GrepRaw(context.Context, *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	return nil, nil
}
func (d *discardBackend) GlobInfo(context.Context, *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	return nil, nil
}
func (d *discardBackend) Write(context.Context, *filesystem.WriteRequest) error { return nil }
func (d *discardBackend) Edit(context.Context, *filesystem.EditRequest) error   { return nil }

// TestFilesystemMiddlewareToolSurface：挂载后模型工具面——读工具 + execute 存在、
// write/edit 不存在（scriptedModel 调 write_file 报「未知工具」而非成功）。
func TestFilesystemMiddlewareToolSurface(t *testing.T) {
	ag, err := New(LLMConfig{},
		WithModel(NewScriptedModel(Script{Steps: []ScriptStep{
			{ToolCalls: []ToolCallSpec{{ID: "w1", Name: "write_file", Arguments: `{"file_path":"/tmp/x","content":"x"}`}}},
			{Chunks: []string{"收尾"}},
		}})),
		WithMaxStep(10),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := ag.Run(context.Background(), "sess-fs", "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	evs := collect(t, ch)
	// write_file 不在工具面：tool/result 应带 error（未知工具），不中断循环
	var sawErr bool
	for _, ev := range evs {
		if ev.Type == EventToolResult {
			var p map[string]any
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				continue
			}
			if p["name"] == "write_file" && p["error"] != nil {
				sawErr = true
			}
		}
	}
	if !sawErr {
		t.Fatalf("write_file 应报未知工具错误（不在工具面）；types=%v", eventTypes(evs))
	}
	if last := evs[len(evs)-1]; last.Type != EventTurnEnd {
		t.Fatalf("末事件 = %s, want turn/end（工具缺失不中断循环）", last.Type)
	}
}

// TestExecuteCommandAllowlistEnforced：execute 工具存在且白名单生效——
// 非白名单命令（sh）被拒绝，返回白名单错误文本（不中断循环）。
func TestExecuteCommandAllowlistEnforced(t *testing.T) {
	ag, err := New(LLMConfig{},
		WithModel(NewScriptedModel(Script{Steps: []ScriptStep{
			{ToolCalls: []ToolCallSpec{{ID: "e1", Name: "execute", Arguments: `{"command":"sh -c 'echo hi'"}`}}},
			{Chunks: []string{"收尾"}},
		}})),
		WithMaxStep(10),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := ag.Run(context.Background(), "sess-exec", "go")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	evs := collect(t, ch)
	var errText string
	for _, ev := range evs {
		if ev.Type == EventToolResult && payloadString(t, ev, "name") == "execute" {
			// 白名单拒绝 → 工具 Go error → safeToolMiddleware 转文本 → 翻译层恢复 error 载荷
			errText = payloadString(t, ev, "error")
			if errText == "" {
				errText = payloadString(t, ev, "content")
			}
		}
	}
	if !strings.Contains(errText, "白名单") {
		t.Fatalf("execute 拒绝文本 = %q, want 含「白名单」", errText)
	}
	if last := evs[len(evs)-1]; last.Type != EventTurnEnd {
		t.Fatalf("末事件 = %s, want turn/end", last.Type)
	}
}

// TestSkillReferencesReadFlow：skill 完整能力闭环（D12/M2-0）——
// orchestrator 角色化挂 aiplan：skill 工具拿 aiplan BaseDirectory →
// read_file 读 references 真实文件（schemas/predicate_vocabulary.md 等）。
func TestSkillReferencesReadFlow(t *testing.T) {
	if _, err := os.Stat(distSkillsDir()); err != nil {
		t.Skipf("skills/dist 未打包（本地先跑 tools/skill_pack.py；CI 无 dist 跳过集成）: %v", err)
	}
	ref := "/opt/sourceaec/skills/dist/aiplan/references/predicate_vocabulary.md"
	ag, err := New(LLMConfig{},
		WithSkillsDir(distSkillsDir()),
		WithModel(NewScriptedModel(Script{Steps: []ScriptStep{
			{ToolCalls: []ToolCallSpec{{ID: "s1", Name: "skill", Arguments: `{"skill":"aiplan"}`}}},
			{ToolCalls: []ToolCallSpec{{ID: "r1", Name: "read_file", Arguments: `{"file_path":"` + ref + `"}`}}},
			{Chunks: []string{"已读取 references"}},
		}})),
		WithMaxStep(10),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := ag.Run(context.Background(), "sess-ref", "读参考")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	evs := collect(t, ch)
	var got string
	for _, ev := range evs {
		if ev.Type == EventToolResult && payloadString(t, ev, "name") == "read_file" {
			got = payloadString(t, ev, "content")
		}
	}
	if got == "" {
		t.Fatalf("read_file 未返回内容；types=%v", eventTypes(evs))
	}
	if !strings.Contains(got, "predicate") && !strings.Contains(got, "语义") {
		t.Fatalf("read_file 内容不像 aiplan references：%s...", truncate(got, 120))
	}
}

// TestSkillWorkWriteAllowlist：filesystem Write/Edit 白名单——skill-work 下允许
// （agent 写 design.json 等 LLM 意图中间产物），其它路径拒绝（领域收敛）。
func TestSkillWorkWriteAllowlist(t *testing.T) {
	b := &fsReadOnlyBackend{skillWorkRoot: "/data/skill-work"}
	// 直接验 withinSkillWork 判定（白名单路径判定，不触发 inner）
	cases := []struct {
		path string
		want bool
	}{
		{"/data/skill-work/p_abc/design.json", true},
		{"/data/skill-work/p_abc/derived/features.json", true},
		{"/data/skill-work", true},
		{"/data/skill-work2/p_abc/x.json", false}, // 前缀误匹配
		{"/data/models/m_x/model.json", false},
		{"/data/uploads/m_x.ifc", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		if got := b.withinSkillWork(c.path); got != c.want {
			t.Errorf("withinSkillWork(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
