import type { HierarchyNode, PertTask } from '../../../lib/diagram-types'
export interface PlacedNode { node: HierarchyNode; x: number; y: number; depth: number; parent?: string }
export function treeLayout(root: HierarchyNode, cardWidth = 170, cardHeight = 84) {
  const nodes: PlacedNode[] = []; let cursor = 0; let maxDepth = 0
  const walk = (node: HierarchyNode, depth: number, parent?: string): number => {
    maxDepth = Math.max(maxDepth, depth)
    const children = node.children || []
    const positions = children.map(child => walk(child, depth + 1, node.id))
    const x = positions.length ? (positions[0]! + positions[positions.length - 1]!) / 2 : cursor++ * (cardWidth + 36) + cardWidth / 2 + 50
    nodes.push({ node, x, y: depth * (cardHeight + 116) + 80, depth, parent })
    return x
  }
  walk(root, 0)
  const width = Math.max(900, cursor * (cardWidth + 36) + 100)
  const height = Math.max(600, (maxDepth + 1) * (cardHeight + 116) + 50)
  const minX = Math.min(...nodes.map(item => item.x - cardWidth / 2)), maxX = Math.max(...nodes.map(item => item.x + cardWidth / 2))
  const minY = Math.min(...nodes.map(item => item.y - cardHeight / 2)), maxY = Math.max(...nodes.map(item => item.y + cardHeight / 2))
  const offsetX = (width - maxX + minX) / 2 - minX, offsetY = (height - maxY + minY) / 2 - minY
  nodes.forEach(item => { item.x += offsetX; item.y += offsetY })
  return { nodes, width, height }
}
export function pertLayout(tasks: PertTask[]) {
  const byId = new Map(tasks.map(task => [task.id, task])); const ordered: PertTask[] = []; const visited = new Set<string>(); const visiting = new Set<string>()
  const visit = (task: PertTask) => {
    if (visiting.has(task.id)) throw new Error('任务依赖存在循环，无法计算关键路径')
    if (visited.has(task.id)) return
    visiting.add(task.id)
    for (const id of task.dependsOn) { const dependency = byId.get(id); if (!dependency) throw new Error(`不存在的依赖任务：${id}`); visit(dependency) }
    visiting.delete(task.id); visited.add(task.id); ordered.push(task)
  }
  tasks.forEach(visit)
  const earliest = new Map<string, number>(); const ranks = new Map<string, number>()
  for (const task of ordered) {
    earliest.set(task.id, Math.max(0, ...task.dependsOn.map(id => earliest.get(id)! + byId.get(id)!.duration)))
    ranks.set(task.id, Math.max(0, ...task.dependsOn.map(id => ranks.get(id)! + 1)))
  }
  const duration = Math.max(0, ...tasks.map(task => earliest.get(task.id)! + task.duration)); const latest = new Map<string, number>()
  for (const task of [...ordered].reverse()) {
    const next = tasks.filter(item => item.dependsOn.includes(task.id))
    latest.set(task.id, next.length ? Math.min(...next.map(item => latest.get(item.id)! - task.duration)) : duration - task.duration)
  }
  const buckets = new Map<number, PertTask[]>(); tasks.forEach(task => { const rank = ranks.get(task.id)!; buckets.set(rank, [...(buckets.get(rank) || []), task]) })
  const maxRows = Math.max(1, ...[...buckets.values()].map(items => items.length)); const height = Math.max(720, maxRows * 246 + 230)
  const nodes = tasks.map(task => {
    const rank = ranks.get(task.id)!, siblings = buckets.get(rank)!, row = siblings.indexOf(task), blockHeight = siblings.length * 174 + Math.max(0, siblings.length - 1) * 72
    const early = earliest.get(task.id)!, late = latest.get(task.id)!
    return { task, x: 70 + rank * 282, y: 140 + (height - 270 - blockHeight) / 2 + row * 246, early, late, critical: Math.abs(early - late) < 1e-8 }
  })
  return { nodes, duration, width: Math.max(1200, Math.max(0, ...ranks.values()) * 282 + 364), height }
}
export const graphColors = ['#6265e8', '#4eb8a9', '#d89c42', '#a18ae0', '#6c9ee8', '#d37ba2']
