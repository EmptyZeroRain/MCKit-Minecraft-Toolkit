<script setup lang="ts">
// MC 源码浏览器（mcsrc）：VS Code 风格工作区。
// 布局对齐上游 mcsrc：左侧栏（版本选择 / 类检索 / 类列表）+ 右侧标签页（源码 / 结构 + 继承）。
// 数据来源：jar 与类索引由服务端下载并缓存（校验官方 sha1）；源码在浏览器内用 Vineflower（WASM）反编译，
// 因此部署侧无需 JVM。
//
// 高度约定：工作区整体是一个定高 flex 容器，编辑器所在的那一层必须是 `min-h-0 flex-1`，
// 否则 Monaco 宿主的高度会塌陷成几像素（历史上就是这里出的问题）。
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
import { getClassDetail, getClassHierarchy, getStatus, listClasses, prepare } from '@/api/mcsrc'
import { decompileClass } from '@/mcsrc/decompiler'
import { resolveDeclaration } from '@/mcsrc/navigation'
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
  type McsrcJumpTarget,
  type McsrcRevealTarget,
  type McsrcSide,
  type McsrcStatus,
  type McsrcToken,
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

// Monaco 查看器体积较大，懒加载：仅打开源码标签时才载入。
const SourceViewer = defineAsyncComponent(() => import('@/components/mcsrc/SourceViewer.vue'))

const route = useRoute()
const router = useRouter()
const ui = useUiStore()

// ---- 版本列表（侧栏顶部的下拉浮层）----
const versionOpen = ref(false)
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
/** 自动准备中（服务端下载 jar + 建索引）——选中版本即触发，无需手动点击 */
const autoPreparing = ref(false)
/** 自动准备进度（来自 /mcsrc/status 轮询，用于渲染进度条） */
const prepareProgress = ref<McsrcStatus | null>(null)
const prepareError = ref('')

/**
 * 状态 / 自动准备的请求序号。
 * 版本或侧别切换后，旧的异步结果（尤其是耗时的自动准备）必须整体丢弃，
 * 否则会把上一个版本的状态、进度或类列表写回界面。
 */
let statusSeq = 0

// ---- 类检索 ----
const classQuery = reactive({ q: '', pkg: '', includeInner: false, page: 1, pageSize: 30 })
const classes = ref<McsrcClass[]>([])
const classTotal = ref(0)
const classTotalPages = ref(1)
const classesLoading = ref(false)
const classesError = ref('')
const needPrepare = ref(false)

// ---- 标签页（VS Code 式：每个类一个标签，切换不丢内容）----
type McsrcTabKind = 'source' | 'structure'

interface McsrcTab {
  /** `${kind}:${className}` */
  key: string
  kind: McsrcTabKind
  className: string
}

/** 单个类的源码标签状态 */
interface SourceTabState {
  loading: boolean
  error: string
  text: string
  tokens: McsrcToken[]
  /** 打开后需要定位的成员（跳转落点） */
  reveal: McsrcRevealTarget | null
}

/** 单个类的结构标签状态 */
interface StructureTabState {
  loading: boolean
  error: string
  structure: McsrcClassStructure | null
  hierarchy: McsrcHierarchy | null
}

const tabs = ref<McsrcTab[]>([])
const activeKey = ref('')
const sourceStates = reactive<Record<string, SourceTabState>>({})
const structureStates = reactive<Record<string, StructureTabState>>({})

const activeTab = computed(() => tabs.value.find((t) => t.key === activeKey.value) ?? null)
const activeSource = computed(() =>
  activeTab.value?.kind === 'source' ? sourceStates[activeTab.value.className] : undefined,
)
const activeStructure = computed(() =>
  activeTab.value?.kind === 'structure' ? structureStates[activeTab.value.className] : undefined,
)

/** 标签标题：取类简名 */
function tabLabel(tab: McsrcTab): string {
  const simple = tab.className.split('.').pop() ?? tab.className
  return tab.kind === 'source' ? simple : `${simple} · 结构`
}

/** 面包屑：包路径逐级 + 类名 */
const breadcrumbParts = computed(() => {
  if (!activeTab.value) return []
  const parts = activeTab.value.className.split('.')
  return parts
})

/** 方法表是否展示编译器生成成员（bridge / synthetic） */
const showSynthetic = ref(false)

const visibleMethods = computed(() =>
  (activeStructure.value?.structure?.methods ?? []).filter((m) => showSynthetic.value || !m.synthetic),
)
const hiddenMethodCount = computed(
  () => (activeStructure.value?.structure?.methods ?? []).filter((m) => m.synthetic).length,
)
/** 祖先链（根 → 直接父类），配合本类名渲染为「根 → … → 本类」 */
const ancestryChain = computed(() =>
  activeStructure.value?.hierarchy ? [...activeStructure.value.hierarchy.ancestors].reverse() : [],
)
/** 可继续下钻的祖先（jar 外的终点类不可点击） */
const clickableAncestors = computed(() => {
  const h = activeStructure.value?.hierarchy
  if (!h) return new Set<string>()
  const list = h.ancestors_in_jar ? h.ancestors : h.ancestors.slice(0, Math.max(0, h.ancestors.length - 1))
  return new Set(list)
})

