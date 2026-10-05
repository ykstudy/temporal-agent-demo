<script setup lang="ts">
import type { CoverageData, CoverageCell } from './types'
import type { DiagramSelection } from './diagrams/lib/diagram-types'
const props = defineProps<{ data: CoverageData; selectedId?: string }>()
const emit = defineEmits<{ select: [selection: DiagramSelection] }>()
const colors: Record<string, string> = { docs_ready: '#edeaff', planned: '#f1f3f8', not_run: '#fff3de', historical_path_verified: '#e5f1f7', historical_docs_verified: '#e5f1f7', not_applicable: '#f5f5f5' }
const width = 450 + props.data.columns.length * 178
const cell = (row: string, column: string) => props.data.cells.find(item => item.row === row && item.column === column)!
function select(item: CoverageCell) { emit('select', { id: item.id, label: item.label, type: '验收状态', description: item.detail, details: [item.status, ...item.case_ids, ...item.evidence_ids] }) }
</script>
<template>
  <svg class="dg-svg coverage-svg" data-diagram="coverage" role="img" :aria-label="data.title" :viewBox="`0 0 ${width} ${180 + data.rows.length * 74}`">
    <title>{{ data.title }}</title><desc>{{ data.description }}</desc>
    <text x="36" y="44" class="matrix-heading">目标差异 × 验证层</text>
    <text x="36" y="73" class="matrix-note">离散状态 · 历史路径证据不关闭目标契约缺口</text>
    <text v-for="(column,index) in data.columns" :key="column.id" :x="490 + index*178" y="118" text-anchor="middle" class="matrix-column">{{ column.label }}</text>
    <g v-for="(row,index) in data.rows" :key="row.id">
      <rect x="24" :y="140+index*74" :width="width-48" height="70" :fill="index%2 ? 'var(--dg-background)' : 'var(--dg-canvas)'"/>
      <text x="38" :y="166+index*74" class="matrix-row">{{ row.id }} · {{ row.label.slice(0,25) }}</text>
      <text x="38" :y="190+index*74" class="matrix-note">{{ row.priority }} · {{ row.case_ids.join(' / ') || '按阶段登记' }}</text>
      <g v-for="(column,c) in data.columns" :key="column.id" role="button" tabindex="0" :aria-label="`${row.id} ${column.label} ${cell(row.id,column.id).label}`" :data-node-id="cell(row.id,column.id).id" @click="select(cell(row.id,column.id))" @keydown.enter="select(cell(row.id,column.id))" @keydown.space.prevent="select(cell(row.id,column.id))">
        <rect :x="409+c*178" :y="153+index*74" width="164" height="43" rx="7" :fill="colors[cell(row.id,column.id).status] || '#f1f3f8'" :stroke="selectedId===cell(row.id,column.id).id ? 'var(--dg-accent)' : 'var(--dg-border)'" :stroke-width="selectedId===cell(row.id,column.id).id ? 2 : 1"/>
        <text :x="491+c*178" :y="180+index*74" text-anchor="middle" class="matrix-status">{{ cell(row.id,column.id).label }}</text>
      </g>
    </g>
    <text x="36" :y="164+data.rows.length*74" class="matrix-note">每格可查看独立来源、适用案例与未覆盖范围。颜色不表示完成百分比。</text>
  </svg>
</template>
