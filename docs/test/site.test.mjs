// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict'
import { readdir, readFile, access } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { join, relative } from 'node:path'
import test from 'node:test'

const root = fileURLToPath(new URL('../site/', import.meta.url))
async function markdown(directory = root) {
  const entries = await readdir(directory, { withFileTypes: true })
  return (await Promise.all(entries.filter(e => !e.name.startsWith('.')).map(async e => {
    const path = join(directory, e.name)
    return e.isDirectory() ? markdown(path) : e.name.endsWith('.md') ? [path] : []
  }))).flat()
}

test('every Chinese page has a nonempty English counterpart and vice versa', async () => {
  for (const file of await markdown()) {
    const path = relative(root, file)
    const counterpart = path.startsWith('en/') ? path.slice(3) : `en/${path}`
    const text = await readFile(join(root, counterpart), 'utf8')
    assert.match(text, /^# |^layout: home/m, counterpart)
    assert.match(text, /\n\n[^#\s-]/, `${counterpart}: body content`)
  }
})

test('root-relative Markdown links resolve without losing the deployment base', async () => {
  for (const file of await markdown()) {
    const text = await readFile(file, 'utf8')
    for (const [, target] of text.matchAll(/\]\((\/[^)\s]*)\)/g)) {
      assert.ok(!target.startsWith('/SourceAEC/'), `VitePress adds base: ${file} → ${target}`)
      const path = decodeURI(target.split(/[?#]/)[0]).slice(1)
      const candidates = path.endsWith('/') || !path ? [`${path}index.md`] :
        /\.[a-z]+$/i.test(path) ? [`public/${path}`] : [`${path}.md`, `${path}/index.md`]
      const found = await Promise.all(candidates.map(p => access(join(root, p)).then(() => true, () => false)))
      assert.ok(found.some(Boolean), `${relative(root, file)} → ${target}`)
    }
  }
})

test('legacy entry points retain their chapter headings and canonical destinations', async () => {
  const cases = [
    ['guide/quickstart', 'operations/quickstart', '# 快速部署'],
    ['development/model-storage', 'architecture/storage', '## 工作流存储与恢复'],
    ['development/execution-lifecycle', 'architecture/execution', '## 资源边界'],
    ['development/web', 'architecture/web', '## 关键机制'],
    ['reference/ai', 'integrations/agents', '# AI 与 MCP'],
  ]
  for (const [old, destination, heading] of cases) {
    const text = await readFile(join(root, `${old}.md`), 'utf8')
    assert.ok(text.includes(heading), `${old}: preserve existing anchors`)
    assert.ok(text.includes(`](/${destination})`), `${old}: canonical destination`)
    assert.match(text, /search: false/)
  }
})

test('sequence diagrams parse rather than silently render an error image', async () => {
  const { default: mermaid } = await import('mermaid')
  for (const file of await markdown()) {
    const text = await readFile(file, 'utf8')
    for (const [, source] of text.matchAll(/```mermaid\n(sequenceDiagram[\s\S]*?)```/g)) {
      await assert.doesNotReject(() => mermaid.parse(source), relative(root, file))
    }
  }
})
