<script setup lang="ts">
// 种子与结构工具：覆盖后端 /mc/* 全部端点。
//   1) 种子解析 / 随机生成
//   2) 结构理论候选坐标（按 region 网格）
//   3) 要塞估算（环形）
//   4) 史莱姆区块查询
//   5) 按「结构出现在目标坐标附近」搜索种子
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import {
  estimateStrongholds,
  findSeeds,
  generateSeeds,
  listCapabilities,
  listStructureTypes,
  parseSeed,
  queryBiome,
  querySlimeChunks,
  queryStructures,
} from '@/api/mc'
import { ApiError } from '@/types/api'
import {
  MC_STRUCTURE_LABELS,
  MC_STRUCTURE_TYPES,
  type BiomeResult,
  type CapabilitiesResult,
  type FindSeedsResult,
  type McStructureType,
  type SeedValue,
  type SlimeChunkResult,
  type StrongholdResult,
  type StructurePosition,
  type StructureQueryResult,
} from '@/types/mc'
import { SEED_VERSIONS, type SeedVersion } from '@/types/seed'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure, reportToolUse } from '@/utils/tool-stats'
import { buildSeedToolsQuery, readSeedToolsQuery } from '@/utils/seed-tools-query'

applyPageMeta({
  title: '种子与结构工具',
  description:
    'Minecraft 种子工具：种子解析与随机生成、结构理论坐标、要塞估算、史莱姆区块查询，以及按结构位置搜索种子。',
  canonicalPath: '/seed-tools',
})

useRecentToolsStore().record('seed-tools', '种子与结构工具', '查询类')

const route = useRoute()
const router = useRouter()

// TODO §14：整页表单状态与 URL 双向同步（编解码在 utils/seed-tools-query.ts）。
const initialState = readSeedToolsQuery(route.query)

/** 统一游戏版本区间（影响结构可用性与群系查询支持范围） */
const version = ref<SeedVersion>(initialState.version)

/** 结构类型优先取后端清单（保持同步），失败时回退到内置常量 */
const backendTypes = ref<string[]>([])

const structureOptions = computed(() => {
  const types = backendTypes.value.length ? backendTypes.value : [...MC_STRUCTURE_TYPES]
  return types.map((t) => ({
    value: t,
    label: MC_STRUCTURE_LABELS[t as McStructureType] ?? t,
  }))
})

/** 能力版本支持矩阵（后端不可用时为空，不展示） */
const capabilities = ref<CapabilitiesResult | null>(null)
const showCapabilities = ref(false)

onMounted(async () => {
  try {
    const res = await listStructureTypes()
    if (res.types?.length) backendTypes.value = res.types
  } catch {
    // 静默降级：使用内置结构类型清单
  }
  try {
    capabilities.value = await listCapabilities()
  } catch {
    // 静默降级：不展示版本支持矩阵
  }
})

/** 候选点的群系可行性标签 */
function feasibleLabel(p: StructurePosition): string {
  if (p.feasible === undefined) return '—'
  if (!p.feasible) return '群系不符'
  return p.shift_x || p.shift_z ? '群系可行（偏移）' : '群系可行'
}

function feasibleClass(p: StructurePosition): string {
  if (p.feasible === undefined) return 'text-text-muted'
  return p.feasible ? 'text-grass-300' : 'text-danger-mc'
}

/** 当前进行中的分区 key（'' 表示空闲） */
const busy = ref('')
const error = ref('')

async function run(key: string, fn: () => Promise<void>) {
  busy.value = key
  error.value = ''
  try {
    await fn()
    // TODO §13：核心动作成功执行 -> 记一次「使用」。
    reportToolUse('seed-tools')
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '操作失败，请稍后重试'
    // TODO §13：核心动作失败 -> 记一次「失败」（用于运营看板观察失败率）。
    reportToolFailure('seed-tools')
  } finally {
    busy.value = ''
  }
}

// ===== 1) 种子解析 / 生成 =====
const parseInput = ref(initialState.parseInput)
const parsedSeed = ref<SeedValue | null>(null)
const genCount = ref(initialState.genCount)
const generated = ref<number[]>([])

