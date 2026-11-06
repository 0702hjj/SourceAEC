# Documentation hosting

Sources live in `docs/site`. Chinese uses the root, English uses `/en/`, and the deployed base is `/SourceAEC/`.
Both languages share API JSON assets. Internal Markdown links use root-relative VitePress paths.

## Local validation

```bash
cd docs
npm ci
npm test
npm run docs:build
npm run docs:dev
```

Tests enforce bilingual coverage and legacy-route contracts; the build checks internal dead links and produces local search indexes.
Mermaid fences render as diagrams. Architecture describes implemented responsibilities, not upstream SDK aspirations.

## GitHub Pages

The repository's `Documentation Pages` workflow is manual and publishes only `main`.
PR verification builds documentation without publishing. Pages hosts static docs, not the Go gateway/database.

1. Review and merge the documentation/workflow into main.
2. In **Settings → Pages → Build and deployment → Source**, select **GitHub Actions**.
   If Pages is disabled, this creates the site. The workflow does not change repository settings itself.
3. In **Actions → Documentation Pages → Run workflow**, select main.
4. Check both build/deploy jobs, then visit `https://0702hjj.github.io/SourceAEC/` and `/SourceAEC/en/`.

The workflow uses `contents: read`, `pages: write`, `id-token: write`, uploads `.vitepress/dist` as a Pages artifact,
and deploys via the `github-pages` environment. Check Pages source, environment protection and Actions permissions if deployment fails.
Do not publish raw Markdown as the built site.

See [GitHub Pages custom workflows](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages)
and [VitePress internationalization](https://vitepress.dev/guide/i18n).