const isReady = computed(() => !!status.value?.indexed)

/** 状态徽章文案 */
const statusLabel = computed(() => {
  if (isReady.value) return '索引就绪'
  if (autoPreparing.value) return '正在准备…'
  if (prepareError.value) return '准备失败'
  return status.value?.cached ? '仅已缓存 jar' : '尚未准备'
})

// ---- 自动准备（下载 jar + 建索引）与进度条 ----
/** 进度轮询间隔：足够平滑，又不会给服务端带来压力（status 为只读轻量接口） */
const PREPARE_POLL_MS = 600
/** 服务端正忙（并发上限 / 限流）时的退避间隔 */
const PREPARE_RETRY_MS = 4000
const PREPARE_MAX_ATTEMPTS = 4

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** 进度百分比：仅「下载中且已知总量」时可算，其余为不确定进度（null） */
const progressPercent = computed(() => {
  const p = prepareProgress.value
  if (!p || p.phase !== 'downloading' || !p.total || p.total <= 0) return null
  const done = Math.min(p.downloaded ?? 0, p.total)
  return Math.max(0, Math.min(100, Math.floor((done / p.total) * 100)))
})

/** 进度文案 */
const progressLabel = computed(() => {
  const p = prepareProgress.value
  if (!p || !p.phase) return '正在准备…'
  if (p.phase === 'indexing') return '正在建立类索引…'
  if (p.total && p.total > 0) {
    return `正在下载官方 jar（${formatSize(p.downloaded ?? 0)} / ${formatSize(p.total)}）`
  }
  return `正在下载官方 jar（已下载 ${formatSize(p.downloaded ?? 0)}）`
})

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

/**
 * 查询版本状态；未就绪则**自动准备**（下载 jar + 建索引），全程无需手动点击。
 */
async function loadStatus() {
  if (!selectedId.value) return
  const version = selectedId.value
  const targetSide = side.value
  const seq = ++statusSeq
  statusLoading.value = true
  statusError.value = ''
  prepareError.value = ''
  prepareProgress.value = null
  needPrepare.value = false
  try {
    const st = await getStatus(version, targetSide)
    if (seq !== statusSeq) return
    status.value = st
    // 其他访客正在准备该版本时，状态里已带进度 —— 立刻展示进度条，不必等下一次轮询。
    if (st.preparing) prepareProgress.value = st
  } catch (err) {
    if (seq !== statusSeq) return
    statusError.value = err instanceof ApiError ? err.message : '查询版本状态失败'
    status.value = null
    return
  } finally {
    if (seq === statusSeq) statusLoading.value = false
  }
  if (seq !== statusSeq) return
  if (isReady.value) {
    await loadClasses()
    return
  }
  classes.value = []
  classTotal.value = 0
  classTotalPages.value = 1
  await autoPrepare(version, targetSide, seq)
}

/**
 * 自动准备：触发服务端下载官方 jar 并建立类索引，同时轮询状态驱动进度条。
 *
 * - 进度条数据来自 `/mcsrc/status`（只读、轻量），而非 prepare 的响应体；
 *   prepare 请求本身只在结束时给出最终状态。
 * - 服务端并发上限为 1，若其他访客正在下载，本请求会被 429 拒绝，此时退避重试。
 */
async function autoPrepare(version: string, targetSide: McsrcSide, seq: number) {
  autoPreparing.value = true
  try {
    for (let attempt = 1; attempt <= PREPARE_MAX_ATTEMPTS; attempt++) {
      if (seq !== statusSeq) return
      // 先发起（不 await），让轮询先跑起来渲染进度条。
      const outcome = prepare(version, targetSide).then(
        (st) => ({ ok: true as const, value: st }),
        (err: unknown) => ({ ok: false as const, value: err }),
      )
      // 与轮询「赛跑」：请求结束即跳出，未结束则每 600ms 拉一次进度。
      // 不先 sleep，避免请求已经失败（如额度拒绝）时用户还要白等一轮。
      while (seq === statusSeq) {
        const tick = await Promise.race([
          outcome.then(() => 'done' as const),
          sleep(PREPARE_POLL_MS).then(() => 'poll' as const),
        ])
        if (seq !== statusSeq) return
        if (tick === 'done') break
        try {
          prepareProgress.value = await getStatus(version, targetSide)
        } catch {
          // 轮询失败不致命：忽略瞬时错误，继续等 prepare 的结果。
        }
      }
      const res = await outcome
      if (seq !== statusSeq) return
      if (res.ok) {
        status.value = res.value
        prepareProgress.value = null
        ui.success(`已准备 ${res.value.version}（${MCSRC_SIDE_LABELS[targetSide]}），共 ${res.value.class_count} 个类`)
        await loadClasses()
        return
      }
      // 服务端正为其他版本下载（并发上限 / 限流）→ 退避重试。
      const busy = res.value instanceof ApiError && res.value.code === 'RATE_LIMITED'
      if (!busy) {
        prepareError.value = res.value instanceof ApiError ? res.value.message : '自动准备失败'
        return
      }
      if (attempt === PREPARE_MAX_ATTEMPTS) {
        prepareError.value = '服务端正忙（正在为其他版本下载），请稍后重试'
        return
      }
      await sleep(PREPARE_RETRY_MS)
    }
  } finally {
    if (seq === statusSeq) {
      autoPreparing.value = false
      prepareProgress.value = null
    }
  }
}

