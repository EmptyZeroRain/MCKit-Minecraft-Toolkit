<script setup lang="ts">
// 安全 Markdown 渲染组件
// 安全策略：只解析 Markdown 语法子集，绝不渲染原始 HTML（防 XSS，需求 11.1）。
// URL 只允许 http/https，其余链接降级为纯文本。
import { computed } from 'vue'
import { renderSafeMarkdown } from '@/utils/markdown'

const props = defineProps<{
  content: string
}>()

const html = computed(() => renderSafeMarkdown(props.content ?? ''))
</script>

<template>
  <div class="markdown-body" v-html="html" />
</template>