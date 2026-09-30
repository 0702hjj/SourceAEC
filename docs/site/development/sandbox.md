# 沙箱执行环境（aibim_sandbox）

两个 Python 编辑服务的 `script/run` 和 `script/save` 会在服务端执行用户或 AI 写的构建脚本。脚本执行就是代码执行，所以全部跑在沙箱里。

沙箱实现在共享包 `services/sandbox`（包名 `aibim_sandbox`），两个服务以 uv path editable 方式引用。它同时是 script-as-source 七个领域模块的唯一事实源：暂存、版本、diff、参数、编辑、定位、校验，两个服务共享同一份实现，各自只留薄适配。

## 后端：生产只有 bwrap 一条路

| 后端 | 隔离能力 | 适用场景 |
| --- | --- | --- |
| `bwrap`（bubblewrap） | 按需只读挂载，沙箱外写入直接 EROFS；断网 | 生产唯一路径 |
| `rlimit` | rlimits 加独立工作目录，网络和外部文件系统不拦截 | 仅测试，须显式选择 |

- 没有 bwrap 时 run/save 拒绝执行，返回 503，不降级。
- 后端用 `SANDBOX_BACKEND` 选择，取值 `auto`、`bwrap`、`rlimit`，非法值启动即失败。`rlimit` 生产环境不要设。旧的 `ALLOW_RLIMIT_FALLBACK` 已删除，设了也不生效。
- 启动日志会打印后端探测结果，部署时确认看到 bwrap 可用。
- 安装：Debian/Ubuntu 用 `sudo apt install bubblewrap`，RHEL 系用 `sudo dnf install bubblewrap`，并确认 `user.max_user_namespaces` 大于 0。

## 资源与并发限额

| env | 默认 | 说明 |
| --- | --- | --- |
| `SCRIPT_RUN_CONCURRENCY` | `3` | run/save 并发上限，满时返回 429 |
| `SCRIPT_MAX_FSIZE_BYTES` | `256 MiB` | 单文件写入上限 |
| `SCRIPT_MAX_OUTPUT_BYTES` | `1 MiB` | stdout 加 stderr 累计上限 |
| `SCRIPT_MAX_PRODUCT_BYTES` | `256 MiB` | 产物与 map 发布上限 |

两个服务同名同义。执行超时 60 秒，超时连孙进程一起终止。

## 依赖环境：PEP 723 加 uv 缓存

脚本可以在头部声明依赖：

```python
# /// script
# dependencies = ["ifcopenshell>=0.8", "numpy"]
# ///
```

- 声明即全量。写了 dependencies 就替换默认集；不写就注入服务默认集，ifc 侧是 `ifcopenshell>=0.8` 加 `numpy`，cad 侧是 `ezdxf>=1.3`。
- 依赖由宿主机进程解析：用 uv 建 venv、只装 wheel，按依赖集内容寻址缓存。沙箱内断网执行。
- run/save 依赖 `uv` 二进制。缺 uv 返回 503，属于部署问题；依赖解析失败返回 422，属于脚本声明问题，报错带 uv 输出截尾。
- 缓存根 `SANDBOX_ENV_CACHE_DIR` 默认在 `$XDG_CACHE_HOME/aibim-sandbox-envs`。不要配到 data/ 或 /tmp 下面，前者 bwrap 不会挂载，后者是临时卷语义。
- 依赖构建超时用 `SCRIPT_ENV_BUILD_TIMEOUT_S` 控制，默认 300 秒。

## 失败语义速查

| 状态码 | 含义 |
| --- | --- |
| 422 | 契约校验失败、沙箱构建失败或依赖解析失败。请求零副作用，按报错修正后重发 |
| 429 | 并发闸满，稍后重试 |
| 503 | 沙箱不可用，缺 bwrap 或缺 uv，属于部署问题 |

编辑层还有一个 409（暂存脚本与 ScriptMap 分叉），不属于沙箱语义，定义在 [IFC 编辑 API](/reference/edit-api#定位链路)。现象级速查见[故障排查](/guide/troubleshooting#状态码去哪查)。
