<script setup lang="ts">
// 工具详情（需求 5.2）：输入区、输出区、复制、清空、示例、说明
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import FormField from '@/components/common/FormField.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import { getTool } from '@/api/public'
import { ApiError } from '@/types/api'
import type { Tool } from '@/types/tool'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { TOOL_CATEGORIES } from '@/types/common'
import { applyPageMeta } from '@/utils/seo'

const route = useRoute()
const recent = useRecentToolsStore()

const slug = computed(() => {
  const s = (route.params.slug as string) || ''
  document.title = `MC 工具站 · ${s}`
  return s
})

const tool = ref<Tool | null>(null)
const loading = ref(true)
const error = ref('')

const input = ref<Record<string, string>>({})
const output = ref('')

async function loadTool() {
  loading.value = true
  error.value = ''
  tool.value = null
  input.value = {}
  output.value = ''
  try {
    tool.value = await getTool(slug.value)
    if (tool.value) {
      recent.record(tool.value.slug, tool.value.name, tool.value.category)
      applyPageMeta({
        title: tool.value.name,
        description: tool.value.summary || `${tool.value.name} - MC 工具站在线工具`,
        canonicalPath: `/tools/${tool.value.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '工具加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(loadTool)
watch(slug, loadTool)

const categoryLabel = computed(() => {
  if (!tool.value) return ''
  return TOOL_CATEGORIES.find((c) => c === tool.value?.category) ?? tool.value?.category ?? ''
})

function fillExample() {
  if (!tool.value) return
  // 将 input_schema 的 placeholder 作为示例值；或填入字段默认示例
  input.value = {}
  for (const field of tool.value.input_schema ?? []) {
    input.value[field.key] = field.placeholder ?? ''
  }
}

function clearAll() {
  input.value = {}
  output.value = ''
}

// 本地工具计算注册表（需求 5.2：工具无需服务端计算时尽量在浏览器本地运行）。
// 内置核心工具（坐标换算/配方/物品 ID/指令）已在路由层重定向到专用页
// （见 utils/builtin-tools.ts），此处仅服务后台维护的其他工具。
// 如需新增本地计算工具，在此按 slug 注册纯函数并返回结果字符串。
function runLocal(slug: string): string {
  const calculators: Record<string, () => string> = {
    // 示例：'sample': () => '本地计算结果'
  }
  return calculators[slug]?.() ?? ''
}

function compute() {
  if (!tool.value) return
  const local = runLocal(slug.value)
  if (local !== '') {
    output.value = local
    return
  }
  // 无本地计算逻辑（需要服务端或复杂逻辑）时，提示用户
  output.value = ''
}
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: tool?.name ?? slug }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadTool" />

    <template v-else-if="tool">
      <div class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-3">
          <h1 class="text-2xl font-bold text-text-primary md:text-3xl">{{ tool.name }}</h1>
          <FavoriteButton kind="tool" :slug="tool.slug" />
        </div>
        <p class="mb-3 text-text-secondary">{{ tool.summary }}</p>
        <div class="flex flex-wrap gap-2">
          <span class="rounded-full border border-border-light bg-ink-700 px-2.5 py-1 text-xs text-text-secondary">
            {{ categoryLabel }}
          </span>
          <span
            v-for="tag in tool.tags"
            :key="tag"
            class="rounded-full border border-border-dark bg-ink-800 px-2.5 py-1 text-xs text-text-muted"
          >
            #{{ tag }}
          </span>
          <span
            v-if="tool.supported_versions.length"
            class="rounded-full border border-border-light bg-ink-700 px-2.5 py-1 text-xs text-text-secondary"
          >
            支持版本：{{ tool.supported_versions.join('、') }}
          </span>
        </div>
      </div>

      <!-- 输入输出工作区 -->
      <div class="grid gap-6 lg:grid-cols-2">
        <section class="card p-6" aria-labelledby="input-title">
          <div class="mb-4 flex items-center justify-between">
            <h2 id="input-title" class="font-semibold text-text-primary">输入</h2>
            <div class="flex gap-2">
              <button
                type="button"
                class="text-xs text-text-muted transition hover:text-text-primary"
                @click="fillExample"
              >
                填入示例
              </button>
              <button
                type="button"
                class="text-xs text-text-muted transition hover:text-text-primary"
                @click="clearAll"
              >
                清空
              </button>
            </div>
          </div>

          <div v-if="(tool.input_schema ?? []).length" class="space-y-4">
            <FormField
              v-for="field in tool.input_schema"
              :key="field.key"
              :label="field.label"
              :required="field.required"
              :hint="field.hint"
            >
              <template #default="{ id }">
                <input
                  :id="id"
                  v-model="input[field.key]"
                  :type="field.type === 'number' ? 'number' : 'text'"
                  :placeholder="field.placeholder"
                  class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
              </template>
            </FormField>
          </div>
          <p v-else class="text-sm text-text-muted">该工具无需输入，点击右侧“生成/计算”开始。</p>

          <button
            type="button"
            class="mt-5 w-full rounded-md bg-grass-600 py-3 font-medium text-white transition hover:bg-grass-500"
            @click="compute"
          >
            生成 / 计算
          </button>
        </section>

        <section class="card p-6" aria-labelledby="output-title">
          <div class="mb-4 flex items-center justify-between">
            <h2 id="output-title" class="font-semibold text-text-primary">输出</h2>
            <CopyButton v-if="output" :text="output" />
          </div>

          <div v-if="output" class="code-block whitespace-pre-wrap p-4 text-grass-300">
            {{ output }}
          </div>
          <div v-else class="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-border-light text-sm text-text-muted">
            输出结果将显示在这里
          </div>
        </section>
      </div>

      <!-- 说明 -->
      <section v-if="tool.description" class="card mt-6 p-6">
        <h2 class="mb-3 font-semibold text-text-primary">工具说明</h2>
        <div class="whitespace-pre-line text-sm leading-relaxed text-text-secondary">
          {{ tool.description }}
        </div>
      </section>
    </template>
  </PageContainer>
</template>