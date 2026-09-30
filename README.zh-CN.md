# SourceAEC

[English](README.md)

SourceAEC 是面向 AEC 的开源、自托管 AI 建模平台，提供 IFC 与 CAD/DXF 的
script-as-source 编辑、语义版本对比，以及设计师和 AI agent 共用的编辑 API。

> 文档：<https://0702hjj.github.io/SourceAEC/>

## 核心能力

- 基于 IfcOpenShell 的 IFC 创建与编辑。
- 基于 ezdxf 的 CAD/DXF 编辑和浏览器画布查看器。
- Python 构建脚本是模型唯一事实源。
- 沙箱试运行与不可变版本快照。
- 按 IFC GlobalId 的属性级语义对比。
- React 前端、Go 网关和两个对等的 Python 编辑服务。
- 进程内 AI agent；未配置 API key 时使用确定性离线模型。

## 架构

```text
浏览器（React、xeokit/web-ifc、Fabric）
        |
        v
Go 网关 :8090 ----> IFC 服务 :8100 ----> IfcOpenShell
        |            CAD 服务 :8200 ----> ezdxf
        |                     |
        +----> converter      +----> 共享沙箱与编辑 API 包
        +----> 进程内 AI agent
```

## 界面

| 模型库 | IFC 属性 |
|---|---|
| ![模型库](assets/screenshots/library.png) | ![IFC 属性](assets/screenshots/properties.png) |

| 版本对比 | AI 对话 |
|---|---|
| ![版本对比](assets/screenshots/diff.png) | ![AI 对话](assets/screenshots/chat.png) |

## 快速开始

需要 Go 1.26、Node.js 22、Python 3.10、`uv`，以及 Linux 上的 bubblewrap。
生产环境缺少 bubblewrap 时，脚本执行会 fail-closed。

```bash
cd converter && npm install
cd ../web && npm install
cd ../services/ifc && uv sync
cd ../cad && uv sync
```

分别启动两个 Python 服务、Go 网关和 Vite 开发服务器：

```bash
# 终端 1
cd services/ifc
VIEWER_DATA_DIR="$(realpath -m ../../data)" uv run uvicorn app.main:app --port 8100

# 终端 2
cd services/cad
VIEWER_DATA_DIR="$(realpath -m ../../data)" uv run uvicorn app.main:app --port 8200

# 终端 3
cd server && go run ./cmd/server

# 终端 4
cd web && npm run dev
```

两个 Python 服务和 Go 网关必须使用同一个绝对数据目录。`VIEWER_LLM_API_KEY`
为空时，agent 使用确定性离线模型。

## 仓库结构

```text
web/               React 前端
server/            Go REST 网关与进程内 agent
converter/         IFC 到 XKT 转换器
services/ifc/      IFC 编辑服务
services/cad/      CAD 编辑服务
services/sandbox/  共享沙箱运行时
services/editapi/  共享脚本编辑 API
mcp/               可选 MCP 桥
skills/             AI 建模与规划工具
tools/              打包与 agent 调试工具
examples/           合成示例
```

## 开发验证

修改哪个组件，就运行对应套件。主要命令：

```bash
cd web && npm test && npm run lint && npm run build
cd server && go test ./... && go vet ./...
cd converter && npm test
cd services/ifc && uv run --group dev pytest
cd services/cad && uv run --group dev pytest
cd services/sandbox && uv run --group dev pytest
cd mcp && uv run --group dev pytest
scripts/check_file_size.sh
scripts/check_public_snapshot.sh
```

提交 PR 前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

文档随源码维护：

```bash
cd docs
npm install
npm run docs:dev
npm run docs:build
```

## 许可证

SourceAEC 主体采用 Apache-2.0；嵌套的 MIT skill 与第三方运行时见
[NOTICE](NOTICE)。前端依赖 AGPL-3.0 的 xeokit，仓库内的 web-ifc WASM 文件
仍受 MPL-2.0 约束。
