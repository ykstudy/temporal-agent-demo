<script setup lang="ts">
import RelationCanvas from './RelationCanvas.vue'
import type {RelationshipData,DiagramSelection} from '../../../lib/diagram-types'
defineProps<{data:RelationshipData;selectedId?:string}>();const emit=defineEmits<{select:[value:DiagramSelection]}>()
</script>
<template><RelationCanvas :data="data" :selected-id="selectedId" mode="knowledge" @select="emit('select',$event)"/></template>
