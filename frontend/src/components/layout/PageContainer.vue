<script setup lang="ts">
// 页面内容容器：统一居中宽度
withDefaults(
  defineProps<{
    /** 最大宽度类 */
    maxWidth?: string
    /** 是否带顶部内边距 */
    padded?: boolean
  }>(),
  {
    maxWidth: 'max-w-7xl',
    padded: true,
  },
)
</script>

<template>
  <div class="mx-auto w-full px-4 sm:px-6" :class="[maxWidth, padded ? 'py-6 sm:py-8' : '']">
    <slot />
  </div>
</template>