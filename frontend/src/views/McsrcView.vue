<script setup lang="ts">
// MC 源码浏览器（mcsrc）：按版本浏览 Minecraft 的类索引。
// 后端负责下载并缓存官方 jar、建立类索引（避免每个访客各自下载数百 MB）；
// 前端按版本检索类，并可查看单个类的**结构视图**（修饰符 / 父类 / 接口 / 字段 / 方法）
// 与**继承关系**（祖先链 + 直接子类）——两者均由后端解析 class 字节码得出，无需反编译。
import { computed, defineAsyncComponent, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listVersions } from '@/api/versions'
import { getClassDetail, getClassHierarchy, getStatus, listClasses, listPrepared, prepare } from '@/api/mcsrc'
import { decompileClass } from '@/mcsrc/decompiler'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { ApiError } from '@/types/api'
import {
  MCSRC_KIND_LABELS,
  MCSRC_SIDES,
  MCSRC_SIDE_LABELS,
  javaVersionLabel,
  memberSignature,
  type McsrcClass,
  type McsrcClassStructure,
  type McsrcHierarchy,
  type McsrcSide,
  type McsrcStatus,
} from '@/types/mcsrc'
import { VERSION_TYPE_LABELS, type VersionEntry, type VersionType } from '@/types/version'
import { formatSize, formatDate } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: 'MC 源码浏览器',
  description:
    '按 Minecraft 版本浏览官方 jar 的类索引：服务端缓存 jar 并建立索引，支持类名检索、包过滤、类结构与继承关系查看，并可在浏览器内反编译 Java 源码。',
  canonicalPath: '/mcsrc',
})

useRecentToolsStore().record('mcsrc', 'MC 源码浏览器', '查询类')

// Monaco 查看器体积较大，懒加载：仅打开源码面板时才载入。
const SourceViewer = defineAsyncComponent(() => import('@/components/mcsrc/SourceViewer.vue'))

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const ui = useUiStore()

// ---- 版本列表 ----
const listFilters = reactive({
  type: ((route.query.type as string) as VersionType | 'all') || 'all',
  q: (route.query.q as string) || '',
})
const versions = ref<VersionEntry[]>([])
const listLoading = ref(false)
const listError = ref('')

const typeOptions: Array<{ value: VersionType | 'all'; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'release', label: VERSION_TYPE_LABELS.release },
  { value: 'snapshot', label: VERSION_TYPE_LABELS.snapshot },
  { value: 'old_beta', label: VERSION_TYPE_LABELS.old_beta },
  { value: 'old_alpha', label: VERSION_TYPE_LABELS.old_alpha },
]

// ---- 当前版本状态 ----
const selectedId = ref((route.query.version as string) || '')
const side = ref<McsrcSide>('client')
const status = ref<McsrcStatus | null>(null)
const statusLoading = ref(false)
const statusError = ref('')
const preparing = ref(false)
const prepared = ref<McsrcStatus[]>([])

// ---- 类检索 ----
const classQuery = reactive({ q: '', pkg: '', includeInner: false, page: 1, pageSize: 30 })
const classes = ref<McsrcClass[]>([])
const classTotal = ref(0)
const classTotalPages = ref(1)
const classesLoading = ref(false)
const classesError = ref('')
const needPrepare = ref(false)

// ---- 类详情（结构视图 + 继承关系）----
const selectedClass = ref('')
const structure = ref<McsrcClassStructure | null>(null)
const hierarchy = ref<McsrcHierarchy | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
/** 方法表是否展示编译器生成成员（bridge / synthetic） */
const showSynthetic = ref(false)

const visibleMethods = computed(() =>
  (structure.value?.methods ?? []).filter((m) => showSynthetic.value || !m.synthetic),
)
const hiddenMethodCount = computed(
  () => (structure.value?.methods ?? []).filter((m) => m.synthetic).length,
)
/** 祖先链（根 → 直接父类），配合本类名渲染为「根 → … → 本类」 */
const ancestryChain = computed(() => (hierarchy.value ? [...hierarchy.value.ancestors].reverse() : []))
/** 可继续下钻的祖先（jar 外的终点类不可点击） */
const clickableAncestors = computed(() => {
  const h = hierarchy.value
  if (!h) return new Set<string>()
  const list = h.ancestors_in_jar ? h.ancestors : h.ancestors.slice(0, Math.max(0, h.ancestors.length - 1))
  return new Set(list)
})