/** 准备失败 / 额度用尽后手动重试（重新查询状态并再次自动准备） */
function retryPrepare() {
  void loadStatus()
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
  versionOpen.value = false
  if (id === selectedId.value) return
  selectedId.value = id
  classQuery.page = 1
  closeAllTabs()
  router.replace({ query: { ...route.query, version: id } })
}

function onSearchClasses() {
  classQuery.page = 1
  loadClasses()
}

function onPageChange(p: number) {
  classQuery.page = p
  loadClasses()
}

// ---- 标签页管理 ----
function ensureSourceState(className: string): SourceTabState {
  // 必须返回 reactive 代理（而不是新建的原始对象），否则后续赋值不触发重新渲染。
  if (!sourceStates[className]) {
    sourceStates[className] = { loading: false, error: '', text: '', tokens: [], reveal: null }
  }
  return sourceStates[className]
}

function ensureStructureState(className: string): StructureTabState {
  if (!structureStates[className]) {
    structureStates[className] = { loading: false, error: '', structure: null, hierarchy: null }
  }
  return structureStates[className]
}

/** 打开（或激活）一个标签；首次打开会触发对应的数据加载 */
function openTab(kind: McsrcTabKind, className: string) {
  if (!selectedId.value || !className) return
  const key = `${kind}:${className}`
  if (!tabs.value.some((t) => t.key === key)) {
    tabs.value.push({ key, kind, className })
  }
  activeKey.value = key
  if (kind === 'source') void loadSource(className)
  else void loadStructure(className)
}

function closeTab(key: string) {
  const index = tabs.value.findIndex((t) => t.key === key)
  if (index < 0) return
  tabs.value.splice(index, 1)
  if (activeKey.value !== key) return
  // 关闭当前标签后激活右邻（没有则左邻），与编辑器习惯一致。
  const next = tabs.value[index] ?? tabs.value[index - 1] ?? null
  activeKey.value = next?.key ?? ''
}

function closeAllTabs() {
  tabs.value = []
  activeKey.value = ''
  Object.keys(sourceStates).forEach((k) => delete sourceStates[k])
  Object.keys(structureStates).forEach((k) => delete structureStates[k])
}

// ---- 源码标签：浏览器内 Vineflower/WASM 反编译（无需部署侧 JVM）----
async function loadSource(className: string) {
  if (!selectedId.value) return
  const version = selectedId.value
  const targetSide = side.value
  const state = ensureSourceState(className)
  if (state.text || state.loading) return
  state.loading = true
  state.error = ''
  try {
    const res = await decompileClass(version, targetSide, className)
    // 期间可能已切换版本 / 侧别，丢弃过期结果。
    if (selectedId.value !== version || side.value !== targetSide) return
    state.text = res.source
    state.tokens = res.tokens
  } catch (err) {
    if (selectedId.value !== version || side.value !== targetSide) return
    state.error = err instanceof ApiError ? err.message : '反编译失败'
  } finally {
    state.loading = false
  }
}

/** 反编译失败后重试 */
function retrySource(className: string) {
  const state = ensureSourceState(className)
  state.error = ''
  state.text = ''
  state.tokens = []
  void loadSource(className)
}

/**
 * 源码内跳转：类 token → 打开目标类；字段 / 方法 token → 先沿继承链定位声明类再打开。
 * 目标就是当前标签的类时不重新反编译，仅把新的定位目标交给查看器。
 */
async function onSourceJump(target: McsrcJumpTarget) {
  if (!selectedId.value) return
  const resolved = await resolveDeclaration(selectedId.value, side.value, {
    className: target.className,
    type: target.reveal?.type ?? 'class',
    name: target.reveal?.name,
    descriptor: target.reveal?.descriptor,
  })
  const reveal = resolved.reveal ? { ...resolved.reveal } : null
  const current = activeTab.value
  if (current?.kind === 'source' && current.className === resolved.className) {
    // 换新对象以触发查看器的 reveal 侦听（同一目标连续点击时也要重新定位）。
    ensureSourceState(resolved.className).reveal = reveal
    return
  }
  openTab('source', resolved.className)
  ensureSourceState(resolved.className).reveal = reveal
}

