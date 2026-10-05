<script setup lang="ts">
import Workspace from './v2/Workspace.vue'
import { defineAsyncComponent } from 'vue'
const ReviewWorkbench = defineAsyncComponent(() => import('./review/ReviewWorkbench.vue'))
const reviewing = new URLSearchParams(window.location.search).has('review')
</script>

<template>
  <ReviewWorkbench v-if="reviewing" />
  <Workspace v-else />
</template>