/** 是否具备触发「准备版本」的权限（前端仅决定按钮显隐，后端仍校验） */
const canPrepare = computed(() => auth.isLoggedIn && auth.hasPermission('ops.mcsrc'))
const isReady = computed(() => !!status.value?.indexed)

async function loadVersions() {
  listLoading.value = true
  listError.value = ''
  try {
    const res = await listVersions({ type: listFilters.type, q: listFilters.q })
    versions.value = res.versions
    if (!selectedId.value && res.versions.length) {
      selectedId.value = res.latest?.release || res.versions[0].id
    }
  } catch (err) {
    listError.value = err instanceof ApiError ? err.message : '加载版本列表失败'
  } finally {
    listLoading.value = false
  }
}

async function loadPrepared() {
  try {
    prepared.value = await listPrepared()
  } catch {
    prepared.value = []
  }
}

async function loadStatus() {
  if (!selectedId.value) return
  statusLoading.value = true
  statusError.value = ''
  needPrepare.value = false
  try {
    status.value = await getStatus(selectedId.value, side.value)
    if (status.value.indexed) {
      await loadClasses()
    } else {
      classes.value = []
      classTotal.value = 0
      classTotalPages.value = 1
    }
  } catch (err) {
    statusError.value = err instanceof ApiError ? err.message : '查询版本状态失败'
    status.value = null
  } finally {
    statusLoading.value = false
  }
}

async function loadClasses() {
  if (!selectedId.value) return
  classesLoading.value = true
  classesError.value = ''
  needPrepare.value = false
  try {
    const res = await listClasses({
      version: selectedId.value,
      side: side.value,
      q: classQuery.q || undefined,
      pkg: classQuery.pkg || undefined,
      include_inner: classQuery.includeInner,
      page: classQuery.page,
      page_size: classQuery.pageSize,
    })
    classes.value = res.items
    classTotal.value = res.total
    classTotalPages.value = Math.max(1, Math.ceil(res.total / (res.page_size || classQuery.pageSize)))
  } catch (err) {
    if (err instanceof ApiError && err.code === 'NEED_PREPARE') {
      needPrepare.value = true
      classes.value = []
      classTotal.value = 0
    } else {
      classesError.value = err instanceof ApiError ? err.message : '检索失败'
    }
  } finally {
    classesLoading.value = false
  }
}

function selectVersion(id: string) {
  if (id === selectedId.value) return
  selectedId.value = id
  classQuery.page = 1
  closeClass()
  router.replace({ query: { ...route.query, version: id } })
}

function onSearchClasses() {
  classQuery.page = 1
  closeClass()
  loadClasses()
}

function onPageChange(p: number) {
  classQuery.page = p
  closeClass()
  loadClasses()
}

// ---- 源码视图（浏览器内 Vineflower/WASM 反编译，无需部署侧 JVM）----
const sourceClass = ref('')
const sourceText = ref('')
const sourceLoading = ref(false)
const sourceError = ref('')

/** 关闭类详情面板（同时关闭源码视图） */
function closeClass() {
  selectedClass.value = ''
  structure.value = null
  hierarchy.value = null
  detailError.value = ''
  closeSource()
}

/** 关闭源码视图 */
function closeSource() {
  sourceClass.value = ''
  sourceText.value = ''
  sourceError.value = ''
  sourceLoading.value = false
}

/** 打开类详情：并行拉取结构视图与继承关系（继承关系失败不阻塞结构展示） */
async function openClass(name: string) {
  if (!selectedId.value) return
  closeSource()
  selectedClass.value = name
  structure.value = null
  hierarchy.value = null
  detailError.value = ''
  detailLoading.value = true
  const [detailRes, hierRes] = await Promise.allSettled([
    getClassDetail(selectedId.value, name, side.value),
    getClassHierarchy(selectedId.value, name, side.value),
  ])
  if (detailRes.status === 'fulfilled') {
    structure.value = detailRes.value
  } else {
    detailError.value = detailRes.reason instanceof ApiError ? detailRes.reason.message : '加载类结构失败'
  }
  if (hierRes.status === 'fulfilled') {
    hierarchy.value = hierRes.value
  }
  detailLoading.value = false
}

