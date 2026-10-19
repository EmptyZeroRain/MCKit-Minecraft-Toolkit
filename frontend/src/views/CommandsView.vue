<script setup lang="ts">
// 指令格式辅助（需求 5.6）
import { onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listCommands } from '@/api/public'
import type { Command } from '@/types/command'
import type { CommandExample } from '@/types/command'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '指令格式辅助',
  description: 'Minecraft 命令语法、参数说明与示例模板，支持一键复制。',
  canonicalPath: '/commands',
})

// 记录最近使用
useRecentToolsStore().record('commands', '指令格式辅助', '辅助类')

const route = useRoute()
const router = useRouter()

const query = reactive({
  q: (route.query.q as string) || '',
  page: Number(route.query.page) || 1,
  pageSize: 10,
})

const commands = ref<Command[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

async function loadCommands() {
  loading.value = true
  error.value = ''
  try {
    const res = await listCommands({ q: query.q || undefined, page: query.page, page_size: query.pageSize })
    commands.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / query.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (query.q) params.q = query.q
  if (query.page > 1) params.page = String(query.page)
  router.replace({ path: '/commands', query: params })
}

watch(
  () => query.q,
  () => {
    query.page = 1
    syncUrl()
    loadCommands()
  },
)

function onPageChange(p: number) {
  query.page = p
  syncUrl()
  loadCommands()
}

function clearSearch() {
  query.q = ''
}

onMounted(loadCommands)

function formatExample(example: CommandExample): string {
  return example.command ?? ''
}
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '指令格式辅助' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">指令格式辅助</h1>
        <p class="text-text-secondary">
          按命令名称、用途检索指令，查看语法、参数说明与示例，一键复制命令模板。
          本页不自动执行任何命令，也不连接你的服务器。
        </p>

      </div>
      <ShareButton />
    </div>

    <!-- 搜索 -->
    <div class="mb-6 relative max-w-xl">
      <svg
        class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path stroke-linecap="round" d="M21 21l-4.35-4.35M17 10.5a6.5 6.5 0 11-13 0 6.5 6.5 0 0113 0z" />
      </svg>
      <input
        v-model="query.q"
        type="search"
        aria-label="搜索指令"
        placeholder="例如：give / tp / 传送 / 给予物品…"
        class="h-11 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-9 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
      />
      <button
        v-if="query.q"
        class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
        aria-label="清除"
        @click="clearSearch"
      >
        ✕
      </button>
    </div>

    <LoadingSkeleton v-if="loading" :count="4" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadCommands" />

    <div v-else-if="commands.length" class="space-y-4">
      <article v-for="cmd in commands" :key="cmd.id" class="card p-5">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <h2 class="font-mono text-lg font-semibold text-grass-300">/{{ cmd.name }}</h2>
            <span
              v-for="v in cmd.game_versions ?? []"
              :key="v"
              class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted"
            >
              {{ v }}
            </span>
          </div>
          <CopyButton tool="commands" :text="`/${cmd.syntax || cmd.name}`" label="复制模板" />
        </div>

        <p class="mb-3 text-sm text-text-secondary">{{ cmd.description }}</p>

        <!-- 语法 -->
        <div class="code-block mb-3 whitespace-pre-wrap p-3 font-mono text-sm text-text-primary">
          /{{ cmd.syntax }}
        </div>

        <!-- 参数 -->
        <div v-if="(cmd.parameters ?? []).length" class="mb-4">
          <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">参数说明</h3>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b border-border-dark text-xs text-text-muted">
                  <th scope="col" class="px-2 py-1.5">参数</th>
                  <th scope="col" class="px-2 py-1.5">类型</th>
                  <th scope="col" class="px-2 py-1.5">说明</th>
                  <th scope="col" class="px-2 py-1.5">必填</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="p in cmd.parameters" :key="p.name" class="border-b border-border-dark/60">
                  <td class="px-2 py-1.5 font-mono text-grass-300">{{ p.name }}</td>
                  <td class="px-2 py-1.5 text-text-secondary">{{ p.type }}</td>
                  <td class="px-2 py-1.5 text-text-secondary">{{ p.description }}</td>
                  <td class="px-2 py-1.5 text-text-muted">{{ p.required ? '是' : '否' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- 示例 -->
        <div v-if="(cmd.examples ?? []).length">
          <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">示例</h3>
          <div class="space-y-2">
            <div
              v-for="(ex, i) in cmd.examples"
              :key="i"
              class="flex items-start justify-between gap-3"
            >
              <code class="code-block min-w-0 flex-1 px-3 py-2 leading-relaxed text-text-primary">
                /{{ formatExample(ex) }}
              </code>
              <CopyButton tool="commands" :text="`/${ex.command}`" label="复制" />
            </div>
          </div>
        </div>

        <p v-if="cmd.permission_notes" class="mt-3 text-xs text-text-muted">
          权限/环境：{{ cmd.permission_notes }}
        </p>
      </article>
    </div>

    <EmptyState
      v-else
      title="未找到匹配的指令"
      description="试试其他关键词，或检查指令名称拼写。版本差异未确认时将标记“待核实”。"
    />

    <div class="mt-8">
      <Pagination
        :page="query.page"
        :page-size="query.pageSize"
        :total="total"
        :total-pages="totalPages"
        @change="onPageChange"
      />
    </div>
  </PageContainer>
</template>