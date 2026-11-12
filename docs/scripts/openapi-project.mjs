// SPDX-License-Identifier: Apache-2.0
function put(object, name, value) {
  Object.defineProperty(object, name, { value, enumerable: true, writable: true, configurable: true })
}

function select(source, names, label) {
  if (!Array.isArray(names)) throw new Error(`invalid projection ${label} selection`)
  const result = {}
  for (const name of names) {
    if (typeof name !== 'string' || !Object.hasOwn(source, name)) throw new Error(`unknown canonical ${label}: ${name}`)
    if (Object.hasOwn(result, name)) throw new Error(`duplicate projected ${label}: ${name}`)
    put(result, name, source[name])
  }
  return result
}

// Keep existing subset names and bring in any components newly referenced by
// their canonical definitions. This does not expose additional API paths.
export function projectContract(document, canonical, selection) {
  document.paths = select(canonical.paths, selection.paths, 'path')
  document.components.schemas = select(canonical.components.schemas, selection.schemas, 'schema')
  for (const [category, definitions] of Object.entries(document.components)) {
    if (category === 'schemas') continue
    for (const name of Object.keys(definitions)) {
      if (!canonical.components[category] || !Object.hasOwn(canonical.components[category], name)) {
        throw new Error(`unknown canonical component: ${category}/${name}`)
      }
      put(definitions, name, canonical.components[category][name])
    }
  }
  const visited = new Set()
  function visit(value) {
    if (!value || typeof value !== 'object' || visited.has(value)) return
    visited.add(value)
    if (value.$ref) {
      if (!value.$ref.startsWith('#/components/')) throw new Error(`unsupported canonical reference: ${value.$ref}`)
      const [, , category, name] = value.$ref.split('/').map(part => part.replaceAll('~1', '/').replaceAll('~0', '~'))
      if (!Object.hasOwn(canonical.components, category) || !Object.hasOwn(canonical.components[category], name)) {
        throw new Error(`unresolved canonical component: ${category}/${name}`)
      }
      if (!Object.hasOwn(document.components, category)) put(document.components, category, {})
      if (!Object.hasOwn(document.components[category], name)) {
        put(document.components[category], name, canonical.components[category][name])
        visit(document.components[category][name])
      }
    }
    Object.values(value).forEach(visit)
  }
  visit(document)
  return document
}
