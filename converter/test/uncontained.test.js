// SPDX-License-Identifier: Apache-2.0
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { convertIfc } = require('../convert');

const fixture = path.join(__dirname, 'fixtures/wall-with-opening-and-window.ifc');
const wall = '3ZYW59sxj8lei475l7EhLU';
const window = '0tA4DSHd50le6Ov9Yu0I9X';
const cases = [
  ['all products', source => source.replace(/^#44 = IFCRELCONTAINEDINSPATIALSTRUCTURE[^\n]*\n/m, '')],
  ['only the wall', source => source.replace('(#45, #102), #38)', '(#102), #38)')],
  ['only the window', source => source.replace('(#45, #102), #38)', '(#45), #38)')],
];

for (const [name, change] of cases) {
  test(`metadata and preview retain ${name} without spatial containment`, async t => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'sourceaec-uncontained-'));
    t.after(() => fs.rmSync(root, { recursive: true, force: true }));
    const source = fs.readFileSync(fixture, 'utf8');
    const modified = change(source);
    assert.notEqual(modified, source, 'fixture mutation must remove containment');
    const input = path.join(root, 'uncontained.ifc');
    fs.writeFileSync(input, modified);
    const stats = await convertIfc(input, path.join(root, 'output'));
    assert.ok(stats.xktBytes > 4 * 1024, 'uncontained geometry should still render');
    const metadata = JSON.parse(fs.readFileSync(path.join(root, 'output/metadata.json'), 'utf8'));
    const ids = new Set(metadata.metaObjects.map(o => o.id));
    assert.equal(ids.size, metadata.metaObjects.length, 'metadata identities must remain unique');
    assert.ok(ids.has(wall), 'uncontained wall missing');
    assert.ok(ids.has(window), 'uncontained window missing');
    for (const object of metadata.metaObjects) {
      if (object.parent != null) assert.ok(ids.has(object.parent), `unresolved parent ${object.parent}`);
      for (const ref of object.propertySetIds || []) {
        assert.ok(metadata.propertySets.some(p => p.id === ref), `unresolved property set ${ref}`);
      }
    }
    assert.ok(metadata.metaObjects.find(o => o.id === wall).propertySetIds?.length, 'wall properties missing');
    if (name === 'only the wall') {
      const contained = metadata.metaObjects.find(o => o.id === window);
      assert.equal(metadata.metaObjects.find(o => o.id === contained.parent).type, 'IfcBuildingStorey');
    }
  });
}