/** 打开源码视图：在浏览器内用 Vineflower（WASM）反编译该类 */
async function openSource(name: string) {
  if (!selectedId.value) return
  sourceClass.value = name
  sourceText.value = ''
  sourceError.value = ''
  sourceLoading.value = true
  try {
    const res = await decompileClass(selectedId.value, side.value, name)
    // 期间用户可能已切换类 / 关闭面板，丢弃过期结果。
    if (sourceClass.value !== name) return
    sourceText.value = res.source
  } catch (err) {
    if (sourceClass.value !== name) return
    sourceError.value = err instanceof ApiError ? err.message : '反编译失败'
  } finally {
    if (sourceClass.value === name) sourceLoading.value = false
  }
}

async function doPrepare() {
  if (preparing.value || !selectedId.value) return
  preparing.value = true
  try {
    const st = await prepare(selectedId.value, side.value)
    status.value = st
    ui.success(`已准备 ${st.version}（${MCSRC_SIDE_LABELS[side.value]}），共 ${st.class_count} 个类`)
    classQuery.page = 1
    await loadPrepared()
    await loadClasses()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '准备失败')
  } finally {
    preparing.value = false
  }
}

// 版本列表筛选变化时重新拉取（保留选中版本）。
watch(
  () => [listFilters.type, listFilters.q],
  () => {
    loadVersions()
  },
)

// 切换选中版本或侧别时刷新状态。
watch([selectedId, side], () => {
  classQuery.page = 1
  closeClass()
  loadStatus()
})

onMounted(async () => {
  // 探测管理会话：决定「准备该版本」按钮是否显示（后端仍会校验权限）。
  void auth.restoreSession()
  await loadVersions()
  await loadPrepared()
  if (selectedId.value) await loadStatus()
})
</script>

