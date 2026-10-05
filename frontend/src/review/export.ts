import { snapshot } from './diagrams/lib/diagram-export'
import type { ReviewBundle, ReviewModel } from './types'

function download(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob), link = document.createElement('a')
  link.href = url; link.download = name; document.body.appendChild(link); link.click(); link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
function crc32(bytes: Uint8Array): number {
  let crc = 0xffffffff
  for (const byte of bytes) { crc ^= byte; for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0) }
  return (crc ^ 0xffffffff) >>> 0
}
function pngText(bytes: Uint8Array, metadata: object): Uint8Array {
  // PNG tEXt is Latin-1; JSON Unicode escapes preserve non-ASCII source labels.
  const ascii = JSON.stringify(metadata).replace(/[\u007f-\uffff]/g, char => '\\u' + char.charCodeAt(0).toString(16).padStart(4, '0'))
  const data = new TextEncoder().encode('AgentStudio\0' + ascii), type = new TextEncoder().encode('tEXt')
  const chunk = new Uint8Array(data.length + 12), view = new DataView(chunk.buffer)
  view.setUint32(0, data.length); chunk.set(type, 4); chunk.set(data, 8); view.setUint32(data.length + 8, crc32(chunk.subarray(4, data.length + 8)))
  // Insert after IHDR (signature 8 + length/type/data/CRC 25).
  const result = new Uint8Array(bytes.length + chunk.length)
  result.set(bytes.subarray(0, 33)); result.set(chunk, 33); result.set(bytes.subarray(33), 33 + chunk.length)
  return result
}
export async function exportReview(svg: SVGSVGElement, format: 'svg' | 'png', model: ReviewModel, bundle: ReviewBundle, filters: object) {
  const image = await snapshot(svg)
  const meta = { model_id: model.id, diagram_type: model.diagram_type, scope: model.scope, status: model.status, baseline: bundle.baseline, generated_at: bundle.generated_at, exported_at: new Date().toISOString(), filters, sources: model.source_ids.map(id => { const source = bundle.sources.find(item => item.id === id)!; return { path: source.path, sha256: source.sha256 } }) }
  const documentSvg = new DOMParser().parseFromString(image.source, 'image/svg+xml')
  const root = documentSvg.documentElement, ns = 'http://www.w3.org/2000/svg'
  const metadata = documentSvg.createElementNS(ns, 'metadata'); metadata.textContent = JSON.stringify(meta); root.prepend(metadata)
  const stamp = documentSvg.createElementNS(ns, 'text')
  stamp.setAttribute('x', '24'); stamp.setAttribute('y', '24'); stamp.setAttribute('font-size', '13'); stamp.setAttribute('fill', getComputedStyle(svg).getPropertyValue('--dg-muted').trim() || '#6d7890')
  stamp.textContent = `${model.scope === 'design' ? '规范目标' : model.scope} · ${model.id} · contract ${bundle.baseline.contract_version} · ${bundle.generated_at}`
  root.append(stamp)
  const source = '<?xml version="1.0" encoding="UTF-8"?>\n' + new XMLSerializer().serializeToString(root)
  const name = `agent-review-${model.id}.${format}`
  if (format === 'svg') { download(new Blob([source], { type: 'image/svg+xml;charset=utf-8' }), name); return }
  const ratio = Math.min(2, 8192 / image.width, 8192 / image.height)
  const canvas = document.createElement('canvas'); canvas.width = Math.max(1, Math.round(image.width * ratio)); canvas.height = Math.max(1, Math.round(image.height * ratio))
  const context = canvas.getContext('2d'); if (!context) throw new Error('无法创建图片画布。')
  const url = URL.createObjectURL(new Blob([source], { type: 'image/svg+xml;charset=utf-8' }))
  try {
    const png = new Image()
    await new Promise<void>((resolve, reject) => { png.onload = () => resolve(); png.onerror = () => reject(new Error('导出 SVG 无法解码。')); png.src = url })
    context.drawImage(png, 0, 0, canvas.width, canvas.height)
    const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob(value => value ? resolve(value) : reject(new Error('PNG 生成失败。')), 'image/png'))
    const bytes = pngText(new Uint8Array(await blob.arrayBuffer()), meta)
    download(new Blob([bytes as Uint8Array<ArrayBuffer>], { type: 'image/png' }), name)
  } finally { URL.revokeObjectURL(url) }
}
