import type { TechnicalNode, TechnicalConnection } from '../../../data/diagrams/technical'
import type { DiagramSelection } from '../../../lib/diagram-types'

export interface Box { x: number; y: number; w: number; h: number }
export interface Point { x: number; y: number }
export function nodeSelection(node: TechnicalNode, type = '节点', extra: string[] = []): DiagramSelection {
  return { id: node.id, label: node.label, type, description: node.description || node.subtitle, details: [...(node.details || []), ...extra] }
}
export function edgeSelection(edge: TechnicalConnection, type = '关系'): DiagramSelection {
  return { id: edge.id, label: edge.label || `${edge.source} → ${edge.target}`, type, description: edge.description, details: [`来源：${edge.source}`, `去向：${edge.target}`] }
}
export function textWidth(text = '', size = 16) { return [...text].reduce((sum, character) => sum + (/[^\u0000-\u007f]/.test(character) ? size : size * .56), 0) }
export function center(box: Box): Point { return { x: box.x + box.w / 2, y: box.y + box.h / 2 } }
export function route(a: Box, b: Box, vertical = false) {
  const ca = center(a), cb = center(b)
  if (vertical && Math.abs(ca.y - cb.y) > Math.max(a.h, b.h)) {
    const downward = cb.y > ca.y, from = { x: ca.x, y: downward ? a.y + a.h : a.y }, to = { x: cb.x, y: downward ? b.y : b.y + b.h }
    const mid = (from.y + to.y) / 2
    return { d: `M ${from.x} ${from.y} C ${from.x} ${mid}, ${to.x} ${mid}, ${to.x} ${to.y}`, label: { x: (from.x + to.x) / 2, y: mid } }
  }
  if (Math.abs(ca.x - cb.x) > Math.max(a.w, b.w)) {
    const right = cb.x > ca.x, from = { x: right ? a.x + a.w : a.x, y: ca.y }, to = { x: right ? b.x : b.x + b.w, y: cb.y }
    const mid = (from.x + to.x) / 2
    return { d: `M ${from.x} ${from.y} C ${mid} ${from.y}, ${mid} ${to.y}, ${to.x} ${to.y}`, label: { x: mid, y: (from.y + to.y) / 2 } }
  }
  const down = cb.y > ca.y, from = { x: ca.x, y: down ? a.y + a.h : a.y }, to = { x: cb.x, y: down ? b.y : b.y + b.h }, mid = (from.y + to.y) / 2
  return { d: `M ${from.x} ${from.y} C ${from.x} ${mid}, ${to.x} ${mid}, ${to.x} ${to.y}`, label: { x: (from.x + to.x) / 2, y: mid } }
}

