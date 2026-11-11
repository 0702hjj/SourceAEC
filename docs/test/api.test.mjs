// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import { registeredRoutes } from '../scripts/api-routes.mjs'

const load = async name => JSON.parse(await readFile(new URL(`../site/public/${name}`, import.meta.url), 'utf8'))
const key = route => `${route.method} ${route.path}`

test('public inventory and OpenAPI cover the registered Go routes exactly', async () => {
  const actual = await registeredRoutes()
  const inventory = await load('go-rest-api.routes.json')
  assert.deepEqual(inventory.endpoints.map(key).sort(), actual.map(key).sort(), 'route inventory drift')
  const spec = await load('go-server.openapi.json')
  const operations = Object.entries(spec.paths).flatMap(([path, item]) => Object.keys(item)
    .filter(method => ['get', 'put', 'post', 'delete', 'patch'].includes(method))
    .map(method => `${method.toUpperCase()} ${path}`))
  assert.deepEqual(operations.sort(), actual.map(key).sort(), 'OpenAPI route drift')
})

test('OpenAPI references and operation identifiers are valid', async () => {
  const spec = await load('go-server.openapi.json')
  const ids = new Set()
  function visit(value) {
    if (!value || typeof value !== 'object') return
    assert.ok(!Object.hasOwn(value, 'const'), 'OpenAPI 3.0 uses enum, not const')
    assert.ok(!Array.isArray(value.type), 'OpenAPI 3.0 type must be a string')
    assert.notEqual(value.type, 'null', 'OpenAPI 3.0 uses nullable')
    if (value.$ref) {
      assert.ok(value.$ref.startsWith('#/'), `unexpected external reference: ${value.$ref}`)
      const target = value.$ref.slice(2).split('/').reduce((o, part) => o?.[part.replaceAll('~1', '/').replaceAll('~0', '~')], spec)
      assert.ok(target, `unresolved ${value.$ref}`)
    }
    if (value.operationId) {
      assert.ok(!ids.has(value.operationId), `duplicate ${value.operationId}`)
      ids.add(value.operationId)
    }
    Object.values(value).forEach(visit)
  }
  visit(spec)
})

test('source inspection and diff schemas reflect native response fields', async () => {
  const { components: { schemas }, paths } = await load('go-server.openapi.json')
  assert.ok(schemas.ScriptStatus.required.includes('revision'))
  assert.ok(schemas.ScriptDiffResult.required.includes('text_diff'))
  assert.ok(schemas.ScriptDiffResult.required.includes('params_changes'))
  assert.ok(!schemas.StagingDiffResult.required.includes('engine'))
  assert.deepEqual(schemas.StagingDiffResult.required.slice(0, 2), ['from', 'to'])
  const discarded = paths['/api/v1/models/{id}/script/discard'].post.responses['200']
    .content['application/json'].schema.properties.data.properties
  assert.equal(discarded.discarded.type, 'integer')
  assert.equal(discarded.script.nullable, true)
})

test('native editing documents Git revision guards and public error translation', async () => {
  const spec = await load('go-server.openapi.json')
  const git = spec.components.schemas.GitChange
  assert.equal(git?.properties.baseRevision.pattern, '^[0-9a-f]{64}$')
  assert.ok(!git.required?.includes('baseRevision'), 'legacy omission remains accepted')
  for (const [path, item] of Object.entries(spec.paths)) {
    if (!/\/models\/\{id\}\/(git|script(?:\/|$)|scripts$|edit\/)/.test(path)) continue
    for (const operation of Object.values(item)) {
      for (const status of ['400', '404', '409', '429', '502', '503', '504']) {
        assert.ok(operation.responses[status], `${path}: missing ${status}`)
      }
      assert.doesNotMatch(operation.description ?? '', /代理|edit-service|cad-edit-service/)
    }
  }
})
