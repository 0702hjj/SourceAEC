# 环境要求与本地启动

开发形态是**宿主机直跑，不用 Docker**：web 由 Vite 起开发服务器，Go server 与两个 Python 服务各起一个进程。生产部署（构建产物、systemd、反向代理、备份）见[生产部署与运维](/guide/deploy)。

## 环境依赖

| 依赖 | 版本 | 用途 | 必需性 |
| --- | --- | --- | --- |
| Go | 1.26+ | server | 必需 |
| Node.js | 22+ | converter 与 web 构建 | 必需 |
| Python + [uv](https://docs.astral.sh/uv/) | 3.10+ | 两个编辑服务 | 编辑和 diff 功能必需，纯浏览可不装 |
| Linux + bubblewrap | — | 脚本沙箱 | 生产必需。缺失时 run/save 直接拒绝执行 |
| ripgrep (`rg`) | — | chat agent 的搜索工具后端 | chat agent 必需 |
| PostgreSQL | 14+ | Issue 等数据的持久化 | 可选，默认用文件存储 |

几点说明：

- Python 依赖全部来自 PyPI 官方发布，`uv sync` 直接安装，不需要本机源码。
- bubblewrap 在 Debian/Ubuntu 上装 `bubblewrap` 包，RHEL 系用 `dnf install bubblewrap`。沙箱只有 bwrap 一条生产路径，没有它就拒绝执行，不会降级。
- ripgrep 缺失时，agent 的 grep 工具会报错，只能退回逐文件读取，所以建议装上。

## 启动

开四个终端，都从**仓库根目录**起。每段命令都假设自己在仓库根，不要顺着上一段往下走。DXF 功能不需要时，终端 1b 里的 cad 服务可以不启。

```bash
# 0. 一次性：安装依赖（子 shell 保证每行都从仓库根解析）
(cd converter && npm install)
(cd web && npm install)
(cd services/ifc && uv sync)
(cd services/cad && uv sync)

# 1. edit-service（:8100）—— VIEWER_DATA_DIR 必须指向 data 的绝对路径
cd services/ifc
VIEWER_DATA_DIR="$(cd ../data && pwd)" uv run uvicorn app.main:app --port 8100

# 1b. cad-edit-service（:8200）—— DXF 模型需要；纯 IFC 可不启
cd services/cad
VIEWER_DATA_DIR="$(cd ../data && pwd)" uv run uvicorn app.main:app --port 8200

# 2. Go server（:8090）
cd server && go run ./cmd/server

# 3. web（:5173）
cd web && npm run dev
```

打开 `http://localhost:5173` 即可使用。**两个 Python 服务的 `VIEWER_DATA_DIR` 必须与 server 的 `dataDir` 指向同一个目录**，配错会 404 或改错文件——这是最常见的启动陷阱。

LLM 三参不配也能跑通界面流程，只是 AI 对话会退到离线模式。完整配置项见[配置说明](/guide/configuration)，沙箱机制见[沙箱执行环境](/development/sandbox)。

## 验证

```bash
# 端到端冒烟（需 server 运行；edit-service 不可达时编辑段落自动跳过）
./scripts/smoke.sh

# 各层测试
cd server && go test ./...
cd services/ifc && uv run --group dev pytest
cd services/cad && uv run --group dev pytest
cd web && npm test
cd converter && npm test
```

浏览、上传、转换不依赖编辑服务和 PostgreSQL；编辑、版本、diff 需要编辑服务在运行。各模块的完整测试矩阵与手工验收清单见[测试与调试](/development/testing)。