function onParse() {
  if (!parseInput.value.trim()) {
    error.value = '请输入要解析的种子'
    return
  }
  void run('parse', async () => {
    const res = await parseSeed({ seed: parseInput.value })
    parsedSeed.value = res.seed
  })
}

function onGenerate() {
  void run('generate', async () => {
    const res = await generateSeeds({ count: genCount.value })
    generated.value = res.seeds
  })
}

// ===== 2) 结构坐标 =====
const stForm = reactive({ ...initialState.structures })
const stResult = ref<StructureQueryResult | null>(null)

function onQueryStructures() {
  if (!stForm.seed.trim()) {
    error.value = '请输入世界种子'
    return
  }
  void run('structures', async () => {
    stResult.value = await queryStructures({ ...stForm, version: version.value })
  })
}

// ===== 2b) 群系查询（仅 1.18+）=====
const bioForm = reactive({ ...initialState.biome })
const bioResult = ref<BiomeResult | null>(null)

function onQueryBiome() {
  if (!bioForm.seed.trim()) {
    error.value = '请输入世界种子'
    return
  }
  void run('biome', async () => {
    bioResult.value = await queryBiome({ ...bioForm, version: version.value })
  })
}

// ===== 3) 要塞估算 =====
const shForm = reactive({ ...initialState.stronghold })
const shResult = ref<StrongholdResult | null>(null)

function onStrongholds() {
  if (!shForm.seed.trim()) {
    error.value = '请输入世界种子'
    return
  }
  void run('strongholds', async () => {
    shResult.value = await estimateStrongholds({ ...shForm })
  })
}

// ===== 4) 史莱姆区块 =====
const slForm = reactive({ ...initialState.slime })
const slResult = ref<SlimeChunkResult | null>(null)

function onSlimeChunks() {
  if (!slForm.seed.trim()) {
    error.value = '请输入世界种子'
    return
  }
  void run('slime', async () => {
    slResult.value = await querySlimeChunks({ ...slForm })
  })
}

// ===== 5) 种子搜索 =====
const fdForm = reactive({ ...initialState.find })
const fdResult = ref<FindSeedsResult | null>(null)

function onFindSeeds() {
  void run('find', async () => {
    fdResult.value = await findSeeds({ ...fdForm })
  })
}

/** 将整页状态写入 URL（省略默认值）。 */
function syncUrl() {
  router.replace({
    path: '/seed-tools',
    query: buildSeedToolsQuery({
      version: version.value,
      parseInput: parseInput.value,
      genCount: genCount.value,
      structures: stForm,
      biome: bioForm,
      stronghold: shForm,
      slime: slForm,
      find: fdForm,
    }),
  })
}