<template>
  <PageContainer>
    <Breadcrumbs :items="[{ label: '首页', to: '/' }, { label: '工具中心', to: '/tools' }, { label: 'MC 源码浏览器' }]" />

    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold text-text-primary">MC 源码浏览器</h1>
        <p class="mt-2 max-w-3xl text-sm text-text-secondary">
          按版本浏览 Minecraft 官方 jar 的类索引。jar 由服务端下载并缓存（校验官方 sha1），
          索引只读取 zip 中央目录，因此数百 MB 的 jar 也能秒级建索引、快速检索；
          点击任一类可查看其结构（修饰符 / 父类 / 接口 / 字段 / 方法）与继承关系（祖先链 / 直接子类），
          也可在浏览器内直接反编译出 Java 源码（Vineflower WASM，无需服务端 JVM）。
        </p>
      </div>
      <ShareButton />
    </div>

    <div class="grid gap-6 lg:grid-cols-[300px_1fr]">
      <!-- 版本列表 -->
      <aside class="space-y-3">
        <div class="card p-3">
          <input
            v-model="listFilters.q"
            type="search"
            aria-label="搜索版本"
            placeholder="搜索版本号，如 1.21 / 24w"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
          <div class="mt-3 flex flex-wrap gap-1.5">
            <button
              v-for="opt in typeOptions"
              :key="opt.value"
              type="button"
              class="rounded-full border px-2.5 py-1 text-xs transition"
              :class="
                listFilters.type === opt.value
                  ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                  : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
              "
              @click="listFilters.type = opt.value"
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
                <span class="shrink-0 rounded-full border border-border-dark px-2 py-0.5 text-[10px] text-text-muted">
                  {{ VERSION_TYPE_LABELS[v.type] ?? v.type }}
                </span>
              </button>
            </li>
          </ul>
        </div>

        <div v-if="prepared.length" class="card p-3">
          <h2 class="mb-2 text-xs font-semibold text-text-secondary">已缓存版本</h2>
          <ul class="space-y-1">
            <li v-for="p in prepared" :key="`${p.version}-${p.side}`" class="flex items-center justify-between gap-2 text-xs">
              <button
                type="button"
                class="font-mono text-grass-300 hover:underline"
                @click="
                  selectVersion(p.version);
                  side = p.side
                "
              >
                {{ p.version }}
              </button>
              <span class="shrink-0 text-text-muted">
                {{ MCSRC_SIDE_LABELS[p.side] }} · {{ p.indexed ? `${p.class_count} 类` : '仅 jar' }}
              </span>
            </li>
          </ul>
        </div>
      </aside>

      <!-- 状态 + 类检索 -->
      <section class="min-w-0 space-y-4">
        <EmptyState v-if="!selectedId" title="请选择版本" description="从左侧列表选择 Minecraft 版本，查看类索引。" />

        <template v-else>
          <div class="card p-5">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="flex flex-wrap items-center gap-3">
                <h2 class="font-mono text-xl font-bold text-text-primary">{{ selectedId }}</h2>
                <div class="flex overflow-hidden rounded-md border border-border-dark">
                  <button
                    v-for="s in MCSRC_SIDES"
                    :key="s"
                    type="button"
                    class="px-3 py-1 text-xs transition"
                    :class="side === s ? 'bg-grass-600 text-white' : 'bg-ink-700 text-text-secondary hover:bg-ink-600'"
                    @click="side = s"
                  >
                    {{ MCSRC_SIDE_LABELS[s] }}
                  </button>
                </div>
              </div>

              <div class="flex items-center gap-2">
                <span
                  v-if="status"
                  class="rounded-full border px-2.5 py-0.5 text-xs"
                  :class="
                    isReady
                      ? 'border-grass-500/40 bg-grass-500/10 text-grass-300'
                      : status.cached
                        ? 'border-amber-mc/40 bg-amber-mc/10 text-amber-mc'
                        : 'border-border-dark bg-ink-700 text-text-muted'
                  "
                >
                  {{ isReady ? '索引就绪' : status.cached ? '仅已缓存 jar' : '尚未准备' }}
                </span>
                <button
                  v-if="status && !isReady && canPrepare"
                  type="button"
                  class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
                  :disabled="preparing"
                  @click="doPrepare"
                >
                  {{ preparing ? '下载并索引中…' : '准备该版本' }}
                </button>
              </div>
            </div>

            <div v-if="statusLoading" role="status" aria-live="polite" class="mt-4 text-sm text-text-muted">
              查询版本状态中…
            </div>
            <p v-else-if="statusError" role="alert" class="mt-4 text-sm text-danger-mc">{{ statusError }}</p>
            <div v-else-if="status" class="mt-4 grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
              <div>
                <div class="text-xs text-text-muted">jar 体积</div>
                <div class="text-text-primary">{{ status.cached ? formatSize(status.size) : '—' }}</div>
              </div>
              <div>
                <div class="text-xs text-text-muted">类数量</div>
                <div class="text-text-primary">{{ status.indexed ? status.class_count : '—' }}</div>
              </div>
              <div>
                <div class="text-xs text-text-muted">抓取时间</div>
                <div class="text-text-primary">{{ status.fetched_at ? formatDate(status.fetched_at) : '—' }}</div>
              </div>
              <div>
                <div class="text-xs text-text-muted">索引时间</div>
                <div class="text-text-primary">{{ status.index_built_at ? formatDate(status.index_built_at) : '—' }}</div>
              </div>
            </div>

            <p
              v-if="status && !isReady && !canPrepare && !statusLoading"
              class="mt-4 rounded border border-border-dark bg-ink-900 px-3 py-2 text-xs text-text-muted"
            >
              该版本尚未准备就绪。为避免重复下载数百 MB，jar 由服务端统一缓存，请联系管理员执行「准备该版本」。
            </p>
            <p v-if="status?.truncated" class="mt-2 text-xs text-amber-mc">
              该 jar 的类数量超出索引上限，索引已截断（仅保留前若干类）。
            </p>
          </div>

          <div class="card p-5">
            <h3 class="mb-3 font-semibold text-text-primary">类检索</h3>

            <div class="flex flex-wrap gap-3">
              <input
                v-model="classQuery.q"
                type="search"
                aria-label="搜索类名"
                placeholder="搜索类名，如 MinecraftServer"
                class="h-10 min-w-0 flex-1 rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                @change="onSearchClasses"
              />
              <input
                v-model="classQuery.pkg"
                type="search"
                aria-label="按包名过滤"
                placeholder="包名前缀，如 net.minecraft.server"
                class="h-10 min-w-0 flex-1 rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                @change="onSearchClasses"
              />
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="classQuery.includeInner" type="checkbox" class="h-4 w-4 accent-grass-600" @change="onSearchClasses" />
                含内部类
              </label>
            </div>

            <!-- 源码视图：浏览器内 Vineflower（WASM）反编译 -->
            <div v-if="sourceClass" class="mt-4 space-y-3" data-test="class-source">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="break-all font-mono text-sm text-text-primary">{{ sourceClass }}</div>
                  <p class="mt-1 text-xs text-text-muted">
                    Java 源码由 Vineflower 在浏览器内反编译（WASM），仅供阅读参考，与官方源码可能有出入。
                  </p>
                </div>
                <div class="flex shrink-0 items-center gap-2">
                  <CopyButton tool="mcsrc" :text="sourceClass" />
                  <button
                    type="button"
                    class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700"
                    @click="closeSource"
                  >
                    返回列表
                  </button>
                </div>
              </div>

              <div v-if="sourceLoading" role="status" aria-live="polite" class="text-sm text-text-muted">
                反编译中…（首次会加载反编译引擎与 jar，可能需要几秒）
              </div>
              <p v-else-if="sourceError" role="alert" class="text-sm text-danger-mc">{{ sourceError }}</p>
              <SourceViewer
                v-else
                :source="sourceText"
                language="java"
                class="h-[65vh] overflow-hidden rounded-md border border-border-dark"
              />
            </div>

            <!-- 类详情：结构视图 + 继承关系（均由后端解析 class 字节码得出） -->
            <div v-else-if="selectedClass" class="mt-4 space-y-4" data-test="class-detail">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono text-sm text-text-primary">{{ selectedClass }}</span>
                    <span
                      v-if="structure"
                      class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted"
                    >
                      {{ MCSRC_KIND_LABELS[structure.kind] }}
                    </span>
                    <span
                      v-if="structure"
                      class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted"
                    >
                      {{ javaVersionLabel(structure.major) }}
                    </span>
                    <span
                      v-if="structure?.inner"
                      class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted"
                    >
                      内部类
                    </span>
                  </div>
                  <p v-if="structure" class="mt-1 break-all font-mono text-xs text-text-muted">
                    {{ [...structure.modifiers, structure.kind, structure.simple].join(' ') }}
                  </p>
                </div>
                <div class="flex shrink-0 items-center gap-2">
                  <button
                    type="button"
                    class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-grass-300 transition hover:bg-ink-700"
                    @click="openSource(selectedClass)"
                  >
                    查看源码
                  </button>
                  <CopyButton tool="mcsrc" :text="selectedClass" />
                  <button
                    type="button"
                    class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700"
                    @click="closeClass"
                  >
                    返回列表
                  </button>
                </div>
              </div>

              <div v-if="detailLoading" role="status" aria-live="polite" class="text-sm text-text-muted">
                加载类结构中…
              </div>
              <p v-else-if="detailError" role="alert" class="text-sm text-danger-mc">{{ detailError }}</p>
              <template v-else-if="structure">
                <div class="grid gap-3 text-sm sm:grid-cols-2">
                  <div>
                    <div class="text-xs text-text-muted">父类</div>
                    <div class="mt-1 flex flex-wrap items-center gap-2">
                      <button
                        v-if="structure.in_jar_super"
                        type="button"
                        class="font-mono text-xs text-grass-300 hover:underline"
                        @click="openClass(structure.super)"
                      >
                        {{ structure.super }}
                      </button>
                      <span v-else class="break-all font-mono text-xs text-text-secondary">{{ structure.super || '—' }}</span>
                      <span v-if="structure.super && !structure.in_jar_super" class="text-[10px] text-text-muted">jar 外</span>
                    </div>
                  </div>
                  <div>
                    <div class="text-xs text-text-muted">直接接口</div>
                    <p v-if="!structure.interfaces.length" class="mt-1 text-xs text-text-muted">—</p>
                    <ul v-else class="mt-1 flex flex-wrap gap-x-3 gap-y-1">
                      <li v-for="iface in structure.interfaces" :key="iface">
                        <button
                          v-if="structure.in_jar_interfaces.includes(iface)"
                          type="button"
                          class="break-all font-mono text-xs text-grass-300 hover:underline"
                          @click="openClass(iface)"
                        >
                          {{ iface }}
                        </button>
                        <span v-else class="break-all font-mono text-xs text-text-secondary">{{ iface }}</span>
                      </li>
                    </ul>
                  </div>
                </div>

                <!-- 继承关系 -->
                <div v-if="hierarchy" data-test="class-hierarchy" class="rounded-md border border-border-dark bg-ink-900/60 p-3">
                  <div class="text-xs font-semibold text-text-secondary">继承关系</div>
                  <div class="mt-2 flex flex-wrap items-center gap-1 text-xs">
                    <span class="text-text-muted">向上：</span>
                    <span v-if="!ancestryChain.length" class="text-text-muted">无祖先（根类）</span>
                    <template v-else>
                      <template v-for="a in ancestryChain" :key="a">
                        <button
                          v-if="clickableAncestors.has(a)"
                          type="button"
                          class="break-all font-mono text-grass-300 hover:underline"
                          @click="openClass(a)"
                        >
                          {{ a }}
                        </button>
                        <span v-else class="break-all font-mono text-text-muted">{{ a }}</span>
                        <span class="text-text-muted">→</span>
                      </template>
                    </template>
                    <span class="break-all font-mono font-semibold text-text-primary">{{ hierarchy.name }}</span>
                    <span v-if="!hierarchy.ancestors_in_jar" class="text-[10px] text-text-muted">（祖先链延伸至 jar 外）</span>
                  </div>
                  <div class="mt-2 flex flex-wrap items-center gap-1 text-xs">
                    <span class="text-text-muted">向下：</span>
                    <span v-if="!hierarchy.descendant_count" class="text-text-muted">无直接子类</span>
                    <template v-else>
                      <span class="text-text-secondary">
                        直接子类 {{ hierarchy.descendant_count }} 个（含间接共 {{ hierarchy.total_descendants }} 个）
                      </span>
                      <ul class="mt-1 flex w-full flex-wrap gap-x-3 gap-y-1">
                        <li v-for="d in hierarchy.descendants" :key="d.name">
                          <button
                            type="button"
                            class="break-all font-mono text-xs text-grass-300 hover:underline"
                            @click="openClass(d.name)"
                          >
                            {{ d.name }}
                          </button>
                        </li>
                      </ul>
                      <span v-if="hierarchy.truncated" class="w-full text-[10px] text-amber-mc">
                        子类过多，仅列出前 {{ hierarchy.descendants.length }} 个。
                      </span>
                    </template>
                  </div>
                </div>

                <!-- 字段 -->
                <div>
                  <h4 class="mb-2 text-xs font-semibold text-text-secondary">字段（{{ structure.fields.length }}）</h4>
                  <p v-if="!structure.fields.length" class="text-xs text-text-muted">无字段。</p>
                  <div v-else class="overflow-x-auto">
                    <table class="w-full min-w-[480px] text-left text-xs">
                      <thead class="border-b border-border-dark text-text-muted">
                        <tr>
                          <th scope="col" class="px-2 py-1.5">修饰符</th>
                          <th scope="col" class="px-2 py-1.5">类型</th>
                          <th scope="col" class="px-2 py-1.5">名称</th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-border-dark">
                        <tr v-for="f in structure.fields" :key="`${f.name}:${f.descriptor}`">
                          <td class="px-2 py-1.5 font-mono text-text-muted">{{ f.modifiers.join(' ') || '—' }}</td>
                          <td class="px-2 py-1.5 break-all font-mono text-text-secondary">{{ f.type || '?' }}</td>
                          <td class="px-2 py-1.5 break-all font-mono text-text-primary">{{ f.name }}</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>

                <!-- 方法 -->
                <div>
                  <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
                    <h4 class="text-xs font-semibold text-text-secondary">方法（{{ visibleMethods.length }}）</h4>
                    <label v-if="hiddenMethodCount" class="flex items-center gap-2 text-xs text-text-secondary">
                      <input v-model="showSynthetic" type="checkbox" class="h-3.5 w-3.5 accent-grass-600" />
                      显示编译器生成成员（{{ hiddenMethodCount }}）
                    </label>
                  </div>
                  <p v-if="!visibleMethods.length" class="text-xs text-text-muted">无方法。</p>
                  <div v-else class="overflow-x-auto">
                    <table class="w-full min-w-[560px] text-left text-xs">
                      <thead class="border-b border-border-dark text-text-muted">
                        <tr>
                          <th scope="col" class="px-2 py-1.5">修饰符</th>
                          <th scope="col" class="px-2 py-1.5">签名</th>
                          <th scope="col" class="px-2 py-1.5">描述符</th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-border-dark">
                        <tr v-for="m in visibleMethods" :key="`${m.name}:${m.descriptor}`">
                          <td class="px-2 py-1.5 font-mono text-text-muted">{{ m.modifiers.join(' ') || '—' }}</td>
                          <td class="px-2 py-1.5 break-all font-mono text-text-primary">{{ memberSignature(m) }}</td>
                          <td class="px-2 py-1.5 break-all font-mono text-text-muted">{{ m.descriptor }}</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </template>
            </div>

            <div v-else-if="classesLoading" role="status" aria-live="polite" class="mt-4 text-sm text-text-muted">
              检索中…
            </div>
            <ErrorState v-else-if="classesError" class="mt-4" :message="classesError" retryable @retry="loadClasses" />
            <EmptyState
              v-else-if="needPrepare || !isReady"
              class="mt-4"
              title="尚未建立类索引"
              description="该版本还没有类索引，请先准备该版本（下载 jar 并建索引）。"
            />
            <div v-else-if="!classes.length" class="mt-4 text-sm text-text-muted">没有匹配的类。</div>
            <div v-else class="mt-4 overflow-x-auto">
              <table class="w-full min-w-[640px] text-left text-sm">
                <thead class="border-b border-border-dark text-xs text-text-muted">
                  <tr>
                    <th scope="col" class="px-3 py-2">类（全限定名）</th>
                    <th scope="col" class="px-3 py-2">包名</th>
                    <th scope="col" class="px-3 py-2 text-right">大小</th>
                    <th scope="col" class="px-3 py-2 text-right">操作</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border-dark">
                  <tr v-for="c in classes" :key="c.name" class="hover:bg-ink-800/60">
                    <td class="px-3 py-2">
                      <button
                        type="button"
                        class="break-all font-mono text-xs text-grass-300 hover:underline"
                        @click="openClass(c.name)"
                      >
                        {{ c.name }}
                      </button>
                      <span v-if="c.inner" class="ml-2 rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted">内部类</span>
                    </td>
                    <td class="px-3 py-2 font-mono text-xs text-text-muted">{{ c.pkg || '—' }}</td>
                    <td class="px-3 py-2 text-right text-xs text-text-secondary">{{ formatSize(c.size) }}</td>
                    <td class="px-3 py-2">
                      <div class="flex justify-end gap-2">
                        <button
                          type="button"
                          class="rounded border border-border-dark px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700"
                          @click="openClass(c.name)"
                        >
                          查看结构
                        </button>
                        <button
                          type="button"
                          class="rounded border border-border-dark px-2 py-1 text-xs text-grass-300 transition hover:bg-ink-700"
                          @click="openSource(c.name)"
                        >
                          查看源码
                        </button>
                        <CopyButton tool="mcsrc" :text="c.name" />
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <Pagination
              v-if="isReady && classTotal > 0 && !selectedClass && !sourceClass"
              class="mt-4"
              :page="classQuery.page"
              :page-size="classQuery.pageSize"
              :total="classTotal"
              :total-pages="classTotalPages"
              @change="onPageChange"
            />
          </div>
        </template>
      </section>
    </div>
  </PageContainer>
</template>
