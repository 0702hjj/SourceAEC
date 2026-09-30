# agent —— SourceAEC Agent 终端调试工具（textual TUI）

本地 GUI（TUI 形态）调试工具：不起前端、不 curl，一条命令对话式调试
chat agent——scriptedModel / 真实 LLM、SSE 帧序列、工具调用产物轨迹、ask_user 中断恢复。

## 三种用法

```bash
./agent                # 交互 TUI（默认；真实 LLM 按本机配置，未配 key 自动 scriptedModel）
./agent --scripted     # scriptedModel 离线模式（本工具拉起的 server 清空 llmAPIKey）
./agent --replay <frames.jsonl>   # 离线回放已保存帧流（不连 server，对照回归）
./agent --demo         # 轻量终端对话（无依赖 ANSI 版，agent_demo.py）
./agent --test         # 跑本工具单测（pytest，同一 venv）
```

首跑自动建 venv（`~/.cache/aiifc-agent/venv`）并装 textual + pytest；
TUI 自动拉起三服务（edit :8100 / cad :8200 / server :8090，`VIEWER_*` 端口 env 可覆盖）。

## scriptedModel / 真实 LLM 开关

server 侧机制（`server/internal/agent/model.go`）：`llmAPIKey` 为空 → `NewChatModel`
返回 nil → 回退 `defaultScriptedModel`（固定一句离线答复的确定性 mock）。配置优先级
env `VIEWER_LLM_API_KEY` > `server/server_config.json` 的 `llmAPIKey`（cmd/server/main.go
loadConfig：空 env **不**覆盖 json 值）——所以「强制 scripted」不能靠设空 env。

`--scripted` 的实现：读 server 配置基座（`server_config.json`，缺则 example），
仅清空 `llmAPIKey` 写到 `~/.cache/aiifc-agent/scripted_server_config.json`，
拉起 server 时带 `-config <该文件>`。生效范围：**只影响由本工具拉起的 server**——
:8090 已在跑则不重启，TUI 会黄字提示该进程的模式。状态栏 / 侧栏徽标显示当前模式。

## SSE 原始帧面板与落盘

侧栏（`f5` 帧面板 / `f6` 产物轨迹 / `f7` 显隐）：

- 每帧一行：`#序号 时间 event-kind [sa_turn_seq 子标签] data预览`，按 kind 着色
  （session.* 灰、part 帧青、subagent 洋红、question.ask 黄、error 红）；
- data 预览超 96 字符截断，行尾 `...(truncated)` 标记；
- 帧流同时落盘 JSONL（一行一帧，首行 header）：默认
  `~/.cache/aiifc-agent/frames/<cid>-<时间戳>.jsonl`（env `AIIFC_AGENT_FRAMES_DIR`
  可改）。位置在 git 外、也不进 `data/`（AGENTS 硬规则）。

帧 kind 全集见 `server/internal/api/chat_translate.go`（agent Event → 浏览器 SSE 帧）：
`session.status` / `message.updated` / `message.part.updated` / `message.part.delta` /
`subagent.status` / `question.ask` / `session.error` / `session.idle`。

## 确定性回放

`agent --replay frames.jsonl`：读回帧流按原序重放进帧面板 + 消息视图 + 轨迹视图，
不连 server、不二次落盘。同帧流两跑渲染必然一致（解析/渲染/轨迹提取全是纯函数），
用于对照回归：改了渲染逻辑后拿旧帧流验证视图不漂。scriptedModel 侧的确定性
（同脚本两跑事件序列一致）由 server 契约测试保证（`server/internal/agent/
scripted_test.go`）；本工具回放的是 SSE 帧层，不依赖 server 脚本可注入。
`question.ask` 在回放中不弹交互框（文本化呈现）。

JSONL 行格式：`{"type":"frame","seq":1,"ts":"...","event":"...","sid":3,"cid":"...","data":{...}}`；
回放加载兼容最小裸形 `{"event":"...","data":{...}}`（手工构造用），坏行跳过并计数提示。

## 产物轨迹视图

`f6` 切换：从帧流提取「哪个 agent（主 / 子 persona+sa_id）→ 调了什么 tool → 产物路径」
时间线，含子 agent started/finished 边界。产物识别（input/output 正则扫描）：
modelId（`m_[0-9a-f]{16}`）、带扩展名路径（.json/.ifc/.dxf/.py/.md/.png/.txt，
含 plans/、skill-work/ 产物链）。

## 与 server 契约测试的关系

server 测试守事件翻译/事件日志的契约（Go 测试）；本工具是消费侧观测面——
帧落盘 JSONL 即「人肉可读的契约样本」，回放用于在不动 server 的前提下验证
消费端渲染对既有帧形状的兼容。两层互补，不重复覆盖。

## 测试

```bash
./agent --test          # 或：~/.cache/aiifc-agent/venv/bin/python -m pytest tools/agent -q
```

- 纯函数单测：SSE 解析、帧行渲染、JSONL 往返、轨迹提取、scripted 配置派生（pytest）；
- textual Pilot 冒烟：`App.run_test()`（headless）驱动回放路径，断言帧面板/消息区渲染。

## 文件

| 文件 | 职责 |
|---|---|
| `agent` | bash 启动器（venv 自管理、参数透传） |
| `agent_tui.py` | TUI 主体（对话流、HITL、侧栏接线、回放模式） |
| `setup_screen.py` | 项目选择/新建/删除屏 |
| `runtime.py` | 服务拉起 / HTTP / SSE 流 / LLM 模式检测与 scripted 派生配置 |
| `frames_store.py` | 帧记录模型 + JSONL 落盘/读回 + 单行渲染（纯函数） |
| `frames_pane.py` | 侧栏组件（帧面板 / 轨迹视图切换） |
| `trace_view.py` | 产物轨迹提取 + 渲染（纯函数） |
| `replay.py` | 回放加载与逐帧喂给（纯函数） |
| `agent_demo.py` | ANSI 轻量对话（--demo） |
