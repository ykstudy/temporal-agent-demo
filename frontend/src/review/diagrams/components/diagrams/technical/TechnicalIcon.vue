<script setup lang="ts">
defineProps<{ name?: string; size?: number }>()
</script>
<template>
  <svg :width="size || 26" :height="size || 26" viewBox="0 0 28 28" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <g v-if="name === 'database'"><ellipse cx="14" cy="6" rx="9" ry="3.5"/><path d="M5 6v15c0 4.7 18 4.7 18 0V6M5 13c0 4.7 18 4.7 18 0M5 20c0 4.7 18 4.7 18 0"/></g>
    <g v-else-if="name === 'server'"><rect x="4" y="3" width="20" height="9" rx="2"/><rect x="4" y="16" width="20" height="9" rx="2"/><path d="M9 7.5h.1M9 20.5h.1M14 7.5h6M14 20.5h6"/></g>
    <g v-else-if="name === 'cloud'"><path d="M8 22a6 6 0 0 1-1-11.9A8 8 0 0 1 22 9a6.5 6.5 0 0 1-1 13H8Z"/></g>
    <g v-else-if="name === 'router'"><rect x="3" y="13" width="22" height="11" rx="2"/><path d="M8 13V6M20 13V6M6 5a3 3 0 0 1 4 0M18 5a3 3 0 0 1 4 0M8 19h.1M12 19h.1M17 19h4"/></g>
    <g v-else-if="name === 'switch'"><rect x="3" y="8" width="22" height="15" rx="2"/><path d="M7 13h3v5H7zM13 13h3v5h-3zM19 13h3v5h-3zM7 5h14M7 26h14"/></g>
    <g v-else-if="name === 'firewall'"><path d="m14 3 10 4v7c0 6-6 10-10 12C10 24 4 20 4 14V7l10-4Z"/><path d="M8 10h12M7 15h14M10 20h8M12 10v5M16 15v5"/></g>
    <g v-else-if="name === 'person'"><circle cx="14" cy="8" r="4"/><path d="M5 25v-3a9 9 0 0 1 18 0v3"/></g>
    <g v-else-if="name === 'workstation'"><rect x="3" y="4" width="22" height="16" rx="2"/><path d="M10 25h8M14 20v5M6 16h16"/></g>
    <g v-else-if="name === 'wallet'"><path d="M23 10V6a2 2 0 0 0-2-2H7a4 4 0 0 0-4 4v13a3 3 0 0 0 3 3h17V10H7a3 3 0 0 1-3-3"/><path d="M19 13h6v7h-6a3.5 3.5 0 0 1 0-7ZM20 16.5h.1"/></g>
    <g v-else-if="name === 'package'"><path d="m14 3 11 6v12l-11 6L3 21V9l11-6ZM3 9l11 6 11-6M14 15v12M8 6l11 6"/></g>
    <g v-else><path d="m14 3 11 6-11 6L3 9l11-6ZM3 15l11 6 11-6M3 21l11 6 11-6"/></g>
  </svg>
</template>
