// SPDX-License-Identifier: Apache-2.0
import { readdir, readFile } from 'node:fs/promises'

const directory = new URL('../../server/internal/api/', import.meta.url)

// Documentation inventory only. Unknown registration syntax fails explicitly.
export async function registeredRoutes() {
  const routes = []
  for (const file of await readdir(directory)) {
    if (!file.endsWith('.go') || file.endsWith('_test.go')) continue
    let source = await readFile(new URL(file, directory), 'utf8')
    source = source.replace(/for _, (\w+) := range \[\]string\{([^}]+)\} \{\s*(\w+\.HandleFunc)\("([^"]+)"\+(\w+), ([^\n]+)\)\s*\}/g,
      (whole, variable, values, register, prefix, used, handler) => {
        if (variable !== used) throw new Error(`unsupported route loop: ${file}`)
        const actions = [...values.matchAll(/"([^"]+)"/g)].map(m => m[1])
        if (!actions.length) throw new Error(`empty route loop: ${file}`)
        return actions.map(action => `${register}("${prefix}${action}", ${handler.replaceAll(`(${variable})`, `("${action}")`)})`).join('\n')
      })
    const lines = source.split('\n').filter(line => /\.HandleFunc\(/.test(line))
    for (const line of lines) {
      const match = line.match(/\.HandleFunc\("(GET|PUT|POST|DELETE|PATCH) ([^"]+)", (.+)\)$/)
      if (!match) throw new Error(`unsupported route registration: ${file}: ${line.trim()}`)
      routes.push({ method: match[1], path: match[2], handler: match[3], file: `server/internal/api/${file}` })
    }
  }
  return routes.sort((a, b) => `${a.path} ${a.method}`.localeCompare(`${b.path} ${b.method}`))
}

