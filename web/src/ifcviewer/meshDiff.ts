// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import type { IfcMeshData } from "./ifcLoader";

export function meshKey(mesh: IfcMeshData): string {
  return mesh.guid ?? `express:${mesh.expressID}`;
}
function equalArray(a: ArrayLike<number>, b: ArrayLike<number>) {
  return a.length === b.length && Array.from(a).every((value, i) => value === b[i]);
}
export function sameGeometry(a: IfcMeshData[], b: IfcMeshData[]) {
  return a.length === b.length && a.every((mesh, i) => {
    const other = b[i];
    return equalArray(mesh.positions, other.positions) && equalArray(mesh.indices, other.indices)
      && equalArray(mesh.transform, other.transform)
      && mesh.color.x === other.color.x && mesh.color.y === other.color.y
      && mesh.color.z === other.color.z && mesh.color.w === other.color.w;
  });
}
export function groupMeshes(meshes: IfcMeshData[]) {
  const groups = new Map<string, IfcMeshData[]>();
  for (const mesh of meshes) {
    const key = meshKey(mesh);
    const group = groups.get(key) ?? [];
    group.push(mesh); groups.set(key, group);
  }
  return groups;
}