watch([version, parseInput, genCount, stForm, bioForm, shForm, slForm, fdForm], syncUrl)
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: '种子与结构工具' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">种子与结构工具</h1>
        <p class="text-text-secondary">
          种子解析与生成、结构理论坐标、要塞估算、史莱姆区块查询与种子搜索。坐标为理论候选/估算值，实际生成受生物群系影响。
        </p>
      </div>
      <ShareButton />
    </div>

    <div
      v-if="error"
      class="mb-6 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      {{ error }}
    </div>

    <!-- 版本选择 + 版本支持说明 -->
    <section class="card mb-6 p-5">
      <div class="flex flex-wrap items-end gap-4">
        <div>
          <label for="tool-version" class="mb-1.5 block text-sm font-medium text-text-primary">
            游戏版本区间
          </label>
          <select
            id="tool-version"
            v-model="version"
            class="h-10 rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          >
            <option v-for="v in SEED_VERSIONS" :key="v" :value="v">{{ v }}</option>
          </select>
        </div>
        <p class="flex-1 text-xs leading-relaxed text-text-muted">
          影响结构可用性与群系查询支持范围。1.12 及更早的结构分布算法与当前实现不同，结果不可作为依据。
        </p>
        <button
          v-if="capabilities"
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-3 py-2 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
          @click="showCapabilities = !showCapabilities"
        >
          {{ showCapabilities ? '收起' : '查看' }}版本支持矩阵
        </button>
      </div>

      <div v-if="capabilities && showCapabilities" class="mt-4 space-y-3">
        <div
          v-for="f in capabilities.features"
          :key="f.id"
          class="rounded-lg border border-border-dark bg-ink-900 p-3"
        >
          <div class="mb-2 flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-text-primary">{{ f.name }}</span>
            <span
              v-for="s in f.supported"
              :key="s.version"
              class="rounded px-1.5 py-0.5 text-xs"
              :class="s.supported ? 'bg-grass-500/15 text-grass-300' : 'bg-danger-mc/15 text-danger-mc'"
              :title="s.note"
            >
              {{ s.version }}：{{ s.supported ? '支持' : '不支持' }}
            </span>
          </div>
          <p class="text-xs leading-relaxed text-text-muted">{{ f.note }}</p>
        </div>
      </div>
    </section>

    <div class="space-y-6">
      <!-- 1) 种子解析 / 生成 -->
      <section class="card p-5">
        <h2 class="mb-4 text-lg font-bold text-text-primary">种子解析与生成</h2>
        <div class="grid gap-6 md:grid-cols-2">
          <form @submit.prevent="onParse">
            <label for="parse-seed" class="mb-1.5 block text-sm font-medium text-text-primary">
              解析种子（数字或文本）
            </label>
            <div class="flex gap-2">
              <input
                id="parse-seed"
                v-model="parseInput"
                type="text"
                placeholder="例如：my world"
                class="h-10 min-w-0 flex-1 rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
              />
              <button
                type="submit"
                :disabled="busy === 'parse'"
                class="rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
              >
                解析
              </button>
            </div>
            <p v-if="parsedSeed !== null" class="mt-2 flex items-center gap-2 text-sm text-text-secondary">
              数值种子：<code class="code-block px-2 py-0.5 text-grass-300">{{ parsedSeed }}</code>
              <CopyButton tool="seed-tools" :text="String(parsedSeed)" label="复制" />
            </p>
          </form>

          <form @submit.prevent="onGenerate">
            <label for="gen-count" class="mb-1.5 block text-sm font-medium text-text-primary">
              随机生成种子（1–100 个）
            </label>
            <div class="flex gap-2">
              <input
                id="gen-count"
                v-model.number="genCount"
                type="number"
                min="1"
                max="100"
                class="h-10 w-24 rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
              />
              <button
                type="submit"
                :disabled="busy === 'generate'"
                class="rounded-md border border-border-light bg-ink-700 px-4 text-sm text-text-secondary transition hover:bg-ink-600 disabled:opacity-60"
              >
                生成
              </button>
            </div>
            <ul v-if="generated.length" class="mt-2 space-y-1">
              <li v-for="s in generated" :key="s" class="flex items-center gap-2 text-sm">
                <code class="code-block min-w-0 flex-1 px-2 py-0.5 text-text-secondary">{{ s }}</code>
                <CopyButton tool="seed-tools" :text="String(s)" label="复制" />
              </li>
            </ul>
          </form>
        </div>
      </section>

      <!-- 2) 结构坐标 -->
      <section class="card p-5">
        <h2 class="mb-4 text-lg font-bold text-text-primary">结构理论候选坐标</h2>
        <form class="grid gap-3 md:grid-cols-6" @submit.prevent="onQueryStructures">
          <div class="md:col-span-2">
            <label for="st-seed" class="mb-1.5 block text-sm font-medium text-text-primary">世界种子</label>
            <input
              id="st-seed"
              v-model="stForm.seed"
              type="text"
              placeholder="例如：-1234567890 或 my world"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </div>
          <div class="md:col-span-2">
            <label for="st-type" class="mb-1.5 block text-sm font-medium text-text-primary">结构类型</label>
            <select
              id="st-type"
              v-model="stForm.type"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option v-for="o in structureOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
          <div>
            <label for="st-rx" class="mb-1.5 block text-sm font-medium text-text-primary">region X</label>
            <input
              id="st-rx"
              v-model.number="stForm.region_x"
              type="number"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </div>
          <div>
            <label for="st-rz" class="mb-1.5 block text-sm font-medium text-text-primary">region Z</label>
            <input
              id="st-rz"
              v-model.number="stForm.region_z"
              type="number"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </div>
          <div class="flex items-end">
            <button
              type="submit"
              :disabled="busy === 'structures'"
              class="h-10 w-full rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
            >
              查询
            </button>
          </div>
        </form>
        <p class="mt-2 text-xs text-text-muted">region 为结构网格单元索引（非区块索引），跨度最大 50×50。</p>

        <div v-if="stResult" class="mt-4">
          <div
            v-if="stResult.supported === false"
            class="mb-3 rounded-md border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc"
            role="alert"
          >
            该组合在当前版本不可靠，请参考下方说明。
          </div>
          <p class="mb-2 flex flex-wrap items-center gap-3 text-sm text-text-secondary">
            <span>{{ stResult.type }} · 共 {{ stResult.positions.length }} 个候选点</span>
            <span v-if="stResult.dimension" class="text-xs text-text-muted">维度：{{ stResult.dimension }}</span>
            <span v-if="stResult.probability !== undefined && stResult.probability < 1" class="text-xs text-text-muted">
              生成尝试频率上限：{{ (stResult.probability * 100).toFixed(1) }}%
            </span>
          </p>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="border-b border-border-dark text-xs text-text-muted">
                <tr>
                  <th scope="col" class="px-3 py-2 font-medium">X</th>
                  <th scope="col" class="px-3 py-2 font-medium">Z</th>
                  <th scope="col" class="px-3 py-2 font-medium">地表群系</th>
                  <th scope="col" class="px-3 py-2 font-medium">可行性</th>
                  <th scope="col" class="px-3 py-2 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border-dark">
                <tr v-for="(p, i) in stResult.positions" :key="i">
                  <td class="px-3 py-2 font-mono text-text-secondary">{{ p.x }}</td>
                  <td class="px-3 py-2 font-mono text-text-secondary">{{ p.z }}</td>
                  <td class="px-3 py-2 text-text-secondary">{{ p.biome || '—' }}</td>
                  <td class="px-3 py-2" :class="feasibleClass(p)">
                    {{ feasibleLabel(p) }}
                    <span v-if="p.shift_x || p.shift_z" class="ml-1 text-xs text-text-muted">
                      ({{ p.shift_x }}, {{ p.shift_z }})
                    </span>
                  </td>
                  <td class="px-3 py-2 text-right"><CopyButton tool="seed-tools" :text="`${p.x} ${p.z}`" label="复制" /></td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-if="stResult.note" class="mt-2 text-xs text-text-muted">{{ stResult.note }}</p>
        </div>
      </section>

      <!-- 2b) 群系查询 -->
      <section class="card p-5">
        <h2 class="mb-4 text-lg font-bold text-text-primary">生物群系查询</h2>
        <form class="grid gap-3 md:grid-cols-4" @submit.prevent="onQueryBiome">
          <div>
            <label for="bio-seed" class="mb-1.5 block text-sm font-medium text-text-primary">世界种子</label>
            <input
              id="bio-seed"
              v-model="bioForm.seed"
              type="text"
              placeholder="例如：-1234567890"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </div>
          <div>
            <label for="bio-x" class="mb-1.5 block text-sm font-medium text-text-primary">X</label>
            <input id="bio-x" v-model.number="bioForm.x" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div>
            <label for="bio-z" class="mb-1.5 block text-sm font-medium text-text-primary">Z</label>
            <input id="bio-z" v-model.number="bioForm.z" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div class="flex items-end">
            <button
              type="submit"
              :disabled="busy === 'biome'"
              class="h-10 w-full rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
            >
              查询群系
            </button>
          </div>
        </form>
        <p class="mt-2 text-xs text-text-muted">仅 1.18+ 支持（服务端为 1.18+ 多噪声引擎）；按海平面附近地表采样。</p>

        <div v-if="bioResult" class="mt-4">
          <p v-if="bioResult.supported" class="text-sm text-text-secondary">
            坐标 ({{ bioResult.x }}, {{ bioResult.z }}) 的地表群系：
            <code class="code-block px-2 py-0.5 text-grass-300">{{ bioResult.biome }}</code>
          </p>
          <p v-else class="rounded-md border border-border-dark bg-ink-900 px-4 py-2 text-sm text-text-muted">
            {{ bioResult.note || '当前版本不支持群系查询。' }}
          </p>
        </div>
      </section>

      <!-- 3) 要塞估算 -->
      <section class="card p-5">
        <h2 class="mb-4 text-lg font-bold text-text-primary">要塞估算</h2>
        <form class="flex flex-wrap items-end gap-3" @submit.prevent="onStrongholds">
          <div class="min-w-56 flex-1">
            <label for="sh-seed" class="mb-1.5 block text-sm font-medium text-text-primary">世界种子</label>
            <input
              id="sh-seed"
              v-model="shForm.seed"
              type="text"
              placeholder="例如：-1234567890"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </div>
          <div>
            <label for="sh-count" class="mb-1.5 block text-sm font-medium text-text-primary">数量</label>
            <input
              id="sh-count"
              v-model.number="shForm.count"
              type="number"
              min="1"
              max="128"
              class="h-10 w-24 rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </div>
          <button
            type="submit"
            :disabled="busy === 'strongholds'"
            class="h-10 rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
          >
            估算
          </button>
        </form>

        <div v-if="shResult" class="mt-4 overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark text-xs text-text-muted">
              <tr><th scope="col" class="px-3 py-2 font-medium">序号</th><th scope="col" class="px-3 py-2 font-medium">X</th><th scope="col" class="px-3 py-2 font-medium">Z</th><th scope="col" class="px-3 py-2 text-right font-medium">操作</th></tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="(p, i) in shResult.positions" :key="i">
                <td class="px-3 py-2 text-text-muted">{{ i + 1 }}</td>
                <td class="px-3 py-2 font-mono text-text-secondary">{{ p.x }}</td>
                <td class="px-3 py-2 font-mono text-text-secondary">{{ p.z }}</td>
                <td class="px-3 py-2 text-right"><CopyButton tool="seed-tools" :text="`${p.x} ${p.z}`" label="复制" /></td>
              </tr>
            </tbody>
          </table>
          <p v-if="shResult.note" class="mt-2 text-xs text-text-muted">{{ shResult.note }}</p>
        </div>
      </section>

      <!-- 4) 史莱姆区块 -->
      <section class="card p-5">
        <h2 class="mb-4 text-lg font-bold text-text-primary">史莱姆区块查询</h2>
        <form class="grid gap-3 md:grid-cols-5" @submit.prevent="onSlimeChunks">
          <div class="md:col-span-1">
            <label for="sl-seed" class="mb-1.5 block text-sm font-medium text-text-primary">世界种子</label>
            <input
              id="sl-seed"
              v-model="slForm.seed"
              type="text"
              placeholder="例如：-1234567890"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </div>
          <div>
            <label for="sl-minx" class="mb-1.5 block text-sm font-medium text-text-primary">min 区块 X</label>
            <input id="sl-minx" v-model.number="slForm.min_chunk_x" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div>
            <label for="sl-minz" class="mb-1.5 block text-sm font-medium text-text-primary">min 区块 Z</label>
            <input id="sl-minz" v-model.number="slForm.min_chunk_z" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div>
            <label for="sl-maxx" class="mb-1.5 block text-sm font-medium text-text-primary">max 区块 X</label>
            <input id="sl-maxx" v-model.number="slForm.max_chunk_x" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div>
            <label for="sl-maxz" class="mb-1.5 block text-sm font-medium text-text-primary">max 区块 Z</label>
            <input id="sl-maxz" v-model.number="slForm.max_chunk_z" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div class="md:col-span-5">
            <button
              type="submit"
              :disabled="busy === 'slime'"
              class="h-10 rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
            >
              查询史莱姆区块
            </button>
            <span class="ml-3 text-xs text-text-muted">区块跨度最大 128</span>
          </div>
        </form>

        <div v-if="slResult" class="mt-4">
          <p class="mb-2 text-sm text-text-secondary">共 {{ slResult.count }} 个史莱姆区块</p>
          <div v-if="slResult.chunks.length" class="max-h-72 overflow-auto">
            <table class="w-full text-left text-sm">
              <thead class="sticky top-0 border-b border-border-dark bg-ink-800 text-xs text-text-muted">
                <tr><th scope="col" class="px-3 py-2 font-medium">区块</th><th scope="col" class="px-3 py-2 font-medium">中心方块坐标</th><th scope="col" class="px-3 py-2 text-right font-medium">操作</th></tr>
              </thead>
              <tbody class="divide-y divide-border-dark">
                <tr v-for="c in slResult.chunks" :key="`${c.chunk_x},${c.chunk_z}`">
                  <td class="px-3 py-2 font-mono text-text-secondary">{{ c.chunk_x }}, {{ c.chunk_z }}</td>
                  <td class="px-3 py-2 font-mono text-text-muted">{{ c.x }} {{ c.z }}</td>
                  <td class="px-3 py-2 text-right"><CopyButton tool="seed-tools" :text="`${c.chunk_x} ${c.chunk_z}`" label="复制" /></td>
                </tr>
              </tbody>
            </table>
          </div>
          <EmptyState v-else title="该范围内没有史莱姆区块" description="换一个范围或种子再试。" />
          <p v-if="slResult.note" class="mt-2 text-xs text-text-muted">{{ slResult.note }}</p>
        </div>
      </section>

      <!-- 5) 种子搜索 -->
      <section class="card p-5">
        <h2 class="mb-4 text-lg font-bold text-text-primary">按结构位置搜索种子</h2>
        <form class="grid gap-3 md:grid-cols-6" @submit.prevent="onFindSeeds">
          <div class="md:col-span-2">
            <label for="fd-type" class="mb-1.5 block text-sm font-medium text-text-primary">目标结构</label>
            <select
              id="fd-type"
              v-model="fdForm.type"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option v-for="o in structureOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
          <div>
            <label for="fd-x" class="mb-1.5 block text-sm font-medium text-text-primary">中心 X</label>
            <input id="fd-x" v-model.number="fdForm.center_x" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div>
            <label for="fd-z" class="mb-1.5 block text-sm font-medium text-text-primary">中心 Z</label>
            <input id="fd-z" v-model.number="fdForm.center_z" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div>
            <label for="fd-radius" class="mb-1.5 block text-sm font-medium text-text-primary">半径</label>
            <input id="fd-radius" v-model.number="fdForm.radius" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </div>
          <div class="flex items-end">
            <button
              type="submit"
              :disabled="busy === 'find'"
              class="h-10 w-full rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
            >
              {{ busy === 'find' ? '搜索中…' : '搜索' }}
            </button>
          </div>
        </form>
        <p class="mt-2 text-xs text-text-muted">服务端搜索限时 5 秒，超时会返回已找到的部分结果。</p>

        <div v-if="fdResult" class="mt-4 overflow-x-auto">
          <p v-if="!fdResult.seeds.length" class="text-sm text-text-muted">未找到匹配的种子。</p>
          <table v-else class="w-full text-left text-sm">
            <thead class="border-b border-border-dark text-xs text-text-muted">
              <tr><th scope="col" class="px-3 py-2 font-medium">种子</th><th scope="col" class="px-3 py-2 font-medium">结构坐标</th><th scope="col" class="px-3 py-2 font-medium">距离</th><th scope="col" class="px-3 py-2 text-right font-medium">操作</th></tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="(s, i) in fdResult.seeds" :key="i">
                <td class="px-3 py-2 font-mono text-grass-300">{{ s.seed }}</td>
                <td class="px-3 py-2 font-mono text-text-secondary">{{ s.x }} {{ s.z }}</td>
                <td class="px-3 py-2 text-text-muted">{{ s.distance.toFixed(1) }}</td>
                <td class="px-3 py-2 text-right"><CopyButton tool="seed-tools" :text="String(s.seed)" label="复制种子" /></td>
              </tr>
            </tbody>
          </table>
          <p v-if="fdResult.note" class="mt-2 text-xs text-text-muted">{{ fdResult.note }}</p>
        </div>
      </section>
    </div>
  </PageContainer>
</template>
