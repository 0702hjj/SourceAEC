// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict'
import test from 'node:test'
import { assembleContract } from '../scripts/openapi.mjs'

test('agent contracts expose the native source revision and diff fields', async () => {
  const { components: { schemas } } = await assembleContract('ai-tools')
  assert.ok(schemas.ScriptStatus.required.includes('revision'), 'agent status omits the current source revision')
  assert.ok(schemas.ScriptDiffResult.required.includes('text_diff'), 'agent diff uses retired response fields')
  assert.equal(schemas.SaveBody.properties.source.default, 'human')
})

test('agent path definitions and shared schemas agree with the canonical Go contract', async () => {
  const go = await assembleContract('go-server')
  const agent = await assembleContract('ai-tools')
  assert.equal(Object.keys(agent.paths).length, 20, 'retain the existing agent path surface')
  for (const [path, definition] of Object.entries(agent.paths)) {
    assert.deepEqual(definition, go.paths[path], path)
  }
  for (const [name, definition] of Object.entries(agent.components.schemas)) {
    assert.deepEqual(definition, go.components.schemas[name], name)
  }
})

test('projection includes transitive components without expanding path exposure', async () => {
  const { projectContract } = await import('../scripts/openapi-project.mjs')
  const canonical = {
    paths: {
      '/selected': { get: { response: { $ref: '#/components/schemas/Result' } } },
      '/hidden': { get: {} },
    },
    components: { schemas: {
      Result: { type: 'object', properties: { value: { $ref: '#/components/schemas/Leaf' } } },
      Leaf: { type: 'string' },
      Unused: { type: 'number' },
    } },
  }
  const base = () => ({ paths: {}, components: { schemas: {} } })
  const selected = projectContract(base(), canonical, { paths: ['/selected'], schemas: [] })
  assert.deepEqual(Object.keys(selected.paths), ['/selected'])
  assert.deepEqual(Object.keys(selected.components.schemas), ['Result', 'Leaf'])
  assert.throws(() => projectContract(base(), canonical, { paths: ['/unknown'], schemas: [] }), /unknown canonical path/)
  assert.throws(() => projectContract(base(), canonical, { paths: ['/selected', '/selected'], schemas: [] }), /duplicate/)
  const broken = structuredClone(canonical)
  broken.paths['/selected'].get.response.$ref = '#/components/schemas/Missing'
  assert.throws(() => projectContract(base(), broken, { paths: ['/selected'], schemas: [] }), /unresolved canonical component/)
})

test('assembler rejects cyclic contract projections', async t => {
  const { mkdtemp, mkdir, writeFile, rm } = await import('node:fs/promises')
  const { tmpdir } = await import('node:os')
  const { join } = await import('node:path')
  const { pathToFileURL } = await import('node:url')
  const temporary = await mkdtemp(join(tmpdir(), 'sourceaec-contract-cycle-'))
  t.after(() => rm(temporary, { recursive: true, force: true }))
  for (const [name, source] of [['go-server', 'ai-tools'], ['ai-tools', 'go-server']]) {
    await mkdir(join(temporary, name))
    await writeFile(join(temporary, name, 'base.json'), JSON.stringify({ paths: {}, components: { schemas: {} } }))
    await writeFile(join(temporary, name, 'source.json'), JSON.stringify({ contract: source, paths: [], schemas: [] }))
  }
  await assert.rejects(() => assembleContract('ai-tools', pathToFileURL(temporary + '/')), /cyclic contract projection/)
})
