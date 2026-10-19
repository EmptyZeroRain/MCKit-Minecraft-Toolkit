<script setup lang="ts">
// 版本映射查询（TODO §21）：按版本与映射来源查询类 / 方法 / 字段的三列名字对照。
//
// 设计说明：
//   - 本站不预置映射数据，数据集由 `cmd/import-mappings` 从官方来源导入；
//     未导入任何数据集时展示导入指引，而不是伪造示例数据。
//   - 结果按类聚合：类名命中优先，仅成员命中的类排在其后；展开可查看成员明细。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import FilterPanel, { type FilterOption } from '@/components/common/FilterPanel.vue'
import { getMappingClass, listMappingSets, searchMappings } from '@/api/dev'
import {
  MAPPING_SEARCH_KIND_OPTIONS,
  MAPPING_TYPES,
  displayName,
  mappingMemberKindLabel,
  mappingTypeLabel,
  type MappingClassDetail,
  type MappingClassHit,
  type MappingSearchKind,
  type MappingSet,
  type MappingType,
} from '@/types/mapping'
import { ApiError } from '@/types/api'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'
import { formatDateTime } from '@/utils/format'
import { queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: '版本映射查询',
  description:
    '查询 Minecraft 类、方法与字段在 Mojang 官方映射、Fabric Yarn、TSRG 等各套映射中的混淆名、中间名与可读名对照。',
  canonicalPath: '/mappings',
})

useRecentToolsStore().record('mappings', '版本映射查询', '查询类')

const route = useRoute()
const router = useRouter()

const filters = reactive<{
  q: string
  setId: string
  mappingType: MappingType | ''
  kind: MappingSearchKind
  page: number
  pageSize: number
}>({
  q: queryString(route.query.q),
  setId: queryString(route.query.set_id),
  mappingType: (queryString(route.query.mapping_type) as MappingType | '') || '',
  kind: (queryString(route.query.kind) as MappingSearchKind) || 'all',
  page: queryNumber(route.query.page, 1),
  pageSize: 10,
})

const sets = ref<MappingSet[]>([])
const setsLoading = ref(true)
const setsError = ref('')

const hits = ref<MappingClassHit[]>([])
const total = ref(0)
const totalPages = ref(1)
const truncated = ref(false)
const loading = ref(false)
const error = ref('')

/** 展开的类详情（同一时刻只展开一个，避免页面过长）。 */
const expandedId = ref('')
const detail = ref<MappingClassDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
const memberFilter = reactive<{ kind: string; q: string; page: number; pageSize: number }>({
  kind: '',
  q: '',
  page: 1,
  pageSize: 20,
})

const hasSets = computed(() => sets.value.length > 0)

const kindOptions: FilterOption[] = MAPPING_SEARCH_KIND_OPTIONS.map((o) => ({
  label: o.label,
  value: o.value,
}))

const typeOptions: FilterOption[] = MAPPING_TYPES.map((t) => ({
  label: mappingTypeLabel(t),
  value: t,
}))

const memberKindOptions: FilterOption[] = [
  { label: '方法', value: 'method' },
  { label: '字段', value: 'field' },
  { label: '参数', value: 'param' },
]

/** 成员类型筛选项（含「全部」）。 */
const memberKindFilterOptions: FilterOption[] = [
  { label: '全部', value: '' },
  ...memberKindOptions,
]

/** 当前数据集（用于展示来源、许可与命名空间说明）。 */
const currentSet = computed(() => sets.value.find((s) => s.id === filters.setId) ?? null)

/** 导入指引中的命令（与 cmd/import-mappings 的 flag 保持一致）。 */
const importGuides = [
  {
    title: 'Mojang 官方映射（ProGuard）',
    hint: '从版本清单取 downloads.client_mappings.url 后下载 client.txt',
    command:
      'go run ./cmd/import-mappings --file client.txt --mc-version 1.21.1 --mapping-type mojang --channel mojang --source-url "<下载地址>"',
  },
  {
    title: 'Fabric Yarn（Tiny v2）',
    hint: '先从 yarn-<version>-v2.jar 解出 mappings/mappings.tiny',
    command:
      'go run ./cmd/import-mappings --file mappings.tiny --mc-version 1.21.1 --mapping-type yarn --channel fabric --build-label "1.21.1+build.3"',
  },
  {
    title: 'TSRG / TSRG2（MCPConfig）',
    hint: 'MCPConfig 仓库按版本提供 joined.tsrg（1.13 ~ 1.20.1）',
    command:
      'go run ./cmd/import-mappings --file joined.tsrg --format tsrg --mc-version 1.16.5 --mapping-type tsrg --channel forge',
  },
]

