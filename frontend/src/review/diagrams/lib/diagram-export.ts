const SVG_NAMESPACE = 'http://www.w3.org/2000/svg'
const INLINE_PROPERTIES = [
  'color', 'fill', 'fill-opacity', 'fill-rule',
  'stroke', 'stroke-opacity', 'stroke-width', 'stroke-linecap', 'stroke-linejoin',
  'stroke-miterlimit', 'stroke-dasharray', 'stroke-dashoffset',
  'opacity', 'visibility', 'display',
  'font-family', 'font-size', 'font-weight', 'font-style', 'font-stretch',
  'font-variant', 'font-kerning', 'font-feature-settings', 'font-variation-settings',
  'letter-spacing', 'word-spacing', 'text-decoration', 'text-anchor',
  'dominant-baseline', 'alignment-baseline', 'baseline-shift', 'white-space',
  'paint-order', 'vector-effect', 'shape-rendering', 'text-rendering', 'image-rendering',
  'marker-start', 'marker-mid', 'marker-end', 'clip-path', 'mask', 'filter',
  'transform', 'transform-origin', 'transform-box',
  'stop-color', 'stop-opacity', 'flood-color', 'flood-opacity', 'color-interpolation',
  'color-interpolation-filters', 'mix-blend-mode', 'isolation',
] as const

interface SvgSnapshot { source: string; width: number; height: number }

