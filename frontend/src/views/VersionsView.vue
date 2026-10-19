<script setup lang="ts">
// MC 版本信息站（移植自 CreeperCode）
// 提供各版本 jar / mappings 下载地址、启动命令与 Java 版本要求。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import { listVersions, getVersionDetail } from '@/api/versions'
import { ApiError } from '@/types/api'
import {
  VERSION_TYPES,
  VERSION_TYPE_LABELS,
  type VersionDetail,
  type VersionEntry,
  type VersionType,
} from '@/types/version'
import { formatSize, shortSha1, formatDate } from '@/utils/format'
import { argDesc, classifierLabel } from '@/utils/launch-args'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: 'MC 版本信息',
  description:
    'Minecraft 各版本客户端/服务端 jar、mappings 下载地址，服务端与客户端启动命令，以及 Java 版本要求。',
  canonicalPath: '/versions',
})

useRecentToolsStore().record('versions', 'MC 版本信息', '查询类')

const route = useRoute()
const router = useRouter()

const filters = reactive({
  type: ((route.query.type as string) as VersionType | 'all') || 'all',
  q: (route.query.q as string) || '',
})

const latest = ref<{ release: string; snapshot: string } | null>(null)
const versions = ref<VersionEntry[]>([])
const listLoading = ref(false)
const listError = ref('')

const selectedId = ref((route.query.version as string) || '')
const detail = ref<VersionDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')

const typeOptions: Array<{ value: VersionType | 'all'; label: string }> = [
  { value: 'all', label: '全部' },
  ...VERSION_TYPES.map((t) => ({ value: t, label: VERSION_TYPE_LABELS[t] })),
]

async function loadVersions() {
  listLoading.value = true
  listError.value = ''
  try {
    const res = await listVersions({ type: filters.type, q: filters.q || undefined })
    latest.value = res.latest
    versions.value = res.versions
    // 未选中时默认展示最新正式版
    if (!selectedId.value && res.latest?.release) {
      selectedId.value = res.latest.release
    }
  } catch (err) {
    listError.value = err instanceof ApiError ? err.message : '版本列表加载失败'
    versions.value = []
  } finally {
    listLoading.value = false
  }
}

async function loadDetail(id: string) {
  if (!id) return
  detailLoading.value = true
  detailError.value = ''
  detail.value = null
  try {
    detail.value = await getVersionDetail({ id })
  } catch (err) {
    detailError.value = err instanceof ApiError ? err.message : '版本详情加载失败'
  } finally {
    detailLoading.value = false
  }
}

function selectVersion(id: string) {
  if (id === selectedId.value) return
  selectedId.value = id
  syncUrl()
  loadDetail(id)
}

function syncUrl() {
  const query: Record<string, string> = {}
  if (filters.type !== 'all') query.type = filters.type
  if (filters.q) query.q = filters.q
  if (selectedId.value) query.version = selectedId.value
  router.replace({ path: '/versions', query })
}

watch(
  () => [filters.type, filters.q],
  () => {
    syncUrl()
    loadVersions()
  },
)

const argCount = computed(() => {
  const cl = detail.value?.clientLaunch
  if (!cl) return 0
  return (cl.defaultUserJVM?.length || 0) + (cl.jvmArgs?.length || 0) + (cl.gameArgs?.length || 0)
})

const librariesExpanded = ref(false)

onMounted(async () => {
  await loadVersions()
  if (selectedId.value) loadDetail(selectedId.value)
})
</script>