async function loadSets() {
  setsLoading.value = true
  setsError.value = ''
  try {
    sets.value = await listMappingSets({
      mapping_type: filters.mappingType || undefined,
    })
    // 未指定数据集时默认选中第一个（按版本降序），让页面开箱可用。
    if (!filters.setId && sets.value.length) {
      filters.setId = sets.value[0].id
    }
  } catch (err) {
    setsError.value = err instanceof ApiError ? err.message : '数据集加载失败，请稍后重试'
    reportToolFailure('mappings')
  } finally {
    setsLoading.value = false
  }
}

async function loadHits() {
  if (!hasSets.value) {
    hits.value = []
    total.value = 0
    totalPages.value = 1
    return
  }
  loading.value = true
  error.value = ''
  expandedId.value = ''
  detail.value = null
  try {
    const res = await searchMappings({
      q: filters.q || undefined,
      set_id: filters.setId || undefined,
      kind: filters.kind,
      page: filters.page,
      page_size: filters.pageSize,
    })
    hits.value = res.hits
    total.value = res.total
    truncated.value = res.truncated
    totalPages.value = res.total_pages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '查询失败，请稍后重试'
    reportToolFailure('mappings')
  } finally {
    loading.value = false
  }
}

async function loadDetail(classId: string) {
  detailLoading.value = true
  detailError.value = ''
  detail.value = null
  try {
    detail.value = await getMappingClass(classId, {
      kind: memberFilter.kind || undefined,
      q: memberFilter.q || undefined,
      page: memberFilter.page,
      page_size: memberFilter.pageSize,
    })
  } catch (err) {
    detailError.value = err instanceof ApiError ? err.message : '成员加载失败'
    reportToolFailure('mappings')
  } finally {
    detailLoading.value = false
  }
}

function toggleDetail(hit: MappingClassHit) {
  if (expandedId.value === hit.id) {
    expandedId.value = ''
    detail.value = null
    return
  }
  expandedId.value = hit.id
  memberFilter.kind = ''
  memberFilter.q = ''
  memberFilter.page = 1
  loadDetail(hit.id)
}

function onMemberFilterChange() {
  memberFilter.page = 1
  if (expandedId.value) loadDetail(expandedId.value)
}

function onMemberKindSelect(value: string) {
  memberFilter.kind = value
  onMemberFilterChange()
}

function onMemberPageChange(p: number) {
  memberFilter.page = p
  if (expandedId.value) loadDetail(expandedId.value)
}

/** 成员明细可复制文本：三列名字 + 描述符。 */
function memberText(m: { kind: string; obf_name: string; mid_name: string; named_name: string }): string {
  return [mappingMemberKindLabel(m.kind), m.obf_name, m.mid_name, m.named_name]
    .filter(Boolean)
    .join(' | ')
}

function syncUrl() {
  const params: Record<string, string> = {}
  if (filters.q) params.q = filters.q
  if (filters.setId) params.set_id = filters.setId
  if (filters.mappingType) params.mapping_type = filters.mappingType
  if (filters.kind && filters.kind !== 'all') params.kind = filters.kind
  if (filters.page > 1) params.page = String(filters.page)
  router.replace({ path: '/mappings', query: params })
}

watch(
  () => [filters.setId, filters.kind],
  () => {
    filters.page = 1
    syncUrl()
    loadHits()
  },
)
watch(
  () => filters.q,
  () => {
    filters.page = 1
    syncUrl()
    loadHits()
  },
)
watch(
  () => filters.mappingType,
  async () => {
    // 类型变化会改变可选数据集：重新拉取列表并按需回落选择。
    await loadSets()
    if (filters.setId && !sets.value.some((s) => s.id === filters.setId)) {
      filters.setId = sets.value[0]?.id ?? ''
    }
    filters.page = 1
    syncUrl()
    loadHits()
  },
)

