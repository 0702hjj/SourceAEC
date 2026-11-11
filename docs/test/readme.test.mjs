// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'
import test from 'node:test'

const root = fileURLToPath(new URL('../../', import.meta.url))

test('both README data flow diagrams parse and retain their architecture diagram', async () => {
  const { default: mermaid } = await import('mermaid')
  for (const name of ['README.md', 'README.zh-CN.md']) {
    const text = await readFile(join(root, name), 'utf8')
    assert.match(text, /```mermaid\nflowchart /, `${name}: architecture`)
    const diagrams = [...text.matchAll(/```mermaid\n(sequenceDiagram[\s\S]*?)```/g)]
    assert.ok(diagrams.length > 0, `${name}: main data flow`)
    for (const [, source] of diagrams) {
      await assert.doesNotReject(() => mermaid.parse(source), name)
    }
  }
})
