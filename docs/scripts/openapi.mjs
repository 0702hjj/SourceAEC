// SPDX-License-Identifier: Apache-2.0
import { readFile, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { projectContract } from './openapi-project.mjs'

export const contractNames = ['go-server', 'ai-tools']
const sourceRoot = new URL('../openapi/', import.meta.url)
const publicRoot = new URL('../site/public/', import.meta.url)

async function readJSON(url) {
  const source = await readFile(url, 'utf8')
  const lines = source.split('\n').length - Number(source.endsWith('\n'))
  if (lines >= 500) throw new Error(`contract source must remain below 500 lines: ${url.pathname}`)
  return JSON.parse(source)
}

function moduleURL(directory, filename) {
  if (typeof filename !== 'string' || !/^[a-zA-Z0-9_.-]+\.json$/.test(filename)) {
    throw new Error(`invalid contract module filename: ${filename}`)
  }
  return new URL(filename, directory)
}

async function section(root, name) {
  const directory = new URL(`${name}/`, root)
  const index = await readJSON(new URL('index.json', directory))
  if (!index || Array.isArray(index) || typeof index !== 'object') throw new Error(`invalid ${name} index`)
  const values = {}
  const used = new Set()
  for (const [key, filename] of Object.entries(index)) {
    if (filename === 'index.json' || used.has(filename)) throw new Error(`duplicate/reserved ${name} module: ${filename}`)
    used.add(filename)
    Object.defineProperty(values, key, {
      value: await readJSON(moduleURL(directory, filename)), enumerable: true,
    })
  }
  return values
}

// Modules are merged before publication, so component references remain local
// to the original single-file document instead of becoming external URL refs.
export async function assembleContract(name, root = sourceRoot, ancestors = []) {
  if (ancestors.includes(name)) throw new Error(`cyclic contract projection: ${name}`)
  if (!contractNames.includes(name)) throw new Error(`unknown contract: ${name}`)
  const directory = new URL(`${name}/`, root)
  const document = await readJSON(new URL('base.json', directory))
  const projection = await readJSON(new URL('source.json', directory)).catch(error => {
    if (error.code === 'ENOENT') return null
    throw error
  })
  if (projection) {
    const canonical = await assembleContract(projection.contract, root, [...ancestors, name])
    return projectContract(document, canonical, projection)
  }
  document.paths = await section(directory, 'paths')
  for (const name of ['schemas', 'responses']) {
    if (Object.hasOwn(document.components, name)) document.components[name] = await section(directory, name)
  }
  return document
}

async function main() {
  const args = process.argv.slice(2)
  if (args.length !== 1 || !['--check', '--write'].includes(args[0])) {
    throw new Error('usage: node scripts/openapi.mjs --check|--write')
  }
  for (const name of contractNames) {
    const data = JSON.stringify(await assembleContract(name), null, 2) + '\n'
    const file = new URL(`${name}.openapi.json`, publicRoot)
    if (args[0] === '--write') await writeFile(file, data)
    else if (await readFile(file, 'utf8') !== data) throw new Error(`stale ${name} snapshot; run npm run api:build`)
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main().catch(error => { process.stderr.write(`${error.message}\n`); process.exitCode = 1 })
}
