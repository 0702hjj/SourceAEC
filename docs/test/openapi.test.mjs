// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict'
import { readFile, readdir } from 'node:fs/promises'
import test from 'node:test'
import { assembleContract, contractNames } from '../scripts/openapi.mjs'

const root = new URL('../openapi/', import.meta.url)
async function files(directory) {
  return (await Promise.all((await readdir(directory, { withFileTypes: true })).map(async entry => {
    const url = new URL(entry.name + (entry.isDirectory() ? '/' : ''), directory)
    return entry.isDirectory() ? files(url) : [url]
  }))).flat()
}

test('bounded API sources deterministically reproduce the exact public snapshots', async () => {
  for (const name of contractNames) {
    const document = await assembleContract(name)
    const published = await readFile(new URL(`../site/public/${name}.openapi.json`, import.meta.url), 'utf8')
    assert.equal(JSON.stringify(document, null, 2) + '\n', published, name)
  }
})

test('all maintained contract JSON modules remain below 500 lines', async () => {
  const sources = (await files(root)).filter(url => url.pathname.endsWith('.json'))
  assert.ok(sources.length > 100, 'both contracts must have responsibility-sized sources')
  for (const url of sources) {
    const source = await readFile(url, 'utf8')
    const lines = source.split('\n').length - Number(source.endsWith('\n'))
    assert.ok(lines < 500, `${url.pathname}: ${lines} lines`)
    assert.doesNotThrow(() => JSON.parse(source), url.pathname)
  }
})

test('both assembled contracts have resolvable references and unique operation IDs', async () => {
  for (const name of contractNames) {
    const document = await assembleContract(name)
    const ids = new Set()
    function visit(value) {
      if (!value || typeof value !== 'object') return
      if (value.$ref) {
        assert.ok(value.$ref.startsWith('#/'), `${name}: unexpected external reference`)
        const target = value.$ref.slice(2).split('/').reduce((v, key) => v?.[key.replaceAll('~1', '/').replaceAll('~0', '~')], document)
        assert.ok(target, `${name}: unresolved ${value.$ref}`)
      }
      if (value.operationId) {
        assert.ok(!ids.has(value.operationId), `${name}: duplicate ${value.operationId}`)
        ids.add(value.operationId)
      }
      Object.values(value).forEach(visit)
    }
    visit(document)
  }
})

test('file-size gate checks maintained JSON sources and registered generated outputs', async () => {
  const { execFile } = await import('node:child_process')
  const { promisify } = await import('node:util')
  const { fileURLToPath } = await import('node:url')
  await promisify(execFile)('bash', [fileURLToPath(new URL('../../scripts/test_file_size_gate.sh', import.meta.url))], { timeout: 15000 })
})

test('assembler rejects directory traversal, duplicate mappings and oversized sources', async t => {
  const { mkdtemp, mkdir, writeFile, rm } = await import('node:fs/promises')
  const { tmpdir } = await import('node:os')
  const { join } = await import('node:path')
  const { pathToFileURL } = await import('node:url')
  const temporary = await mkdtemp(join(tmpdir(), 'sourceaec-contract-'))
  t.after(() => rm(temporary, { recursive: true, force: true }))
  const root = pathToFileURL(temporary + '/')
  await mkdir(new URL('go-server/paths/', root), { recursive: true })
  await writeFile(new URL('go-server/base.json', root), JSON.stringify({ paths: {}, components: {} }))
  const index = new URL('go-server/paths/index.json', root)
  await writeFile(index, JSON.stringify({ '/probe': '../private.json' }))
  await assert.rejects(() => assembleContract('go-server', root), /invalid contract module/)
  await writeFile(new URL('go-server/paths/probe.json', root), '{}\n')
  await writeFile(index, JSON.stringify({ '/a': 'probe.json', '/b': 'probe.json' }))
  await assert.rejects(() => assembleContract('go-server', root), /duplicate/)
  await writeFile(index, JSON.stringify({ '/probe': 'probe.json' }))
  await writeFile(new URL('go-server/paths/probe.json', root), '\n'.repeat(500) + '{}\n')
  await assert.rejects(() => assembleContract('go-server', root), /below 500 lines/)
})

test('CLI checks are read-only and stale snapshots require an explicit build', async t => {
  const { mkdtemp, mkdir, writeFile, rm, copyFile } = await import('node:fs/promises')
  const { tmpdir } = await import('node:os')
  const { join } = await import('node:path')
  const { promisify } = await import('node:util')
  const { execFile } = await import('node:child_process')
  const temporary = await mkdtemp(join(tmpdir(), 'sourceaec-contract-cli-'))
  t.after(() => rm(temporary, { recursive: true, force: true }))
  await mkdir(join(temporary, 'scripts'))
  await mkdir(join(temporary, 'site/public'), { recursive: true })
  const script = join(temporary, 'scripts/openapi.mjs')
  await copyFile(new URL('../scripts/openapi.mjs', import.meta.url), script)
  for (const name of contractNames) {
    await mkdir(join(temporary, `openapi/${name}/paths`), { recursive: true })
    await writeFile(join(temporary, `openapi/${name}/base.json`), JSON.stringify({ paths: {}, components: {} }))
    await writeFile(join(temporary, `openapi/${name}/paths/index.json`), '{}')
    await writeFile(join(temporary, `site/public/${name}.openapi.json`), '{"stale":true}\n')
  }
  const execute = promisify(execFile)
  await assert.rejects(() => execute(process.execPath, [script, '--check']), /stale go-server snapshot/)
  assert.equal(await readFile(join(temporary, 'site/public/go-server.openapi.json'), 'utf8'), '{"stale":true}\n')
  await execute(process.execPath, [script, '--write'])
  await execute(process.execPath, [script, '--check'])
  for (const name of contractNames) {
    assert.deepEqual(JSON.parse(await readFile(join(temporary, `site/public/${name}.openapi.json`), 'utf8')), { paths: {}, components: {} })
  }
})
