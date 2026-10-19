<script setup lang="ts">
// 资源类型徽章
import { RESOURCE_TYPE_LABELS, type ResourceType } from '@/types/common'

defineProps<{ type: ResourceType }>()

const styleMap: Record<ResourceType, string> = {
  mod: 'border-blue-400/40 bg-blue-400/10 text-blue-300',
  modpack: 'border-purple-400/40 bg-purple-400/10 text-purple-300',
  resource_pack: 'border-amber-mc/40 bg-amber-mc/10 text-amber-mc',
  shader: 'border-cyan-400/40 bg-cyan-400/10 text-cyan-300',
  world: 'border-grass-500/40 bg-grass-500/10 text-grass-300',
  datapack: 'border-emerald-400/40 bg-emerald-400/10 text-emerald-300',
  plugin: 'border-orange-400/40 bg-orange-400/10 text-orange-300',
}
</script>

<template>
  <span
    class="inline-flex items-center rounded border px-1.5 py-0.5 text-xs font-medium"
    :class="styleMap[type]"
  >
    {{ RESOURCE_TYPE_LABELS[type] ?? type }}
  </span>
</template>