<script setup lang="ts">defineProps<{ prefix: string }>()</script>
<template><defs>
  <marker :id="`${prefix}-arrow`" viewBox="0 0 12 12" refX="10" refY="6" markerWidth="7" markerHeight="7" orient="auto"><path d="M2 2 10 6 2 10Z" fill="var(--dg-accent, #6265e8)"/></marker>
  <marker :id="`${prefix}-open`" viewBox="0 0 12 12" refX="10" refY="6" markerWidth="8" markerHeight="8" orient="auto"><path d="M2 2 10 6 2 10" fill="none" stroke="var(--dg-accent, #6265e8)" stroke-width="1.4"/></marker>
  <marker :id="`${prefix}-triangle`" viewBox="0 0 16 16" refX="14" refY="8" markerWidth="12" markerHeight="12" orient="auto"><path d="M2 2 14 8 2 14Z" fill="var(--dg-canvas, white)" stroke="var(--dg-accent, #6265e8)" stroke-width="1.4"/></marker>
  <marker :id="`${prefix}-diamond`" viewBox="0 0 20 12" refX="2" refY="6" markerWidth="15" markerHeight="10" orient="auto"><path d="M2 6 10 1 18 6 10 11Z" fill="var(--dg-accent, #6265e8)"/></marker>
  <marker :id="`${prefix}-empty-diamond`" viewBox="0 0 20 12" refX="2" refY="6" markerWidth="15" markerHeight="10" orient="auto"><path d="M2 6 10 1 18 6 10 11Z" fill="var(--dg-canvas, white)" stroke="var(--dg-accent, #6265e8)" stroke-width="1.3"/></marker>
</defs></template>
