# SourceAEC

[English](README.md)

SourceAEC 是开源、自托管、面向 Agent 的 IFC/CAD 操作接口。业务代码全部使用
Go：编辑 API、暂存、校验、沙箱编排、持久化、Git、版本、方案交付和 MCP。

Python 仅作为外部几何运行时。模型源码由用户或 Agent 生成，在沙箱中执行；
仓库不维护 Python 应用、SDK、CLI、hooks 或测试。skill 只保留必要的声明和参考，
不捆绑上游 SDK。IfcOpenShell、ezdxf、SimpleCADAPI 等属于外部运行能力。

## 启动

需要 Go 1.27.3+、PostgreSQL、Git、Linux bubblewrap 和 `prlimit`。
前端与转换器需要 Node.js 22。通过 uv 准备 Python 3.12 几何环境，或用
`SOURCEAEC_PYTHON` 指定已安装所需依赖的解释器绝对路径。

```bash
export VIEWER_PG_DSN='postgres://localhost/sourceaec'
cd converter && npm ci
cd ../web && npm ci && npm run build
cd ../server && go run ./cmd/server
```

可复制 `server/server_config.example.json` 为 `server/server_config.json` 配置路径。
网关在进程内处理业务，不需要启动 Python HTTP 服务。对外部署需配置
`VIEWER_API_TOKEN` 和 `VIEWER_CORS_ORIGINS`；缺少 bubblewrap 时拒绝执行脚本。

编辑仍走 stage → run → save。读取当前脚本后携带 `baseRevision` 暂存修改，
试运行成功并检查真实产物后再保存。方案交付只校验并版本化声明性数据，
不包含旧规划 CLI 的自动设计或合规判断。

## 验证

```bash
cd server && go test ./... && go vet ./...
cd web && npm test && npm run lint && npm run build
cd converter && npm test
cd docs && npm run docs:build
```

Go 集成测试使用独立 PostgreSQL 数据库，配置 `VIEWER_PG_DSN` 和
`VIEWER_TEST_PG_DSN`。根目录执行 `scripts/check_file_size.sh`、
`scripts/check_public_snapshot.sh` 和 `scripts/check_go_business.sh`。

发布包用 `scripts/build_server_release.sh VERSION` 构建，包含 Go 网关、业务运行时和
Go MCP 桥。前端、转换器和几何依赖单独安装。MCP 通过 `SOURCEAEC_API_URL` 与
`VIEWER_API_TOKEN` 访问 Go 网关。

文档：<https://0702hjj.github.io/SourceAEC/>。贡献与授权要求见
[CONTRIBUTING.md](CONTRIBUTING.md)，许可证与依赖声明见 [NOTICE](NOTICE)。

## 开发来源声明

SourceAEC 是一项独立实现，其公开技术背景包括 SimpleCADAPI 项目及其发表于
*Computer-Aided Design* 的相关研究、公开 IFC 标准、有文档记录的开源接口和其他
公开技术资料。任何第三方的私有或保密仓库、源代码、内部文档、客户数据、图纸或
其他非公开技术材料，均未被有意用作实现输入或复制到本项目中。公开 Git 历史始于
首次源码快照；此后的开发由 commit 和 Pull Request 记录。

本项目在维护者实习合同约定的职责范围之外独立开展，并非为履行实习工作任务而
开发；未利用实习单位的物质技术条件，完全基于公开资料及个人自费取得的设备、
账号、算力与其他资源独立实现。

所有贡献都必须具有可说明的合法来源。贡献者须确认其有权提交相关内容，披露所含
第三方材料及其许可证，并且不得提交保密信息或商业秘密。DCO 签署和来源要求详见
[CONTRIBUTING.md](CONTRIBUTING.md)。如认为仓库中的材料侵犯了你的合法权益，请
通过 Issue 或私密渠道提供受影响的路径和权利依据，以便及时调查处理。不要在公开
Issue 中披露保密材料或个人数据；如可用，应优先使用 GitHub 私密安全报告或私下联系
维护者。

项目的独立来源、开发记录、权利立场和第三方许可边界详见
[PROJECT_PROVENANCE.md](PROJECT_PROVENANCE.md)。

## 使用边界与保证

SourceAEC 是开发者工具和参考实现，不是安全关键、法定审查、测绘或施工审批系统。
生成的 IFC/DXF 内容、AI 建议、转换结果和沙箱结果，在用于设计、采购、施工、运行
或合规决定前，必须由具备相应资质的人员独立复核。本项目不承诺输出准确、适用于某
一特定用途、能够与所有下游工具互操作、符合任何法律或标准，或不涉及第三方权利
主张。适用的保证和责任边界以 Apache-2.0 许可证及所含依赖的许可证/声明为准。

## 许可证

SourceAEC 自有材料采用 Apache-2.0。合并后的 `skills/sourceaec` 在 `licenses/` 中
保留源自旧规划与 DXF skill 材料的 MIT 声明。第三方运行时见 [NOTICE](NOTICE)。
IFC Web 体验包含相互独立的 xeokit 与 web-ifc 查看路径，但当前标准 Web 构建同时
包含两者；运行时切换到 web-ifc 并不会从该构建中移除 xeokit，也不会自动消除其
AGPL-3.0 义务。仓库内的 web-ifc WASM 文件仍受 MPL-2.0 约束。
