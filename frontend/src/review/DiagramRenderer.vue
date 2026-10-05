<script setup lang="ts">
import { computed, type Component } from 'vue'
import StateDiagram from './diagrams/components/diagrams/business/StateDiagram.vue'
import SwimlaneDiagram from './diagrams/components/diagrams/business/SwimlaneDiagram.vue'
import SequenceDiagram from './diagrams/components/diagrams/technical/SequenceDiagram.vue'
import ClassDiagram from './diagrams/components/diagrams/technical/ClassDiagram.vue'
import LineageDiagram from './diagrams/components/diagrams/technical/LineageDiagram.vue'
import ArchitectureDiagram from './diagrams/components/diagrams/technical/ArchitectureDiagram.vue'
import DependencyDiagram from './diagrams/components/diagrams/technical/DependencyDiagram.vue'
import DataFlowDiagram from './diagrams/components/diagrams/technical/DataFlowDiagram.vue'
import KnowledgeDiagram from './diagrams/components/diagrams/structure/KnowledgeDiagram.vue'
import TimelineDiagram from './diagrams/components/diagrams/structure/TimelineDiagram.vue'
import CoverageDiagram from './CoverageDiagram.vue'
import type { DiagramSelection } from './diagrams/lib/diagram-types'
const props = defineProps<{ type: string; data: object; selectedId?: string }>()
const emit = defineEmits<{ select: [selection: DiagramSelection] }>()
const registry: Record<string, Component> = { state: StateDiagram, swimlane: SwimlaneDiagram, sequence: SequenceDiagram, class: ClassDiagram, lineage: LineageDiagram, architecture: ArchitectureDiagram, dependency: DependencyDiagram, dataflow: DataFlowDiagram, knowledge: KnowledgeDiagram, timeline: TimelineDiagram, coverage: CoverageDiagram }
const component = computed(() => registry[props.type])
</script>
<template><component :is="component" v-if="component" :data="data" :selected-id="selectedId" @select="emit('select',$event)"/><p v-else role="alert">未支持的审查图类型。</p></template>