function localReferences(value: string): string {
  // Browsers expand marker/gradient URLs to the page URL in computed styles.
  // Every reference to a definition inside this SVG must remain local in its file.
  return value.replace(/url\(\s*["']?[^"'()]*#([^"'()]+)["']?\s*\)/g, 'url(#$1)')
}

export async function snapshot(svg: SVGSVGElement): Promise<SvgSnapshot> {
  const doc = svg.ownerDocument
  const view = doc.defaultView
  if (!view) throw new Error('无法读取图表的显示样式。')
  if (doc.fonts) await doc.fonts.ready

  const box = svg.viewBox.baseVal
  const width = box.width || svg.width.baseVal.value || svg.getBoundingClientRect().width
  const height = box.height || svg.height.baseVal.value || svg.getBoundingClientRect().height
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    throw new Error('图表尺寸无效，无法导出。')
  }
  const x = box.width > 0 ? box.x : 0
  const y = box.height > 0 ? box.y : 0
  const clone = svg.cloneNode(true) as SVGSVGElement
  const originals = [svg, ...svg.querySelectorAll<SVGElement>('*')]
  const copies = [clone, ...clone.querySelectorAll<SVGElement>('*')]

  originals.forEach((original, index) => {
    const copy = copies[index]!
    const computed = view.getComputedStyle(original)
    for (const property of INLINE_PROPERTIES) {
      const value = computed.getPropertyValue(property)
      if (value) copy.style.setProperty(property, localReferences(value))
    }
    // Preserve any local URLs authored as attributes as well as CSS URLs.
    for (const attribute of [...copy.attributes]) {
      if (attribute.value.includes('url(')) copy.setAttribute(attribute.name, localReferences(attribute.value))
      if (attribute.value.includes('var(') && (INLINE_PROPERTIES as readonly string[]).includes(attribute.name)) {
        const resolved = computed.getPropertyValue(attribute.name)
        if (resolved) copy.setAttribute(attribute.name, localReferences(resolved))
      }
      if (/^on/i.test(attribute.name)) copy.removeAttribute(attribute.name)
    }
    copy.style.setProperty('animation', 'none')
    copy.style.setProperty('transition', 'none')
  })

  clone.setAttribute('xmlns', SVG_NAMESPACE)
  clone.setAttribute('width', String(width))
  clone.setAttribute('height', String(height))
  clone.setAttribute('viewBox', `${x} ${y} ${width} ${height}`)
  clone.setAttribute('preserveAspectRatio', 'xMidYMid meet')
  clone.style.setProperty('width', `${width}px`)
  clone.style.setProperty('height', `${height}px`)
  clone.style.setProperty('min-width', '0')
  clone.style.setProperty('min-height', '0')
  clone.style.setProperty('max-width', 'none')
  clone.style.setProperty('max-height', 'none')
  clone.style.setProperty('transform', 'none')
  clone.style.setProperty('transform-origin', '0 0')
  clone.style.setProperty('overflow', 'hidden')

  // A solid first rectangle makes every diagram export the active canvas theme,
  // including diagrams whose screen background belongs to their parent panel.
  const canvas = view.getComputedStyle(svg).getPropertyValue('--dg-canvas').trim() || '#ffffff'
  const background = doc.createElementNS(SVG_NAMESPACE, 'rect')
  background.setAttribute('x', String(x))
  background.setAttribute('y', String(y))
  background.setAttribute('width', String(width))
  background.setAttribute('height', String(height))
  background.setAttribute('fill', canvas)
  background.setAttribute('data-export-background', '')
  background.style.setProperty('fill', canvas)
  background.style.setProperty('stroke', 'none')
  clone.insertBefore(background, clone.firstChild)

  // Computed presentation styles are now self contained. Stylesheets and scripts
  // are unnecessary in a native SVG artifact and can restart screen animations.
  clone.querySelectorAll('style, script').forEach(element => element.remove())
  const source = `<?xml version="1.0" encoding="UTF-8"?>\n${new XMLSerializer().serializeToString(clone)}`
  return { source, width, height }
}

function normalizedFilename(filename: string, extension: 'svg' | 'png'): string {
  const name = filename.replace(/[\\/:*?"<>|\u0000-\u001f]/g, '-').trim() || 'diagram'
  return `${name.replace(/\.(?:svg|png)$/i, '')}.${extension}`
}

function download(blob: Blob, filename: string, doc: Document): void {
  const url = URL.createObjectURL(blob)
  const link = doc.createElement('a')
  link.href = url
  link.download = filename
  link.style.display = 'none'
  doc.body.appendChild(link)
  link.click()
  link.remove()
  // Keep the URL alive until the browser's asynchronous download has started.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Export the full native SVG, independent of the workbench's viewport transform. */
export async function exportDiagramSvg(svg: SVGSVGElement, filename: string): Promise<void> {
  const result = await snapshot(svg)
  download(new Blob([result.source], { type: 'image/svg+xml;charset=utf-8' }), normalizedFilename(filename, 'svg'), svg.ownerDocument)
}

/** Render the full diagram at 2× resolution, capped at 8192 pixels on either side. */
export async function exportDiagramPng(svg: SVGSVGElement, filename: string): Promise<void> {
  const result = await snapshot(svg)
  const ratio = Math.min(2, 8192 / result.width, 8192 / result.height)
  const canvas = svg.ownerDocument.createElement('canvas')
  canvas.width = Math.max(1, Math.round(result.width * ratio))
  canvas.height = Math.max(1, Math.round(result.height * ratio))
  const context = canvas.getContext('2d')
  if (!context) throw new Error('当前浏览器无法创建图片画布。')
  const url = URL.createObjectURL(new Blob([result.source], { type: 'image/svg+xml;charset=utf-8' }))
  try {
    const image = new Image()
    image.decoding = 'async'
    await new Promise<void>((resolve, reject) => {
      image.onload = () => resolve()
      image.onerror = () => reject(new Error('图表图片解码失败，请重试。'))
      image.src = url
    })
    context.drawImage(image, 0, 0, canvas.width, canvas.height)
    const png = await new Promise<Blob>((resolve, reject) => canvas.toBlob(blob => blob ? resolve(blob) : reject(new Error('PNG 图片生成失败。')), 'image/png'))
    download(png, normalizedFilename(filename, 'png'), svg.ownerDocument)
  } finally {
    URL.revokeObjectURL(url)
  }
}
