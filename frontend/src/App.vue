<script setup lang="ts">
// 根组件：仅承载全局 Toast 出口与路由视图
import ToastHost from '@/components/common/Toast.vue'
</script>

<template>
  <RouterView />
  <ToastHost />
</template>