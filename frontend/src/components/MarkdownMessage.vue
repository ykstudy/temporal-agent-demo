<script setup lang="ts">
import { computed } from 'vue';
import { renderMarkdown } from '../lib/markdown';

const props = defineProps<{ content: string }>();
const rendered = computed(() => renderMarkdown(props.content));
</script>

<template>
  <div class="markdown-message" v-html="rendered" />
</template>
