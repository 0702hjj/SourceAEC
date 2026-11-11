# 文档构建与托管

文档源位于 `docs/site`。中文使用根路径，英文使用 `/en/`；站点 base 为 `/SourceAEC/`。
两种语言共享 API JSON 文件，正文内部使用 root-relative VitePress 链接。

## 本地验证

```bash
cd docs
npm ci
npm test
npm run docs:build
npm run docs:dev
```

`npm test` 检查双语页面覆盖和迁移链接契约；构建检查站内死链并生成本地搜索索引。
Mermaid 代码块由站点插件渲染。业务架构图描述实际实现，不表示宿主机器或外部 SDK 的目标架构。

## GitHub Pages 发布

仓库内的 `Documentation Pages` 工作流只允许手动触发，并只发布 `main`。
PR 的 Verify 构建文档但不发布。GitHub Pages 只托管静态文档，不运行 Go 网关或数据库。

1. 先审核并合并文档及工作流到 main。
2. 管理员在仓库 **Settings → Pages → Build and deployment → Source** 选择 **GitHub Actions**。
   若 Pages 尚未启用，此步骤会创建站点；工作流不擅自修改仓库设置。
3. 在 **Actions → Documentation Pages → Run workflow** 选择 main。
4. 检查 build 与 deploy 两个 job，随后访问 `https://0702hjj.github.io/SourceAEC/` 和 `/SourceAEC/en/`。

工作流使用最小的 `contents: read`、`pages: write`、`id-token: write` 权限，
将 `.vitepress/dist` 上传为 Pages artifact，再经 `github-pages` environment 发布。
失败时检查 Pages source、environment 保护规则和 Actions 权限；不要把 Markdown 源目录直接当作构建结果发布。

参见 [GitHub Pages 工作流](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages)
与 [VitePress 多语言配置](https://vitepress.dev/guide/i18n)。
