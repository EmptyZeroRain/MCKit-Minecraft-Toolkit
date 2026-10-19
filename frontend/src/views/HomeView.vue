<script setup lang="ts">
// 首页（需求 1.2 第 1 条）：全站搜索、快捷工具、资源分类、最近更新
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import GlobalSearch from '@/components/search/GlobalSearch.vue'
import ToolCard from '@/components/cards/ToolCard.vue'
import BuiltinToolCard from '@/components/cards/BuiltinToolCard.vue'
import ResourceCard from '@/components/cards/ResourceCard.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { listTools, listResources } from '@/api/public'
import { BUILTIN_TOOLS } from '@/utils/builtin-tools'
import type { Tool } from '@/types/tool'
import type { Resource } from '@/types/resource'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: 'MC 工具站',
  description:
    'Minecraft Java 版工具与资源导航站：合成配方查询、坐标换算、物品 ID 查询、指令格式辅助，以及 Mod / 整合包 / 资源包等资源导航。',
  canonicalPath: '/',
})

const featuredTools = ref<Tool[]>([])
const hotResources = ref<Resource[]>([])
const loading = ref(true)
const error = ref('')

async function loadHome() {
  loading.value = true
  error.value = ''
  try {
    const [toolsRes, resourcesRes] = await Promise.all([
      listTools({ page: 1, page_size: 4 }),
      listResources({ q: '', page: 1, page_size: 4 }),
    ])
    featuredTools.value = toolsRes.items.filter((t) => t.is_featured).slice(0, 4)
    if (!featuredTools.value.length) featuredTools.value = toolsRes.items.slice(0, 4)
    hotResources.value = resourcesRes.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(loadHome)

const resourceTypes = [
  { type: 'mod', label: 'Mod', to: '/resources?type=mod' },
  { type: 'modpack', label: '整合包', to: '/resources?type=modpack' },
  { type: 'resource_pack', label: '资源包', to: '/resources?type=resource_pack' },
  { type: 'shader', label: '光影', to: '/resources?type=shader' },
  { type: 'world', label: '存档', to: '/resources?type=world' },
  { type: 'datapack', label: '数据包', to: '/resources?type=datapack' },
  { type: 'plugin', label: '插件', to: '/resources?type=plugin' },
]

const recentTools = useRecentToolsStore()
function clickTool(t: Tool) {
  recentTools.record(t.slug, t.name, t.category)
}
</script>

<template>
  <div>
    <!-- Hero：全站搜索 -->
    <section class="relative overflow-hidden border-b border-border-dark bg-ink-900/60">
      <div class="mc-grid-bg absolute inset-0" aria-hidden="true" />
      <PageContainer class="relative">
        <div class="py-14 text-center md:py-20">
          <div class="mb-4 inline-flex items-center gap-2 rounded-full border border-grass-500/30 bg-grass-500/10 px-4 py-1.5 text-xs text-grass-300">
            <span class="inline-block h-1.5 w-1.5 rounded-full bg-grass-400" />
            Minecraft Java 版玩家工具与资源导航
          </div>
          <h1 class="mb-6 text-3xl font-bold leading-tight text-text-primary md:text-5xl">
            一站式 <span class="text-grass-400">MC</span> 工具与资源导航
          </h1>
          <p class="mx-auto mb-8 max-w-2xl text-text-secondary md:text-lg">
            合成配方查询、坐标换算、物品 ID、指令格式辅助，以及 Mod / 整合包 / 资源包等资源导航。
          </p>
          <div class="mx-auto max-w-2xl">
            <GlobalSearch />
          </div>
          <div class="mt-6 flex flex-wrap justify-center gap-2 text-xs text-text-muted">
            <span>热门：</span>
            <RouterLink
              v-for="kw in ['钻石剑', '坐标换算', 'Fabric', '光影']"
              :key="kw"
              :to="{ path: '/search', query: { q: kw } }"
              class="rounded-full border border-border-dark px-2.5 py-1 text-text-secondary transition hover:border-grass-500/60 hover:text-grass-300"
            >
              {{ kw }}
            </RouterLink>
          </div>
        </div>
      </PageContainer>
    </section>

    <!-- 快捷工具 -->
    <PageContainer>
      <section class="py-8">
        <div class="mb-5 flex items-center justify-between">
          <h2 class="text-xl font-bold text-text-primary">快捷工具</h2>
          <RouterLink to="/tools" class="text-sm text-grass-400 hover:underline">全部工具 →</RouterLink>
        </div>
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <BuiltinToolCard v-for="tool in BUILTIN_TOOLS" :key="tool.slug" :tool="tool" />
        </div>
      </section>

      <section class="flex flex-col gap-8 py-8 lg:flex-row">
        <!-- 推荐工具 -->
        <div class="flex-1">
          <div class="mb-5 flex items-center justify-between">
            <h2 class="text-xl font-bold text-text-primary">推荐工具</h2>
            <RouterLink to="/tools" class="text-sm text-grass-400 hover:underline">查看全部</RouterLink>
          </div>
          <LoadingSkeleton v-if="loading" :count="2" />
          <ErrorState v-else-if="error" :message="error" retryable @retry="loadHome" />
          <div v-else-if="featuredTools.length" class="grid gap-4 sm:grid-cols-2">
            <ToolCard
              v-for="tool in featuredTools"
              :key="tool.id"
              :tool="tool"
              @click="clickTool(tool)"
            />
          </div>
          <div v-else class="rounded-card border border-dashed border-border-light p-8 text-center text-text-muted">
            暂无推荐工具
          </div>
        </div>

        <!-- 资源分类 -->
        <aside class="w-full lg:w-72">
          <h2 class="mb-5 text-xl font-bold text-text-primary">资源分类</h2>
          <div class="grid grid-cols-2 gap-2">
            <RouterLink
              v-for="rt in resourceTypes"
              :key="rt.type"
              :to="rt.to"
              class="card card-hover flex items-center justify-between px-4 py-3"
            >
              <span class="text-sm text-text-primary">{{ rt.label }}</span>
              <span class="text-text-muted">→</span>
            </RouterLink>
          </div>
        </aside>
      </section>

      <!-- 最近更新资源 -->
      <section class="py-8">
        <div class="mb-5 flex items-center justify-between">
          <h2 class="text-xl font-bold text-text-primary">最近更新</h2>
          <RouterLink to="/resources" class="text-sm text-grass-400 hover:underline">全部资源 →</RouterLink>
        </div>
        <LoadingSkeleton v-if="loading" :count="4" />
        <ErrorState v-else-if="error" :message="error" retryable @retry="loadHome" />
        <div v-else-if="hotResources.length" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <ResourceCard v-for="res in hotResources" :key="res.id" :resource="res" />
        </div>
        <div v-else class="rounded-card border border-dashed border-border-light p-8 text-center text-text-muted">
          暂无资源
        </div>
      </section>
    </PageContainer>
  </div>
</template>