function onPageChange(p: number) {
  filters.page = p
  syncUrl()
  loadHits()
}

function clearFilters() {
  filters.q = ''
  filters.kind = 'all'
  filters.page = 1
  syncUrl()
  loadHits()
}

onMounted(async () => {
  await loadSets()
  await loadHits()
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '版本映射查询' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">版本映射查询</h1>
        <p class="text-text-secondary">
          查询类、方法与字段在混淆名 / 中间名 / 可读名三套命名空间下的对照；数据由使用者按官方来源导入，本站不预置。
        </p>
      </div>
      <ShareButton />
    </div>

    <LoadingSkeleton v-if="setsLoading" :count="3" />
    <ErrorState v-else-if="setsError" :message="setsError" retryable @retry="loadSets" />

    <!-- 空态：未导入任何数据集时给出可执行的导入指引 -->
    <template v-else-if="!hasSets">
      <EmptyState
        title="尚未导入映射数据"
        description="真实映射文件体积大且随版本变化，本站不预置猜测数据。请按下方命令从官方来源导入后再查询。"
      />
      <div class="mt-6 space-y-4">
        <article v-for="guide in importGuides" :key="guide.title" class="card p-4">
          <h2 class="text-sm font-semibold text-text-primary">{{ guide.title }}</h2>
          <p class="mb-2 mt-1 text-xs text-text-muted">{{ guide.hint }}</p>
          <div class="flex flex-wrap items-start gap-2">
            <pre
              class="min-w-0 flex-1 overflow-x-auto rounded border border-border-dark bg-ink-900 p-3 text-xs text-text-secondary"
            ><code>{{ guide.command }}</code></pre>
            <CopyButton tool="mappings" :text="guide.command" label="复制命令" success-message="已复制导入命令" />
          </div>
        </article>
        <p class="text-xs text-text-muted">
          导入完成后刷新本页；也可查看
          <RouterLink
            :to="{ name: 'dev-doc-detail', params: { slug: 'tooling-mapping-import-guide' } }"
            class="text-grass-300 hover:underline"
          >
            映射导入使用指南
          </RouterLink>
          了解各来源的获取方式与许可限制。
        </p>
      </div>
    </template>

    <template v-else>
      <!-- 数据集与筛选 -->
      <section class="card mb-6 p-4" aria-labelledby="mapping-set-title">
        <h2 id="mapping-set-title" class="mb-3 text-sm font-semibold text-text-primary">映射数据集</h2>
        <div class="grid gap-4 lg:grid-cols-4">
          <label class="block lg:col-span-2">
            <span class="mb-1 block text-xs text-text-muted">选择数据集（版本 + 来源）</span>
            <select
              v-model="filters.setId"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option v-for="s in sets" :key="s.id" :value="s.id">
                {{ s.mc_version }} · {{ mappingTypeLabel(s.mapping_type) }}
                {{ s.build_label ? `· ${s.build_label}` : '' }}（{{ s.class_count }} 类）
              </option>
            </select>
          </label>
          <div class="lg:col-span-2">
            <FilterPanel v-model="filters.mappingType" title="映射来源" :options="typeOptions" clearable />
          </div>
        </div>

        <dl v-if="currentSet" class="mt-3 grid gap-x-6 gap-y-1 text-xs text-text-muted sm:grid-cols-2">
          <div class="flex gap-2">
            <dt>标识</dt>
            <dd class="font-mono text-text-secondary">{{ currentSet.set_key }}</dd>
          </div>
          <div class="flex gap-2">
            <dt>规模</dt>
            <dd class="text-text-secondary">
              {{ currentSet.class_count }} 类 / {{ currentSet.member_count }} 成员
            </dd>
          </div>
          <div class="flex gap-2">
            <dt>导入时间</dt>
            <dd class="text-text-secondary">{{ formatDateTime(currentSet.imported_at) }}</dd>
          </div>
          <div class="flex gap-2">
            <dt>许可</dt>
            <dd class="text-text-secondary">{{ currentSet.license || '—' }}</dd>
          </div>
          <div v-if="currentSet.source_url" class="flex gap-2 sm:col-span-2">
            <dt>来源</dt>
            <dd class="min-w-0 truncate">
              <a
                :href="currentSet.source_url"
                class="text-grass-300 hover:underline"
                target="_blank"
                rel="noopener noreferrer"
              >
                {{ currentSet.source_url }}
              </a>
            </dd>
          </div>
          <div v-if="currentSet.note" class="sm:col-span-2">
            <dt class="inline">命名空间说明</dt>
            <dd class="inline text-text-secondary">：{{ currentSet.note }}</dd>
          </div>
        </dl>
      </section>

      <!-- 查询 -->
      <div class="relative mb-4 max-w-xl">
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
          v-model="filters.q"
          type="search"
          aria-label="搜索类或成员名"
          placeholder="类名或方法 / 字段名，例如：Entity / tick / func_70071_h_"
          class="h-11 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-9 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
        />
        <button
          v-if="filters.q"
          class="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary"
          aria-label="清除"
          @click="filters.q = ''"
        >
          ✕
        </button>
      </div>

      <div class="grid gap-6 lg:grid-cols-4">
        <aside class="space-y-4 lg:col-span-1">
          <FilterPanel v-model="filters.kind" title="查询范围" :options="kindOptions" />
          <button
            type="button"
            class="w-full rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
            @click="clearFilters"
          >
            清除筛选
          </button>
        </aside>

        <div class="space-y-4 lg:col-span-3">
          <p v-if="!loading && !error" class="text-xs text-text-muted">
            命中 {{ total }} 个类；同一关键词会同时匹配混淆名、中间名与可读名三列。
          </p>
          <p v-if="truncated" class="rounded border border-amber-mc/40 bg-amber-mc/10 px-3 py-2 text-xs text-amber-300">
            命中结果过多，已按扫描上限截断；请收窄关键词或指定查询范围。
          </p>

          <LoadingSkeleton v-if="loading" :count="4" />
          <ErrorState v-else-if="error" :message="error" retryable @retry="loadHits" />

          <ul v-else-if="hits.length" class="space-y-3">
            <li v-for="hit in hits" :key="hit.id" class="card overflow-hidden">
              <div class="flex flex-wrap items-start justify-between gap-2 p-4">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono text-sm font-medium text-text-primary">
                      {{ displayName(hit) }}
                    </span>
                    <span class="rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">
                      {{ hit.member_count }} 成员
                    </span>
                    <span v-if="hit.matched_total" class="text-xs text-text-muted">
                      命中 {{ hit.matched_total }} 个成员
                    </span>
                  </div>
                  <dl class="mt-2 grid gap-x-4 gap-y-0.5 text-xs sm:grid-cols-3">
                    <div class="flex gap-1">
                      <dt class="text-text-muted">obf</dt>
                      <dd class="truncate font-mono text-text-secondary">{{ hit.obf_name || '—' }}</dd>
                    </div>
                    <div class="flex gap-1">
                      <dt class="text-text-muted">mid</dt>
                      <dd class="truncate font-mono text-text-secondary">{{ hit.mid_name || '—' }}</dd>
                    </div>
                    <div class="flex gap-1">
                      <dt class="text-text-muted">named</dt>
                      <dd class="truncate font-mono text-text-secondary">{{ hit.named_name || '—' }}</dd>
                    </div>
                  </dl>
                </div>
                <button
                  type="button"
                  class="shrink-0 rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
                  :aria-expanded="expandedId === hit.id"
                  @click="toggleDetail(hit)"
                >
                  {{ expandedId === hit.id ? '收起成员' : '查看成员' }}
                </button>
              </div>

              <!-- 成员预览（查询命中时直接展示，无需展开） -->
              <ul
                v-if="hit.matched_members.length && expandedId !== hit.id"
                class="divide-y divide-border-dark border-t border-border-dark px-4 py-2 text-xs"
              >
                <li
                  v-for="m in hit.matched_members"
                  :key="m.kind + m.obf_name + m.obf_desc"
                  class="flex flex-wrap items-baseline gap-2 py-1"
                >
                  <span class="rounded bg-ink-700 px-1.5 py-0.5 text-text-muted">
                    {{ mappingMemberKindLabel(m.kind) }}
                  </span>
                  <code class="font-mono text-text-secondary">{{ displayName(m) }}</code>
                  <span class="text-text-muted">obf {{ m.obf_name || '—' }}</span>
                </li>
              </ul>

              <!-- 展开：成员明细 + 过滤 + 分页 -->
              <div v-if="expandedId === hit.id" class="border-t border-border-dark p-4">
                <div class="mb-3 grid gap-3 sm:grid-cols-2">
                  <div>
                    <span class="mb-1 block text-xs text-text-muted">成员类型</span>
                    <div class="flex flex-wrap gap-1.5">
                      <button
                        v-for="opt in memberKindFilterOptions"
                        :key="opt.value"
                        type="button"
                        class="rounded-full border px-3 py-1 text-xs transition"
                        :class="
                          memberFilter.kind === opt.value
                            ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                            : 'border-border-light bg-ink-700 text-text-secondary hover:bg-ink-600'
                        "
                        @click="onMemberKindSelect(opt.value)"
                      >
                        {{ opt.label }}
                      </button>
                    </div>
                  </div>
                  <label class="block">
                    <span class="mb-1 block text-xs text-text-muted">在成员中过滤</span>
                    <input
                      v-model="memberFilter.q"
                      type="search"
                      class="h-10 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                      placeholder="成员名关键词"
                      @change="onMemberFilterChange"
                    />
                  </label>
                </div>

                <LoadingSkeleton v-if="detailLoading" :count="2" />
                <ErrorState v-else-if="detailError" :message="detailError" retryable @retry="loadDetail(hit.id)" />
                <template v-else-if="detail">
                  <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
                    <span class="text-xs text-text-muted">共 {{ detail.member_total }} 个成员</span>
                    <CopyButton
                      tool="mappings"
                      :text="`${displayName(detail.class)}\n${detail.members.map(memberText).join('\n')}`"
                      label="复制成员列表"
                      success-message="已复制成员列表"
                    />
                  </div>
                  <div class="overflow-x-auto">
                    <table class="w-full text-left text-xs">
                      <thead class="border-b border-border-dark text-text-muted">
                        <tr>
                          <th scope="col" class="px-2 py-2 font-medium">类型</th>
                          <th scope="col" class="px-2 py-2 font-medium">obf</th>
                          <th scope="col" class="px-2 py-2 font-medium">mid</th>
                          <th scope="col" class="px-2 py-2 font-medium">named</th>
                          <th scope="col" class="hidden px-2 py-2 font-medium md:table-cell">描述符</th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-border-dark">
                        <tr v-for="m in detail.members" :key="m.kind + m.obf_name + m.obf_desc">
                          <td class="px-2 py-2 text-text-muted">{{ mappingMemberKindLabel(m.kind) }}</td>
                          <td class="px-2 py-2 font-mono text-text-secondary">{{ m.obf_name || '—' }}</td>
                          <td class="px-2 py-2 font-mono text-text-secondary">{{ m.mid_name || '—' }}</td>
                          <td class="px-2 py-2 font-mono text-grass-300">{{ m.named_name || '—' }}</td>
                          <td class="hidden px-2 py-2 font-mono text-text-muted md:table-cell">
                            {{ m.obf_desc || m.named_desc || '—' }}
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                  <p v-if="!detail.members.length" class="py-3 text-center text-xs text-text-muted">
                    该筛选条件下没有成员。
                  </p>
                  <Pagination
                    :page="memberFilter.page"
                    :page-size="memberFilter.pageSize"
                    :total="detail.member_total"
                    :total-pages="Math.max(1, Math.ceil(detail.member_total / memberFilter.pageSize))"
                    @change="onMemberPageChange"
                  />
                </template>
              </div>
            </li>
          </ul>

          <EmptyState
            v-else
            title="未命中任何类"
            description="试试缩短关键词（如只输入类名的一部分），或把查询范围改为「全部」。"
            action-label="清除筛选"
            @action="clearFilters"
          />

          <Pagination
            :page="filters.page"
            :page-size="filters.pageSize"
            :total="total"
            :total-pages="totalPages"
            @change="onPageChange"
          />
        </div>
      </div>
    </template>
  </PageContainer>
</template>
