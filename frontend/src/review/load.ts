import Ajv2020 from 'ajv/dist/2020'
import addFormats from 'ajv-formats'
import type { ReviewBundle } from './types'
import publicationSchema from '../../../design/review/review-bundle.schema.json'
import productSource from '../../../docs/PRODUCT-GUIDE.md?raw'
import invariantSource from '../../../design/00-invariants/INVARIANTS.md?raw'
import publication from './publication.generated.json'

export async function sha256(text: string): Promise<string> {
  const bytes = new TextEncoder().encode(text)
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
}

export async function loadReview(): Promise<ReviewBundle> {
  const [schemaResponse, dataResponse] = await Promise.all([
    fetch('./review/schema.json', { cache: 'no-cache' }), fetch('./review/bundle.json', { cache: 'no-cache' }),
  ])
  if (!schemaResponse.ok || !dataResponse.ok) throw new Error('审查发布文件不可读取，请先生成文档模型。')
  const text = await dataResponse.text()
  if (new TextEncoder().encode(text).length > 12_000_000) throw new Error('审查发布文件超过 12 MB 上限。')
  if (await sha256(text) !== publication.bundle_sha256) throw new Error('审查发布摘要不一致，请生成数据后重新构建前端。')
  const schemaText = await schemaResponse.text()
  if (await sha256(schemaText) !== publication.schema_sha256) throw new Error('发布 Schema 摘要与当前构建不一致。')
  if (JSON.stringify(JSON.parse(schemaText)) !== JSON.stringify(publicationSchema)) throw new Error('发布 Schema 与当前构建规范不一致。')
  const value: unknown = JSON.parse(text)
  const validator = new Ajv2020({ strict: false, allErrors: true, validateFormats: true })
  addFormats(validator)
  const validate = validator.compile<ReviewBundle>(publicationSchema)
  if (!validate(value)) throw new Error(`审查数据不符合发布结构：${validator.errorsText(validate.errors, { separator: '；' })}`)
  const sources = new Map(value.sources.map(source => [source.id, source]))
  if (sources.size !== value.sources.length || new Set(value.models.map(model => model.id)).size !== value.models.length) throw new Error('审查身份重复。')
  for (const source of value.sources) {
    if (source.path.split('/').includes('..') || source.path.startsWith('/')) throw new Error('来源路径越界。')
    if (await sha256(source.text) !== source.sha256) throw new Error(`来源摘要不一致：${source.path}`)
  }
  const paths = new Map(value.sources.map(source => [source.path, source]))
  if (paths.size !== value.sources.length) throw new Error('来源路径重复。')
  const productDigest = await sha256(productSource), invariantDigest = await sha256(invariantSource)
  if (value.baseline.source_sha256 !== productDigest || value.baseline.invariants_sha256 !== invariantDigest
    || paths.get('docs/PRODUCT-GUIDE.md')?.sha256 !== productDigest
    || paths.get('design/00-invariants/INVARIANTS.md')?.sha256 !== invariantDigest) throw new Error('发布基线与当前冻结来源不一致。')
  if (paths.get('design/review/review-bundle.schema.json')?.text !== schemaText) throw new Error('发布 Schema 与来源原文不一致。')
  for (const model of value.models) {
    for (const id of [...model.source_ids, ...Object.values(model.bindings).flatMap(binding => binding.source_ids)]) {
      if (!sources.has(id)) throw new Error(`来源引用不存在：${id}`)
    }
  }
  return value
}