// ---- 结构标签：并行拉取结构视图与继承关系（继承关系失败不阻塞结构展示）----
async function loadStructure(className: string) {
  if (!selectedId.value) return
  const version = selectedId.value
  const targetSide = side.value
  const state = ensureStructureState(className)
  if (state.structure || state.loading) return
  state.loading = true
  state.error = ''
  const [detailRes, hierRes] = await Promise.allSettled([
    getClassDetail(version, className, targetSide),
    getClassHierarchy(version, className, targetSide),
  ])
  if (selectedId.value !== version || side.value !== targetSide) return
  if (detailRes.status === 'fulfilled') {
    state.structure = detailRes.value
  } else {
    state.error = detailRes.reason instanceof ApiError ? detailRes.reason.message : '加载类结构失败'
  }
  if (hierRes.status === 'fulfilled') {
    state.hierarchy = hierRes.value
  }
  state.loading = false
}

/** 结构加载失败后重试 */
function retryStructure(className: string) {
  const state = ensureStructureState(className)
  state.error = ''
  state.structure = null
  state.hierarchy = null
  void loadStructure(className)
}

// 版本列表筛选变化时重新拉取（保留选中版本）。
watch(
  () => [listFilters.type, listFilters.q],
  () => {
    loadVersions()
  },
)

// 切换选中版本或侧别：旧标签的源码 / 结构都属于上一个版本，必须整体清空。
watch([selectedId, side], () => {
  classQuery.page = 1
  closeAllTabs()
  loadStatus()
})

onMounted(async () => {
  // 版本号可能来自 URL 查询参数：此时 watch 不会触发，需显式加载一次；
  // 否则由 watch(selectedId) 在填入默认版本（最新正式版）后统一触发，
  // 避免挂载阶段重复发起状态查询与自动准备。
  const fromQuery = !!selectedId.value
  await loadVersions()
  if (fromQuery) await loadStatus()
})
</script>