<template>
  <PageContainer>
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: 'MC 版本信息' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">MC 版本信息</h1>
        <p class="text-text-secondary">
          查询 Minecraft 各版本客户端/服务端 jar、mappings 下载地址，获取启动命令与 Java 版本要求。
        </p>
        <div v-if="latest" class="mt-3 flex flex-wrap gap-3 text-sm">
          <span class="text-text-muted">最新正式版：
            <button class="font-mono text-grass-400 hover:underline" @click="selectVersion(latest.release)">
              {{ latest.release }}
            </button>
          </span>
          <span class="text-text-muted">最新快照：
            <button class="font-mono text-grass-400 hover:underline" @click="selectVersion(latest.snapshot)">
              {{ latest.snapshot }}
            </button>
          </span>
        </div>

      </div>
      <ShareButton />
    </div>

    <div class="grid gap-6 lg:grid-cols-[320px_1fr]">
      <!-- 版本列表 -->
      <aside class="space-y-3">
        <div class="card p-3">
          <input
            v-model="filters.q"
            type="search"
            aria-label="搜索版本"
            placeholder="搜索版本号，如 1.20 / 25w"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
          <div class="mt-3 flex flex-wrap gap-1.5">
            <button
              v-for="opt in typeOptions"
              :key="opt.value"
              type="button"
              class="rounded-full border px-2.5 py-1 text-xs transition"
              :class="
                filters.type === opt.value
                  ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                  : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
              "
              @click="filters.type = opt.value"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <div class="card overflow-hidden">
          <div v-if="listLoading" role="status" aria-live="polite" class="p-4 text-sm text-text-muted">
            加载版本列表中…
          </div>
          <div v-else-if="listError" role="alert" class="p-4 text-sm text-danger-mc">{{ listError }}</div>
          <div v-else-if="!versions.length" class="p-4 text-sm text-text-muted">没有匹配的版本</div>
          <ul v-else class="max-h-[60vh] divide-y divide-border-dark overflow-y-auto">
            <li v-for="v in versions" :key="v.id">
              <button
                type="button"
                class="flex w-full items-center justify-between gap-2 px-4 py-2.5 text-left transition hover:bg-ink-700"
                :class="{ 'bg-ink-700': v.id === selectedId }"
                @click="selectVersion(v.id)"
              >
                <span class="font-mono text-sm text-text-primary">{{ v.id }}</span>
                <span
                  class="shrink-0 rounded-full border px-2 py-0.5 text-[10px]"
                  :class="{
                    'border-grass-500/40 text-grass-300': v.type === 'release',
                    'border-sky-400/40 text-sky-300': v.type === 'snapshot',
                    'border-amber-mc/40 text-amber-mc': v.type === 'old_beta' || v.type === 'old_alpha',
                  }"
                >
                  {{ VERSION_TYPE_LABELS[v.type] ?? v.type }}
                </span>
              </button>
            </li>
          </ul>
        </div>
      </aside>

      <!-- 版本详情 -->
      <section class="min-w-0">
        <LoadingSkeleton v-if="detailLoading" :count="3" />
        <ErrorState v-else-if="detailError" :message="detailError" retryable @retry="loadDetail(selectedId)" />
        <EmptyState
          v-else-if="!detail"
          title="请选择一个版本"
          description="从左侧列表选择版本，查看下载地址、启动命令与 Java 要求。"
        />

        <div v-else class="space-y-4">
          <!-- 头部 -->
          <div class="card p-5">
            <div class="mb-2 flex flex-wrap items-center gap-3">
              <h2 class="font-mono text-xl font-bold text-text-primary">{{ detail.id }}</h2>
              <span class="rounded-full border border-grass-500/40 bg-grass-500/10 px-2.5 py-0.5 text-xs text-grass-300">
                {{ VERSION_TYPE_LABELS[detail.type] ?? detail.type }}
              </span>
              <span class="text-sm text-text-muted">发布于 {{ formatDate(detail.releaseTime) }}</span>
            </div>
            <div class="text-sm text-text-secondary">
              Java 版本要求：
              <strong class="text-text-primary">Java {{ detail.java.majorVersion }}+</strong>
              <span v-if="detail.java.component" class="text-text-muted">（runtime: {{ detail.java.component }}）</span>
            </div>
            <ul v-if="detail.startupNotes?.length" class="mt-1 list-disc pl-5 text-xs text-text-muted">
              <li v-for="n in detail.startupNotes" :key="n">{{ n }}</li>
            </ul>
          </div>

          <!-- 服务端启动命令 -->
          <div v-if="detail.startupCommand" class="card p-5">
            <h3 class="mb-3 font-semibold text-text-primary">服务端启动命令</h3>
            <div class="flex flex-wrap items-center gap-3">
              <code class="code-block min-w-0 flex-1 px-3 py-2 text-text-primary">{{ detail.startupCommand }}</code>
              <CopyButton tool="versions" :text="detail.startupCommand" />
            </div>
            <p class="mt-2 text-xs text-text-muted">将下载的服务端 jar 重命名为 server.jar 放到启动目录。</p>
          </div>

          <!-- 客户端启动命令 -->
          <div v-if="detail.clientLaunch" class="card p-5">
            <h3 class="mb-3 font-semibold text-text-primary">客户端启动命令</h3>
            <div class="flex flex-wrap items-center gap-3">
              <code class="code-block min-w-0 flex-1 whitespace-pre-wrap px-3 py-2 text-text-primary">{{ detail.clientLaunch.command }}</code>
              <CopyButton tool="versions" :text="detail.clientLaunch.command" />
            </div>
            <p class="mt-2 text-xs text-text-muted">
              <code>${{ '{' }}...}</code> 占位符（用户名、资源目录、classpath 等）由启动器在运行时替换；
              带平台标注的参数未拼入上方命令，按需手动添加。
            </p>

            <details class="mt-3" @toggle="librariesExpanded = !librariesExpanded">
              <summary class="cursor-pointer text-sm text-grass-400">启动参数明细（{{ argCount }} 条）</summary>
              <div class="mt-3 space-y-3">
                <template v-for="group in [
                  { title: '默认 JVM 参数（官方启动器）', args: detail.clientLaunch.defaultUserJVM },
                  { title: 'JVM 参数', args: detail.clientLaunch.jvmArgs },
                  { title: '游戏参数', args: detail.clientLaunch.gameArgs },
                ]" :key="group.title">
                  <div v-if="group.args?.length">
                    <h4 class="mb-1 text-xs font-semibold uppercase tracking-wide text-text-muted">{{ group.title }}</h4>
                    <div class="overflow-x-auto">
                      <table class="w-full text-left text-sm">
                        <tbody>
                          <tr v-for="(a, i) in group.args" :key="i" class="border-b border-border-dark/60">
                            <td class="px-2 py-1.5 font-mono text-xs text-grass-300">{{ a.value }}</td>
                            <td class="px-2 py-1.5 text-text-secondary">{{ argDesc(a.value) || '—' }}</td>
                            <td class="px-2 py-1.5 text-xs text-text-muted">{{ a.platforms || '全平台' }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </template>
              </div>
            </details>
          </div>

          <!-- 下载地址 -->
          <div class="card p-5">
            <h3 class="mb-3 font-semibold text-text-primary">下载地址</h3>
            <div v-if="detail.downloads.length" class="overflow-x-auto">
              <table class="w-full text-left text-sm">
                <thead>
                  <tr class="border-b border-border-dark text-xs text-text-muted">
                    <th scope="col" class="px-2 py-2">文件</th>
                    <th scope="col" class="px-2 py-2">大小</th>
                    <th scope="col" class="px-2 py-2">SHA1</th>
                    <th scope="col" class="px-2 py-2">链接</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="d in detail.downloads" :key="d.name" class="border-b border-border-dark/60">
                    <td class="px-2 py-2 text-text-primary">{{ d.name }}</td>
                    <td class="px-2 py-2 text-text-secondary">{{ formatSize(d.size) }}</td>
                    <td class="px-2 py-2 font-mono text-xs text-text-muted">{{ shortSha1(d.sha1) }}</td>
                    <td class="px-2 py-2">
                      <a :href="d.url" target="_blank" rel="noopener noreferrer nofollow" class="text-grass-400 hover:underline">下载</a>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else class="text-sm text-text-muted">该版本没有可用的下载项。</p>
          </div>

          <!-- 运行库 -->
          <details v-if="detail.libraries.length" class="card p-5">
            <summary class="cursor-pointer font-semibold text-text-primary">
              运行库 Libraries（{{ detail.libraries.length }} 个）
              <span class="ml-2 text-xs font-normal text-text-muted">点击展开</span>
            </summary>
            <div class="mt-3 overflow-x-auto">
              <table class="w-full text-left text-sm">
                <thead>
                  <tr class="border-b border-border-dark text-xs text-text-muted">
                    <th scope="col" class="px-2 py-2">文件</th>
                    <th scope="col" class="px-2 py-2">平台</th>
                    <th scope="col" class="px-2 py-2">大小</th>
                    <th scope="col" class="px-2 py-2">链接</th>
                  </tr>
                </thead>
                <tbody>
                  <template v-for="lib in detail.libraries" :key="lib.name">
                    <tr class="border-b border-border-dark/60">
                      <td class="px-2 py-2 font-mono text-xs text-text-secondary">{{ lib.name }}</td>
                      <td class="px-2 py-2 text-xs text-text-muted">{{ lib.platforms || '全平台' }}</td>
                      <td class="px-2 py-2 text-text-secondary">{{ lib.size ? formatSize(lib.size) : '-' }}</td>
                      <td class="px-2 py-2">
                        <a v-if="lib.url" :href="lib.url" target="_blank" rel="noopener noreferrer nofollow" class="text-grass-400 hover:underline">下载</a>
                        <span v-else class="text-text-muted">无</span>
                      </td>
                    </tr>
                    <tr v-for="n in lib.natives" :key="n.classifier" class="border-b border-dashed border-border-dark/60">
                      <td class="px-2 py-1.5 pl-6 font-mono text-xs text-text-muted">└ {{ classifierLabel(n.classifier) }}</td>
                      <td class="px-2 py-1.5"></td>
                      <td class="px-2 py-1.5 text-xs text-text-muted">{{ formatSize(n.size) }}</td>
                      <td class="px-2 py-1.5">
                        <a :href="n.url" target="_blank" rel="noopener noreferrer nofollow" class="text-grass-400 hover:underline">下载</a>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>
          </details>

          <p v-if="detail.mainClass" class="text-xs text-text-muted">
            客户端主类：<code class="code-block px-2 py-0.5">{{ detail.mainClass }}</code>
          </p>
        </div>
      </section>
    </div>
  </PageContainer>
</template>