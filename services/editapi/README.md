# editapi —— ifc/cad 共享 REST 编辑面领域层 + 契约套件

`services/ifc`（FastAPI + ifcopenshell, :8100）与 `services/cad`（FastAPI +
ezdxf, :8200）是同构镜像服务。本目录承载两块内容：

- **`src/aibim_editapi/`**：共享领域基座——`profile`（ServiceProfile
  服务差异显式声明）、`config`（Settings + load_settings(profile)）、
  `script_runner`（sandbox_config(profile) + ScriptRunner 适配层），以及
  `route_common`/`versions`/`script_versions` 三个领域门面（实现单点仍在
  `aibim_sandbox`；此处不复制第二份）。两侧 `app/` 同名模块改为
  薄 shim：声明各自 PROFILE 后 re-export，公开名与测试 seam 零改动。
- **`tests/`**：HTTP 级契约套件，把两侧**共享面**的现状行为钉死，
  作为收编迁移的回归门：**迁移复用本套件时不得改断言**。

契约套件不引入第三个 venv，寄生在目标服务的 venv 下运行。


## 为什么是「同一套代码 × 两个环境」

两个服务的 venv 依赖互斥（ifc=ifcopenshell、cad=ezdxf），单进程无法同时
import 两个 `app` 包。契约套件因此不自己建环境，而是**寄生在目标服务的
venv 下运行**：`tests/conftest.py` 解析目标（env `EDITAPI_TARGET=ifc|cad`，
缺省从 cwd 推导），把 `services/{ifc|cad}` 插进 `sys.path` 后 `create_app()`
——等价于在目标服务里跑一组外部测试文件。

## 怎么跑（×2）

```bash
# ifc 环境（ifcopenshell 侧）
cd services/ifc && uv run --group dev pytest ../editapi/tests -q

# cad 环境（ezdxf 侧）
cd services/cad && uv run --group dev pytest ../editapi/tests -q

# 不在服务目录下时显式指定目标
EDITAPI_TARGET=ifc uv run --group dev pytest ../editapi/tests -q   # 仍须在 services/ifc 下（用它的 venv）
```

两条命令各跑一遍、都要绿，才算契约面完整通过。沙箱后端约束与两侧套件相同：
有 bwrap 走 bwrap 真路径，无 bwrap 的环境 conftest 自动显式选 rlimit（仅测试）。

## 覆盖面（共享 REST 面，两侧同路径同语义）

- 暂存：`PUT/GET /v1/models/{id}/script`、`GET .../script/params`、
  `POST .../script/undo|redo|discard`、归档 seed、契约违规 422 / 404 / 409。
- run/save：`POST .../script/run`（成功 + 失败 422 零副作用）、
  `POST .../script/save`（`scripts/v{n}.py` 全留 + `versions/v{n}.{ifc|dxf}`
  只留最新 + meta/map sidecar lockstep）、`GET .../scripts`、`GET .../versions`、
  `POST .../script/rollback`。
- locate/edit-call：`GET .../script/locate`（命中/未命中/stale 降级）、
  `POST .../script/edit-call`（标量改写 + 404/409/422 零副作用）。
- diff：`POST .../script/diff`、`GET .../script/staging/diff`、
  `POST .../diff`（语义 diff 基本流 + 缺参 422 / 未知版本 404 / 结果缓存）。

## 差异进 profile，断言保持形状级

两侧已知差异全部收在 `tests/conftest.py` 的 `TargetProfile`：

| 差异 | ifc | cad |
|---|---|---|
| 产物扩展名 | `.ifc` | `.dxf` |
| locate 入参/响应键 | `?guid=` / `designKey` | `?key=` / `key` |
| edit-call body 键 | `designKey` | `key` |
| 可改写实参名 | `name` | `text` |
| data_dir 种子 | `converter/test/fixtures/wall-with-opening-and-window.ifc` | ezdxf 现造最小 DXF |
| 语义 diff 版本对 | 墙改名（ifcopenshell 手写快照） | 同 key LINE 终点变化 |

断言只做形状级一致（状态码、envelope 字段存在性与类型、版本目录布局），
不复制服务特有断言：ifc 的 semanticDiff 计数/alignment、cad 的 XDATA/
buildingChanges/render.json 留在两侧各自套件。已知响应字段差异（ifc save 多
`alignment`、cad script/diff 多 `buildingChanges`）不进共享断言。

## 与 sandbox 契约套件的关系

`services/sandbox/tests/` 是纯函数/适配器级契约；本套件是 HTTP 级
（TestClient 打真实路由），机制不同但目标一致：收编前的行为钉死。