<template>
  <PageContainer max-width="max-w-[1600px]">
    <Breadcrumbs :items="[{ label: '首页', to: '/' }, { label: '工具中心', to: '/tools' }, { label: 'MC 源码浏览器' }]" />

    <div class="mt-4 mb-3 flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <h1 class="text-xl font-bold text-text-primary">MC 源码浏览器</h1>
        <p class="mt-1 text-xs text-text-secondary">
          按版本浏览 Minecraft 官方 jar：类索引由服务端缓存，源码在浏览器内反编译（Vineflower WASM，无需服务端 JVM）。
        </p>
      </div>
      <ShareButton />
    </div>

    <!-- 工作区：定高，内部各自滚动（VS Code 式布局） -->
    <div
      class="flex h-[calc(100vh-10rem)] min-h-[560px] flex-col overflow-hidden rounded-lg border border-border-dark bg-ink-900 lg:flex-row"
    >
      <!-- ============ 左侧栏 ============ -->
      <aside class="flex max-h-[45vh] w-full shrink-0 flex-col border-b border-border-dark lg:max-h-none lg:w-72 lg:border-r lg:border-b-0">
        <!-- 版本选择 -->
        <div class="relative border-b border-border-dark p-2">
          <button
            type="button"
            data-test="mcsrc-version-picker"
            class="flex w-full items-center justify-between gap-2 rounded-md border border-border-dark bg-ink-800 px-2.5 py-1.5 text-left transition hover:bg-ink-700"
            @click="versionOpen = !versionOpen"
          >
            <span class="truncate font-mono text-xs text-text-primary">{{ selectedId || '选择版本' }}</span>
            <span class="shrink-0 text-[10px] text-text-muted">▾</span>
          </button>

          <template v-if="versionOpen">
            <!-- 点击浮层外任意处收起 -->
            <div class="fixed inset-0 z-20" @click="versionOpen = false" />
            <div
              data-test="mcsrc-version-menu"
              class="absolute top-full right-2 left-2 z-30 mt-1 rounded-md border border-border-dark bg-ink-900 p-2 shadow-2xl"
            >
              <input
                v-model="listFilters.q"
                type="search"
                aria-label="搜索版本"
                placeholder="搜索版本号，如 1.21 / 24w"
                class="h-8 w-full rounded border border-border-dark bg-ink-950 px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
              />
              <div class="mt-2 flex flex-wrap gap-1">
                <button
                  v-for="opt in typeOptions"
                  :key="opt.value"
                  type="button"
                  class="rounded-full border px-2 py-0.5 text-[10px] transition"
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
              <div v-if="listLoading" role="status" aria-live="polite" class="mt-2 px-1 py-2 text-xs text-text-muted">
                加载版本列表中…
              </div>
              <p v-else-if="listError" role="alert" class="mt-2 px-1 py-2 text-xs text-danger-mc">{{ listError }}</p>
              <p v-else-if="!versions.length" class="mt-2 px-1 py-2 text-xs text-text-muted">没有匹配的版本</p>
              <ul v-else class="mt-2 max-h-64 divide-y divide-border-dark overflow-y-auto">
                <li v-for="v in versions" :key="v.id">
                  <button
                    type="button"
                    class="flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left transition hover:bg-ink-700"
                    :class="{ 'bg-ink-700': v.id === selectedId }"
                    @click="selectVersion(v.id)"
                  >
                    <span class="font-mono text-xs text-text-primary">{{ v.id }}</span>
                    <span class="shrink-0 text-[10px] text-text-muted">{{ VERSION_TYPE_LABELS[v.type] ?? v.type }}</span>
                  </button>
                </li>
              </ul>
            </div>
          </template>
        </div>

        <!-- 侧别 + 状态 -->
        <div class="border-b border-border-dark p-2">
          <div class="flex overflow-hidden rounded-md border border-border-dark">
            <button
              v-for="s in MCSRC_SIDES"
              :key="s"
              type="button"
              class="flex-1 px-2 py-1 text-[11px] transition"
              :class="side === s ? 'bg-grass-600 text-white' : 'bg-ink-800 text-text-secondary hover:bg-ink-700'"
              @click="side = s"
            >
              {{ MCSRC_SIDE_LABELS[s] }}
            </button>
          </div>

          <div class="mt-2 flex flex-wrap items-center gap-2">
            <span
              v-if="status"
              class="rounded-full border px-2 py-0.5 text-[10px]"
              :class="
                isReady || autoPreparing
                  ? 'border-grass-500/40 bg-grass-500/10 text-grass-300'
                  : status.cached
                    ? 'border-amber-mc/40 bg-amber-mc/10 text-amber-mc'
                    : 'border-border-dark bg-ink-700 text-text-muted'
              "
            >
              {{ statusLabel }}
            </span>
            <span v-if="status && isReady" class="text-[10px] text-text-muted">{{ status.class_count }} 类</span>
            <span v-if="status?.cached" class="text-[10px] text-text-muted">{{ formatSize(status.size) }}</span>
            <button
              v-if="status && !isReady && !autoPreparing"
              type="button"
              class="ml-auto rounded border border-grass-500/40 bg-grass-500/10 px-2 py-0.5 text-[10px] text-grass-300 transition hover:bg-grass-500/20"
              @click="retryPrepare"
            >
              {{ prepareError ? '重试准备' : '准备该版本' }}
            </button>
          </div>

          <p v-if="statusError" role="alert" class="mt-2 text-[11px] text-danger-mc">{{ statusError }}</p>
          <p v-else-if="statusLoading" role="status" aria-live="polite" class="mt-2 text-[11px] text-text-muted">
            查询版本状态中…
          </p>

          <!-- 自动准备进度：选中版本即自动下载 jar + 建索引，无需手动点击 -->
          <div v-if="autoPreparing" class="mt-2" data-test="mcsrc-prepare-progress">
            <div class="flex items-center justify-between gap-2 text-[10px] text-text-secondary">
              <span role="status" aria-live="polite" class="truncate">{{ progressLabel }}</span>
              <span v-if="progressPercent !== null" class="shrink-0 font-mono text-text-muted">{{ progressPercent }}%</span>
            </div>
            <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-ink-700">
              <div
                v-if="progressPercent !== null"
                class="h-full rounded-full bg-grass-500 transition-[width] duration-300"
                :style="{ width: `${progressPercent}%` }"
                role="progressbar"
                aria-label="下载进度"
                :aria-valuenow="progressPercent"
                aria-valuemin="0"
                aria-valuemax="100"
              />
              <div v-else class="h-full w-1/3 animate-pulse rounded-full bg-grass-500/70" role="progressbar" aria-label="准备进度" />
            </div>
          </div>

          <p
            v-else-if="prepareError"
            role="alert"
            class="mt-2 rounded border border-danger-mc/40 bg-danger-mc/10 px-2 py-1 text-[10px] text-danger-mc"
            data-test="mcsrc-prepare-error"
          >
            {{ prepareError }}
          </p>

          <p v-if="status?.truncated" class="mt-2 text-[10px] text-amber-mc">
            类数量超出索引上限，索引已截断。
          </p>

          <div v-if="status && isReady" class="mt-2 grid grid-cols-2 gap-x-2 gap-y-0.5 text-[10px] text-text-muted">
            <span class="truncate">抓取 {{ status.fetched_at ? formatDate(status.fetched_at) : '—' }}</span>
            <span class="truncate">索引 {{ status.index_built_at ? formatDate(status.index_built_at) : '—' }}</span>
          </div>
        </div>

        <!-- 类检索 -->
        <div class="border-b border-border-dark p-2">
          <input
            v-model="classQuery.q"
            type="search"
            aria-label="搜索类名"
            placeholder="搜索类名，如 MinecraftServer"
            class="h-8 w-full rounded border border-border-dark bg-ink-800 px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            @change="onSearchClasses"
          />
          <input
            v-model="classQuery.pkg"
            type="search"
            aria-label="按包名过滤"
            placeholder="包名前缀，如 net.minecraft.server"
            class="mt-1.5 h-8 w-full rounded border border-border-dark bg-ink-800 px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            @change="onSearchClasses"
          />
          <label class="mt-1.5 flex items-center gap-1.5 text-[11px] text-text-secondary">
            <input v-model="classQuery.includeInner" type="checkbox" class="h-3.5 w-3.5 accent-grass-600" @change="onSearchClasses" />
            含内部类
          </label>
        </div>

        <!-- 类列表（滚动区） -->
        <div class="min-h-0 flex-1 overflow-y-auto">
          <p v-if="!selectedId" class="p-3 text-xs text-text-muted">请选择版本。</p>
          <div v-else-if="statusLoading" role="status" aria-live="polite" class="p-3 text-xs text-text-muted">
            查询版本状态中…
          </div>
          <p v-else-if="statusError" role="alert" class="p-3 text-xs text-danger-mc">{{ statusError }}</p>
          <div v-else-if="classesLoading" role="status" aria-live="polite" class="p-3 text-xs text-text-muted">检索中…</div>
          <ErrorState v-else-if="classesError" class="p-3" :message="classesError" retryable @retry="loadClasses" />
          <div v-else-if="autoPreparing" role="status" aria-live="polite" class="p-3 text-xs text-text-muted">
            正在准备该版本，完成后会自动显示类列表…
          </div>
          <EmptyState
            v-else-if="needPrepare || !isReady"
            class="p-3"
            title="尚未建立类索引"
            description="该版本还没有类索引，可点击「准备该版本」重试，或稍后再访问。"
          />
          <p v-else-if="!classes.length" class="p-3 text-xs text-text-muted">没有匹配的类。</p>
          <ul v-else data-test="mcsrc-class-list" class="divide-y divide-border-dark">
            <li
              v-for="c in classes"
              :key="c.name"
              data-test="mcsrc-class-row"
              class="group flex items-center gap-1.5 px-2 py-1.5 transition hover:bg-ink-800/70"
            >
              <button type="button" class="min-w-0 flex-1 text-left" :title="c.name" @click="openTab('source', c.name)">
                <span
                  class="block truncate font-mono text-xs"
                  :class="activeTab?.className === c.name ? 'text-grass-300' : 'text-text-primary'"
                >
                  {{ c.simple }}
                </span>
                <span class="block truncate text-[10px] text-text-muted">{{ c.pkg || '（默认包）' }}</span>
              </button>
              <span v-if="c.inner" class="shrink-0 rounded bg-ink-700 px-1 py-0.5 text-[9px] text-text-muted">内部类</span>
              <span class="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  class="rounded border border-border-dark px-1.5 py-0.5 text-[10px] text-text-secondary transition hover:bg-ink-700"
                  @click="openTab('structure', c.name)"
                >
                  查看结构
                </button>
                <button
                  type="button"
                  class="rounded border border-border-dark px-1.5 py-0.5 text-[10px] text-grass-300 transition hover:bg-ink-700"
                  @click="openTab('source', c.name)"
                >
                  查看源码
                </button>
              </span>
            </li>
          </ul>
        </div>

        <!-- 分页 -->
        <div v-if="isReady && classTotal > 0" class="border-t border-border-dark p-2">
          <Pagination
            :page="classQuery.page"
            :page-size="classQuery.pageSize"
            :total="classTotal"
            :total-pages="classTotalPages"
            @change="onPageChange"
          />
        </div>
      </aside>

      <!-- ============ 主区 ============ -->
      <section class="flex min-h-0 min-w-0 flex-1 flex-col">
        <!-- 标签栏 -->
        <div class="flex h-9 shrink-0 items-stretch overflow-x-auto border-b border-border-dark bg-ink-950">
          <div
            v-for="tab in tabs"
            :key="tab.key"
            class="group flex shrink-0 items-center gap-1 border-r border-border-dark px-2.5 text-xs transition"
            :class="tab.key === activeKey ? 'bg-ink-900 text-text-primary' : 'text-text-muted hover:bg-ink-800'"
          >
            <button type="button" class="max-w-[180px] truncate font-mono" :title="tab.className" @click="activeKey = tab.key">
              {{ tabLabel(tab) }}
            </button>
            <button
              type="button"
              data-test="mcsrc-tab-close"
              class="rounded px-1 text-text-muted transition hover:bg-ink-700 hover:text-text-primary"
              :aria-label="`关闭 ${tabLabel(tab)}`"
              @click="closeTab(tab.key)"
            >
              ×
            </button>
          </div>
          <span v-if="!tabs.length" class="px-3 py-2 text-[11px] text-text-muted">未打开文件</span>
        </div>

        <!-- 面包屑（当前文件路径） -->
        <div
          v-if="activeTab"
          class="flex h-8 shrink-0 items-center gap-1 overflow-hidden border-b border-border-dark px-3 font-mono text-[11px] whitespace-nowrap"
        >
          <template v-for="(part, i) in breadcrumbParts" :key="`${part}-${i}`">
            <span v-if="i < breadcrumbParts.length - 1" class="text-text-muted">{{ part }}</span>
            <span v-else class="text-text-primary">{{ part }}</span>
            <span v-if="i < breadcrumbParts.length - 1" class="text-text-muted">›</span>
          </template>
          <span v-if="activeTab.kind === 'structure'" class="ml-2 text-text-muted">（结构视图）</span>
        </div>

        <!-- 内容区 -->
        <div class="min-h-0 flex-1 overflow-hidden">
          <!-- 空态：未打开任何标签 -->
          <EmptyState
            v-if="!activeTab"
            class="p-8"
            title="未打开文件"
            description="从左侧列表选择类名即可打开源码；点「查看结构」可查看类结构与继承关系。"
          />

          <!-- 源码标签 -->
          <div v-else-if="activeTab.kind === 'source'" data-test="class-source" class="flex h-full flex-col">
            <div class="flex shrink-0 items-center justify-between gap-2 border-b border-border-dark px-3 py-1">
              <span class="truncate font-mono text-xs text-text-primary">{{ activeTab.className }}</span>
              <div class="flex shrink-0 items-center gap-2">
                <span v-if="activeSource?.error" class="text-[10px] text-danger-mc">反编译失败</span>
                <button
                  v-if="activeSource?.error"
                  type="button"
                  class="rounded border border-border-dark px-2 py-0.5 text-[10px] text-grass-300 transition hover:bg-ink-700"
                  @click="retrySource(activeTab.className)"
                >
                  重新反编译
                </button>
                <CopyButton tool="mcsrc" :text="activeTab.className" />
              </div>
            </div>
            <p class="shrink-0 px-3 py-1 text-[10px] text-text-muted" data-test="mcsrc-source-hint">
              Ctrl/⌘ + 点击（或 F12）类名与成员名可跳转：类跳类、字段与方法先定位到真正的声明类。
            </p>

            <!-- 关键：这一层必须 min-h-0 + flex-1，Monaco 才能拿到确定高度 -->
            <div class="min-h-0 flex-1 overflow-hidden">
              <div v-if="activeSource?.loading" role="status" aria-live="polite" class="p-3 text-sm text-text-muted">
                反编译中…（首次会加载反编译引擎与 jar，可能需要几秒）
              </div>
              <p v-else-if="activeSource?.error" role="alert" class="p-3 text-sm text-danger-mc">
                {{ activeSource.error }}
              </p>
              <SourceViewer
                v-else
                :source="activeSource?.text ?? ''"
                :tokens="activeSource?.tokens ?? []"
                :reveal="activeSource?.reveal ?? null"
                language="java"
                class="h-full w-full"
                @jump="onSourceJump"
              />
            </div>
          </div>

          <!-- 结构标签：结构视图 + 继承关系（均由后端解析 class 字节码得出） -->
          <div v-else data-test="class-detail" class="h-full space-y-4 overflow-y-auto p-4">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-mono text-sm text-text-primary">{{ activeTab.className }}</span>
                  <span v-if="activeStructure?.structure" class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted">
                    {{ MCSRC_KIND_LABELS[activeStructure.structure.kind] }}
                  </span>
                  <span v-if="activeStructure?.structure" class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted">
                    {{ javaVersionLabel(activeStructure.structure.major) }}
                  </span>
                  <span v-if="activeStructure?.structure?.inner" class="rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted">
                    内部类
                  </span>
                </div>
                <p v-if="activeStructure?.structure" class="mt-1 break-all font-mono text-xs text-text-muted">
                  {{ [...activeStructure.structure.modifiers, activeStructure.structure.kind, activeStructure.structure.simple].join(' ') }}
                </p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <button
                  type="button"
                  class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-grass-300 transition hover:bg-ink-700"
                  @click="openTab('source', activeTab.className)"
                >
                  查看源码
                </button>
                <CopyButton tool="mcsrc" :text="activeTab.className" />
              </div>
            </div>

            <div v-if="activeStructure?.loading" role="status" aria-live="polite" class="text-sm text-text-muted">
              加载类结构中…
            </div>
            <div v-else-if="activeStructure?.error" class="space-y-2">
              <p role="alert" class="text-sm text-danger-mc">{{ activeStructure.error }}</p>
              <button
                type="button"
                class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700"
                @click="retryStructure(activeTab.className)"
              >
                重试
              </button>
            </div>
            <template v-else-if="activeStructure?.structure">
              <div class="grid gap-3 text-sm sm:grid-cols-2">
                <div>
                  <div class="text-xs text-text-muted">父类</div>
                  <div class="mt-1 flex flex-wrap items-center gap-2">
                    <button
                      v-if="activeStructure.structure.in_jar_super"
                      type="button"
                      class="font-mono text-xs text-grass-300 hover:underline"
                      @click="openTab('structure', activeStructure.structure.super)"
                    >
                      {{ activeStructure.structure.super }}
                    </button>
                    <span v-else class="break-all font-mono text-xs text-text-secondary">
                      {{ activeStructure.structure.super || '—' }}
                    </span>
                    <span v-if="activeStructure.structure.super && !activeStructure.structure.in_jar_super" class="text-[10px] text-text-muted">
                      jar 外
                    </span>
                  </div>
                </div>
                <div>
                  <div class="text-xs text-text-muted">直接接口</div>
                  <p v-if="!activeStructure.structure.interfaces.length" class="mt-1 text-xs text-text-muted">—</p>
                  <ul v-else class="mt-1 flex flex-wrap gap-x-3 gap-y-1">
                    <li v-for="iface in activeStructure.structure.interfaces" :key="iface">
                      <button
                        v-if="activeStructure.structure.in_jar_interfaces.includes(iface)"
                        type="button"
                        class="break-all font-mono text-xs text-grass-300 hover:underline"
                        @click="openTab('structure', iface)"
                      >
                        {{ iface }}
                      </button>
                      <span v-else class="break-all font-mono text-xs text-text-secondary">{{ iface }}</span>
                    </li>
                  </ul>
                </div>
              </div>

              <!-- 继承关系 -->
              <div
                v-if="activeStructure.hierarchy"
                data-test="class-hierarchy"
                class="rounded-md border border-border-dark bg-ink-900/60 p-3"
              >
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
                        @click="openTab('structure', a)"
                      >
                        {{ a }}
                      </button>
                      <span v-else class="break-all font-mono text-text-muted">{{ a }}</span>
                      <span class="text-text-muted">→</span>
                    </template>
                  </template>
                  <span class="break-all font-mono font-semibold text-text-primary">{{ activeStructure.hierarchy.name }}</span>
                  <span v-if="!activeStructure.hierarchy.ancestors_in_jar" class="text-[10px] text-text-muted">
                    （祖先链延伸至 jar 外）
                  </span>
                </div>
                <div class="mt-2 flex flex-wrap items-center gap-1 text-xs">
                  <span class="text-text-muted">向下：</span>
                  <span v-if="!activeStructure.hierarchy.descendant_count" class="text-text-muted">无直接子类</span>
                  <template v-else>
                    <span class="text-text-secondary">
                      直接子类 {{ activeStructure.hierarchy.descendant_count }} 个（含间接共
                      {{ activeStructure.hierarchy.total_descendants }} 个）
                    </span>
                    <ul class="mt-1 flex w-full flex-wrap gap-x-3 gap-y-1">
                      <li v-for="d in activeStructure.hierarchy.descendants" :key="d.name">
                        <button
                          type="button"
                          class="break-all font-mono text-xs text-grass-300 hover:underline"
                          @click="openTab('structure', d.name)"
                        >
                          {{ d.name }}
                        </button>
                      </li>
                    </ul>
                    <span v-if="activeStructure.hierarchy.truncated" class="w-full text-[10px] text-amber-mc">
                      子类过多，仅列出前 {{ activeStructure.hierarchy.descendants.length }} 个。
                    </span>
                  </template>
                </div>
              </div>

              <!-- 字段 -->
              <div>
                <h4 class="mb-2 text-xs font-semibold text-text-secondary">
                  字段（{{ activeStructure.structure.fields.length }}）
                </h4>
                <p v-if="!activeStructure.structure.fields.length" class="text-xs text-text-muted">无字段。</p>
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
                      <tr v-for="f in activeStructure.structure.fields" :key="`${f.name}:${f.descriptor}`">
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
        </div>

        <!-- 状态栏 -->
        <div
          class="flex h-7 shrink-0 items-center justify-between gap-3 border-t border-border-dark bg-ink-950 px-3 text-[10px] text-text-muted"
        >
          <span class="truncate">
            <template v-if="activeTab?.kind === 'source'">源码由 Vineflower（WASM）在浏览器内反编译，仅供阅读参考</template>
            <template v-else-if="activeTab">类结构由服务端解析 class 字节码得出，无需反编译</template>
            <template v-else>选择左侧的类即可打开源码或结构视图</template>
          </span>
          <span class="shrink-0 font-mono">
            {{ selectedId }} · {{ MCSRC_SIDE_LABELS[side] }}<template v-if="status?.indexed"> · {{ status.class_count }} 类</template>
          </span>
        </div>
      </section>
    </div>
  </PageContainer>
</template>
