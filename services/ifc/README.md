# ifc-edit-service

IFC 业务逻辑核心（FastAPI + ifcopenshell）：**script-as-source 编辑 API**（`PUT /script` 暂存 → `script/run` 沙箱试运行 → `script/save` 大版本）+ 版本快照与语义 diff。可脱离 Go server / web / converter / PostgreSQL 独立部署与调用，详见文档站 [Edit Service 独立部署与移植](https://0702hjj.github.io/SourceAEC/development/edit-service.html)。

## 运行

```bash
uv sync
VIEWER_DATA_DIR="$(cd ../data && pwd)" uv run uvicorn app.main:app --port 8100
```

配置（环境变量）：`EDIT_SERVICE_PORT`（默认 8100）、`VIEWER_DATA_DIR`（默认 `../data`，建议绝对路径）、`AIIFC_FLOWS_DIR`（默认 `../../skills/aiifc/references/docs/flows`，沙箱脚本契约校验依赖 aiifc skill flows）、`EDIT_SERVICE_MAX_MODELS`（默认 8）。

沙箱（W-0048，环境变量）：`SCRIPT_MAX_FSIZE_BYTES`（RLIMIT_FSIZE 单文件写上限，默认 256 MiB）、`SCRIPT_MAX_OUTPUT_BYTES`（脚本 stdout+stderr 累计上限，超出杀进程组 422，默认 1 MiB）、`SCRIPT_MAX_PRODUCT_BYTES`（产物与 map sidecar 发布上限，超限 422 不落盘，默认 256 MiB）、`SCRIPT_RUN_CONCURRENCY`（进程级 run/save 并发闸，满即 429，默认 3）、`SANDBOX_BACKEND`（沙箱后端：`auto` 默认——bwrap 优先、缺失则 run/save fail-closed 503；`bwrap` 显式——不可用即 503；`rlimit` 不隔离网络与沙箱外 FS，**仅测试可设，生产勿设**）、`SANDBOX_ENV_CACHE_DIR`（T4 依赖环境缓存根，默认 `$XDG_CACHE_HOME/aibim-sandbox-envs`，勿配 data/ 下）、`SCRIPT_ENV_BUILD_TIMEOUT_S`（uv 解析+安装超时，默认 300）。

脚本依赖（W-0048 T4）：构建脚本头部可写 PEP 723 `# /// script` 块声明 `dependencies`（**声明即全量**，替换默认集）；无声明注入默认集 `ifcopenshell>=0.8` + `numpy`（存量脚本基线）。依赖由宿主机的 uv 解析进内容寻址缓存环境（`--only-binary :all:`，不执行 sdist 构建代码），沙箱内只读挂载执行——**run/save 依赖 uv 二进制**（缺失 503；依赖不存在 422；断网 503）。

## 编辑 API

模型 id 必须匹配 `^m_[0-9a-f]{16}$`，对应 IFC 路径 `{VIEWER_DATA_DIR}/uploads/{id}.ifc`。

| 端点 | 语义 |
| --- | --- |
| `GET/PUT /models/{id}/script` | 读当前脚本 / 暂存一次编辑（整体替换或仅改 PARAMS） |
| `GET /models/{id}/script/params` · `POST .../undo|redo|discard` | PARAMS 提取 / 暂存导航与放弃 |
| `POST /models/{id}/script/run` | 沙箱试运行（预览，无版本） |
| `POST /models/{id}/script/save` | 晋升大版本（`scripts/v{n}.py` + `v{n}.map.json` 全留，`versions/v{n}.ifc` 只留最新） |
| `GET /models/{id}/scripts` · `POST .../script/rollback` · `.../diff` · `GET .../staging/diff` | 大版本列表 / 回退 / 脚本 diff / 暂存步 diff |
| `GET /models/{id}/script/locate?guid=` · `POST .../script/edit-call` | guid→调用点定位 / libcst 标量改写（edit-call 仅直连） |
| `GET /models/{id}/versions` · `POST /models/{id}/diff` · `POST .../diff/upload` | 版本列表 / 版本间语义 diff / 上传对比 |
| `GET/DELETE /models/{id}/pending` · `GET /models/{id}/history` | 只读保留（pending 为 script-run 回放簿记；history 只增） |
| `POST /models/{id}/user-edits` | 登记外部用户修改（`source="USER"`） |
| `PUT/DELETE /models/{id}/entities/{guid}` · `GET .../editable-schema` · `POST /models/{id}/commit` | **退役，410 Gone**（直改 IFC 已废弃，一切修改走构建脚本；回捞锚点 `fb55a8a`） |

完整契约（body、错误码、envelope 语义、Go 代理映射）见文档站 [IFC 编辑 API](https://0702hjj.github.io/SourceAEC/reference/edit-api.html)；独立部署与移植指南见 [Edit Service](https://0702hjj.github.io/SourceAEC/development/edit-service.html)；机器可消费 OpenAPI schema 见 `docs/site/public/ai-tools.openapi.json`。

## 测试

```bash
uv run --group dev pytest
```
