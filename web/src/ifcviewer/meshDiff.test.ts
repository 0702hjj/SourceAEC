// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import { describe, it, expect } from "vitest";
import { groupMeshes, sameGeometry } from "./meshDiff";
import type { IfcMeshData } from "./ifcLoader";
const mesh: IfcMeshData = { guid: "stable", expressID: 1,
  positions: new Float32Array([1, 2, 3]), indices: new Uint32Array([0]),
  transform: [1, 0, 0], color: { x: 1, y: 1, z: 1, w: 1 } };
describe("incremental mesh identity", () => {
  it("retains geometry when STEP numbering changes", () => {
    const next = { ...mesh, expressID: 20 };
    expect([...groupMeshes([next]).keys()]).toEqual(["stable"]);
    expect(sameGeometry([mesh], [next])).toBe(true);
  });
  it("detects geometry, placement and appearance changes", () => {
    for (const next of [{ ...mesh, positions: new Float32Array([2, 2, 3]) },
      { ...mesh, transform: [2, 0, 0] }, { ...mesh, color: { ...mesh.color, w: 0.5 } }]) {
      expect(sameGeometry([mesh], [next])).toBe(false);
    }
  });
});
