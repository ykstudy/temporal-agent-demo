import type { BusinessItem, BusinessLink, BusinessPort } from '../../../data/diagrams/business'
import type { DiagramSelection } from '../../../lib/diagram-types'

export interface Box { id: string; x: number; y: number; width: number; height: number }
export interface Point { x: number; y: number }
export function selectionLabel(item: BusinessItem, nodes: readonly BusinessItem[] = [], type = '图形元素'): string {
  const label = item.label.trim()
  if (label) return label
  const link = item as Partial<BusinessLink>
  if (typeof link.source === 'string' && typeof link.target === 'string') {
    const endpointLabel = (id: string, fallback: string) => nodes.find(node => node.id === id)?.label.trim() || id.trim() || fallback
    return `${endpointLabel(link.source, '来源节点')} → ${endpointLabel(link.target, '目标节点')}`
  }
  return `${type.trim() || '图形元素'}${item.id.trim() ? ` ${item.id.trim()}` : ''}`
}
export function selection(item: BusinessItem, type: string, nodes: readonly BusinessItem[] = []): DiagramSelection { return { id: item.id, label: selectionLabel(item, nodes, type), type, description: item.description, details: item.details } }
export function anchor(box: Box, port: BusinessPort): Point {
  return port === 'left' ? { x: box.x - box.width / 2, y: box.y } : port === 'right' ? { x: box.x + box.width / 2, y: box.y } : port === 'top' ? { x: box.x, y: box.y - box.height / 2 } : { x: box.x, y: box.y + box.height / 2 }
}
export function route(edge: BusinessLink, boxes: Box[]): { path: string; label: Point; start: Point; end: Point } | undefined {
  const source = boxes.find(box => box.id === edge.source), target = boxes.find(box => box.id === edge.target)
  if (!source || !target) return
  const horizontal = Math.abs(target.x - source.x) > Math.abs(target.y - source.y)
  const from = edge.from || (horizontal ? target.x >= source.x ? 'right' : 'left' : target.y >= source.y ? 'bottom' : 'top')
  const to = edge.to || (horizontal ? target.x >= source.x ? 'left' : 'right' : target.y >= source.y ? 'top' : 'bottom')
  const start = anchor(source, from), end = anchor(target, to)
  let points: Point[]
  if (edge.via?.length) {
    const hints = [start, ...edge.via, end]
    points = [start]
    for (let index = 1; index < hints.length; index++) {
      const previous = points.at(-1)!, next = hints[index]!
      if (Math.abs(previous.x - next.x) > 1 && Math.abs(previous.y - next.y) > 1) {
        const horizontalFirst = index === 1 ? from === 'left' || from === 'right' : index === hints.length - 1 ? to === 'top' || to === 'bottom' : true
        points.push(horizontalFirst ? { x: next.x, y: previous.y } : { x: previous.x, y: next.y })
      }
      points.push(next)
    }
    points = points.filter((point, index, array) => index === 0 || index === array.length - 1 || !((array[index - 1]!.x === point.x && array[index + 1]!.x === point.x) || (array[index - 1]!.y === point.y && array[index + 1]!.y === point.y)))
  }
  else if (Math.abs(start.x - end.x) < 1 || Math.abs(start.y - end.y) < 1) points = [start, end]
  else if (from === 'left' || from === 'right') {
    if (to === 'left' || to === 'right') { const middle = (start.x + end.x) / 2; points = [start, { x: middle, y: start.y }, { x: middle, y: end.y }, end] }
    else points = [start, { x: end.x, y: start.y }, end]
  } else if (to === 'top' || to === 'bottom') { const middle = (start.y + end.y) / 2; points = [start, { x: start.x, y: middle }, { x: end.x, y: middle }, end] }
  else points = [start, { x: start.x, y: end.y }, end]
  return { path: points.map((point, index) => `${index ? 'L' : 'M'} ${point.x} ${point.y}`).join(' '), label: edge.labelPosition || { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 - 12 }, start, end }
}
export function truncate(text: string, max = 17): string { return text.length > max ? `${text.slice(0, max - 1)}…` : text }
export function lines(text: string, max = 12): string[] { if (!text) return []; const parts: string[] = []; for (let index = 0; index < text.length; index += max) parts.push(text.slice(index, index + max)); return parts }
