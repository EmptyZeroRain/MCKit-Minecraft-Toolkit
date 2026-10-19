<script setup lang="ts">
// 种子地图：输入世界种子，绘制区块地图并标出史莱姆区块与结构理论候选坐标。
// 说明：
//   - 史莱姆区块由前端本地引擎精确计算（即时可用）；
//   - 结构理论坐标与近似出生点依赖后端 /seed/analyze，失败时降级为「仅本地史莱姆区块」；
//   - 近似出生点由后端群系采样（include_biomes）在前端做最近有效群系估算。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import FilterPanel from '@/components/common/FilterPanel.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import { analyzeSeed } from '@/api/seed'
import { generateSeeds } from '@/api/mc'
import { ApiError } from '@/types/api'
import { SEED_VERSIONS, type StructurePoint, type SeedVersion } from '@/types/seed'
import { localSeedAnalysis } from '@/utils/seed/engine'
import { approximateSpawn } from '@/utils/seed/spawn'
import {
  blockToPixel,
  gridStepBlocks,
  mapHalfBlocks,
  structureColor,
  structureLabel,
} from '@/utils/seed/map'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure, reportToolUse } from '@/utils/tool-stats'
import { changedOnly, queryNumber, queryString } from '@/utils/share'

applyPageMeta({
  title: '种子地图',
  description:
    '输入 Minecraft 世界种子，查看史莱姆区块分布与村庄、要塞、远古城市等结构的理论候选坐标。',
  canonicalPath: '/seed-map',
})

useRecentToolsStore().record('seed-map', '种子地图', '查询类')

const CANVAS_SIZE = 560

const route = useRoute()
const router = useRouter()

// TODO §14：种子 / 版本 / 半径 / 结构筛选同步到 URL；带种子的链接打开后自动分析，
// 使「某个种子的分析结果」可直接分享复现。
function restoreVersion(value: string): SeedVersion {
  return (SEED_VERSIONS as readonly string[]).includes(value)
    ? (value as SeedVersion)
    : SEED_VERSIONS[0]
}

const seed = ref(queryString(route.query.seed))
const version = ref<SeedVersion>(restoreVersion(queryString(route.query.version)))
const radius = ref(queryNumber(route.query.radius, 8))

/** 将查询条件写入 URL（仅保留非默认值）。 */
function syncUrl() {
  router.replace({
    path: '/seed-map',
    query: changedOnly(
      {
        seed: seed.value,
        version: version.value,
        radius: radius.value,
        type: listType.value,
      },
      { seed: '', version: SEED_VERSIONS[0], radius: 8, type: '' },
    ),
  })
}

const worldSeed = ref('')
const structures = ref<StructurePoint[]>([])
const loading = ref(false)
const error = ref('')
const fieldError = ref('')
/** 后端不可用时降级为本地结果 */
const degraded = ref(false)
const note = ref('')

const canvasRef = ref<HTMLCanvasElement | null>(null)
const listType = ref(queryString(route.query.type))

watch([seed, version, radius, listType], syncUrl)

const half = computed(() => mapHalfBlocks(radius.value))

/** 列表中实际出现的结构类型（含计数） */
const presentTypes = computed(() => {
  const counts = new Map<string, number>()
  for (const s of structures.value) counts.set(s.type, (counts.get(s.type) ?? 0) + 1)
  return [...counts.entries()].map(([value, count]) => ({
    value,
    label: `${structureLabel(value)}（${count}）`,
  }))
})

const filteredStructures = computed(() =>
  listType.value ? structures.value.filter((s) => s.type === listType.value) : structures.value,
)

const slimeCount = computed(() => structures.value.filter((s) => s.type === 'slime_chunk').length)

/** 绘制地图：网格 + 史莱姆区块 + 结构标记 + 原点 */
function draw() {
  const cv = canvasRef.value
  if (!cv) return
  const ctx = cv.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const size = CANVAS_SIZE
  cv.width = size * dpr
  cv.height = size * dpr
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

  const h = half.value
  const scale = size / (h * 2)

  // 背景
  ctx.fillStyle = '#0b1220'
  ctx.fillRect(0, 0, size, size)

  // 网格
  const step = gridStepBlocks(h)
  ctx.strokeStyle = 'rgba(92, 122, 180, 0.20)'
  ctx.lineWidth = 1
  for (let b = -h; b <= h; b += step) {
    const v = blockToPixel(b, 0, h, size)
    ctx.beginPath()
    ctx.moveTo(v.px, 0)
    ctx.lineTo(v.px, size)
    ctx.stroke()
    const hz = blockToPixel(0, b, h, size)
    ctx.beginPath()
    ctx.moveTo(0, hz.py)
    ctx.lineTo(size, hz.py)
    ctx.stroke()
  }

  // 史莱姆区块：结构点坐标是区块中心（cx*16+8），回推区块左上角
  ctx.fillStyle = 'rgba(124, 209, 95, 0.32)'
  for (const s of structures.value) {
    if (s.type !== 'slime_chunk') continue
    const tl = blockToPixel(s.x - 8, s.z - 8, h, size)
    ctx.fillRect(tl.px, tl.py, 16 * scale, 16 * scale)
  }

  // 其他结构标记
  for (const s of structures.value) {
    if (s.type === 'slime_chunk') continue
    const p = blockToPixel(s.x, s.z, h, size)
    ctx.beginPath()
    ctx.arc(p.px, p.py, 4.5, 0, Math.PI * 2)
    ctx.fillStyle = structureColor(s.type)
    ctx.fill()
    ctx.strokeStyle = '#0b1220'
    ctx.lineWidth = 1.5
    ctx.stroke()
  }

  // 原点十字
  ctx.strokeStyle = 'rgba(230, 237, 247, 0.75)'
  ctx.lineWidth = 1.5
  ctx.beginPath()
  ctx.moveTo(size / 2 - 7, size / 2)
  ctx.lineTo(size / 2 + 7, size / 2)
  ctx.moveTo(size / 2, size / 2 - 7)
  ctx.lineTo(size / 2, size / 2 + 7)
  ctx.stroke()
}

async function analyze() {
  fieldError.value = ''
  error.value = ''
  degraded.value = false
  note.value = ''

  if (!seed.value.trim()) {
    fieldError.value = '请输入世界种子'
    return
  }

  loading.value = true
  try {
    // 本地引擎先算出精确史莱姆区块（毫秒级，无网络依赖）
    const local = localSeedAnalysis(seed.value, { radius: radius.value, version: version.value })
    worldSeed.value = local.world_seed
    structures.value = local.slime_chunks

    // 后端补齐结构理论候选坐标与群系采样（用于近似出生点）
    const res = await analyzeSeed({
      seed: seed.value,
      version: version.value,
      radius: radius.value,
      include_structures: true,
      include_biomes: true,
    })
    worldSeed.value = res.world_seed
    const remote = res.structures ?? local.slime_chunks
    const spawn = res.biomes ? approximateSpawn(res.biomes) : null
    structures.value = spawn
      ? [
          { type: 'spawn', x: spawn.x, z: spawn.z, label: `近似出生点（${spawn.biome}）` },
          ...remote,
        ]
      : remote
    note.value = res.note ?? ''
    degraded.value = false
    // TODO §13：种子分析成功 -> 记一次「使用」。
    reportToolUse('seed-map')
  } catch (err) {
    // 降级：保留本地史莱姆区块，提示后端不可用
    degraded.value = true
    error.value = err instanceof ApiError ? err.message : '结构数据获取失败'
    // TODO §13：后端结构数据获取失败计入「失败」（本地史莱姆区块仍可用，
    // 但运营看板需要暴露这类接口故障）。
    reportToolFailure('seed-map')
  } finally {
    loading.value = false
    await nextTick()
    draw()
  }
}

/** TODO §14：填入示例种子并直接分析，便于首次使用者理解输入格式与产出。 */
function fillExample() {
  seed.value = '-1234567890'
  void analyze()
}

async function randomSeed() {
  try {
    const res = await generateSeeds({ count: 1 })
    if (res.seeds.length) {
      seed.value = String(res.seeds[0])
      await analyze()
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '随机种子生成失败'
  }
}

function reset() {
  seed.value = ''
  structures.value = []
  worldSeed.value = ''
  note.value = ''
  error.value = ''
  degraded.value = false
  fieldError.value = ''
  listType.value = ''
  void nextTick().then(draw)
}

onMounted(() => {
  void nextTick().then(draw)
  // TODO §14：带种子的分享链接打开后直接复现分析结果。
  if (seed.value.trim()) void analyze()
})
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: '种子地图' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">种子地图</h1>
        <p class="text-text-secondary">
          输入世界种子（数字或文本），查看史莱姆区块分布与村庄、要塞、远古城市等结构的理论候选坐标。
        </p>
      </div>
      <ShareButton />
    </div>

    <!-- 查询表单 -->
    <form class="card mb-6 p-5" @submit.prevent="analyze">
      <div class="grid gap-4 md:grid-cols-[1fr_180px_200px]">
        <div>
          <label for="seed" class="mb-1.5 block text-sm font-medium text-text-primary">世界种子</label>
          <input
            id="seed"
            v-model="seed"
            type="text"
            placeholder="例如：-1234567890 或 my world"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            :aria-invalid="!!fieldError"
          />
          <p v-if="fieldError" role="alert" class="mt-1 text-xs text-danger-mc">{{ fieldError }}</p>
        </div>
        <div>
          <label for="seed-version" class="mb-1.5 block text-sm font-medium text-text-primary">版本区间</label>
          <select
            id="seed-version"
            v-model="version"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          >
            <option v-for="v in SEED_VERSIONS" :key="v" :value="v">{{ v }}</option>
          </select>
        </div>
        <div>
          <label for="seed-radius" class="mb-1.5 block text-sm font-medium text-text-primary">
            区块半径：{{ radius }}
          </label>
          <input
            id="seed-radius"
            v-model.number="radius"
            type="range"
            min="1"
            max="32"
            class="mt-3 w-full accent-grass-500"
          />
        </div>
      </div>

      <div class="mt-4 flex flex-wrap items-center gap-3">
        <button
          type="submit"
          :disabled="loading"
          class="inline-flex items-center gap-2 rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
        >
          {{ loading ? '分析中…' : '分析种子' }}
        </button>
        <button
          type="button"
          :disabled="loading"
          class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 disabled:opacity-60"
          @click="fillExample"
        >
          填入示例
        </button>
        <button
          type="button"
          :disabled="loading"
          class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 disabled:opacity-60"
          @click="randomSeed"
        >
          随机种子
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-2 text-sm text-text-muted transition hover:text-text-primary"
          @click="reset"
        >
          清空
        </button>
        <span v-if="worldSeed" class="ml-auto text-sm text-text-muted">
          世界种子：<code class="code-block px-2 py-0.5 text-grass-300">{{ worldSeed }}</code>
        </span>
      </div>

      <p class="mt-3 text-xs leading-relaxed text-text-muted">
        半径以<strong>区块</strong>为单位（1 区块 = 16×16 方块，上限 32）；区域坐标只统计该范围内的区块与结构候选。
        下方坐标均为<strong>理论候选值</strong>，实际生成还会受生物群系与结构间距影响，不等于确定结果。
      </p>
    </form>

    <!-- 后端降级提示 -->
    <div
      v-if="degraded"
      class="mb-6 rounded-card border border-amber-mc/40 bg-amber-mc/10 px-4 py-3 text-sm text-amber-mc"
      role="status"
    >
      结构数据获取失败（{{ error }}），当前仅显示前端本地精确计算的史莱姆区块。
    </div>

    <!-- 地图 -->
    <section class="mb-8">
      <div class="mb-3 flex flex-wrap items-baseline gap-3">
        <h2 class="text-lg font-bold text-text-primary">区块地图</h2>
        <span class="text-xs text-text-muted">
          范围 ±{{ half }} 方块（约 {{ radius }}×2+1 区块）· 史莱姆区块 {{ slimeCount }} 个
        </span>
      </div>
      <div class="card overflow-hidden p-4">
        <canvas
          ref="canvasRef"
          class="mx-auto block w-full max-w-[560px] rounded-md border border-border-dark"
          style="aspect-ratio: 1 / 1; height: auto"
          role="img"
          :aria-label="`种子 ${worldSeed || '未分析'} 的区块地图`"
        />
        <p v-if="!worldSeed" class="mt-3 text-center text-sm text-text-muted">
          输入种子并点击「分析种子」后，这里会显示区块地图。
        </p>
        <div v-else-if="!structures.length" class="mt-3 text-center text-sm text-text-muted">
          该范围内未发现史莱姆区块或结构候选点。
        </div>
        <div v-else class="mt-3 flex flex-wrap justify-center gap-x-4 gap-y-1.5 text-xs text-text-muted">
          <span v-for="t in presentTypes" :key="t.value" class="inline-flex items-center gap-1.5">
            <span
              class="inline-block h-2.5 w-2.5 rounded-sm"
              :style="{ backgroundColor: structureColor(t.value) }"
              aria-hidden="true"
            />
            {{ t.label }}
          </span>
        </div>
      </div>
    </section>

    <!-- 结构列表 -->
    <section v-if="worldSeed">
      <div class="mb-3 flex flex-wrap items-baseline gap-3">
        <h2 class="text-lg font-bold text-text-primary">结构坐标</h2>
        <span class="text-xs text-text-muted">
          理论候选/估算值，实际生成受生物群系影响
        </span>
      </div>

      <div class="mb-4">
        <FilterPanel
          v-model="listType"
          title="按类型筛选"
          :options="presentTypes"
          all-label="全部"
        />
      </div>

      <div v-if="filteredStructures.length" class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">类型</th>
                <th scope="col" class="px-4 py-3 font-medium">X</th>
                <th scope="col" class="px-4 py-3 font-medium">Z</th>
                <th scope="col" class="hidden px-4 py-3 font-medium md:table-cell">说明</th>
                <th scope="col" class="px-4 py-3 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="(s, i) in filteredStructures" :key="`${s.type}-${s.x}-${s.z}-${i}`" class="hover:bg-ink-800/60">
                <td class="px-4 py-2.5">
                  <span class="inline-flex items-center gap-2 text-text-primary">
                    <span
                      class="inline-block h-2.5 w-2.5 rounded-full"
                      :style="{ backgroundColor: structureColor(s.type) }"
                      aria-hidden="true"
                    />
                    {{ structureLabel(s.type) }}
                  </span>
                </td>
                <td class="px-4 py-2.5 font-mono text-text-secondary">{{ s.x }}</td>
                <td class="px-4 py-2.5 font-mono text-text-secondary">{{ s.z }}</td>
                <td class="hidden px-4 py-2.5 text-xs text-text-muted md:table-cell">{{ s.label || '—' }}</td>
                <td class="px-4 py-2.5 text-right">
                  <CopyButton tool="seed-map" :text="`${s.x} ${s.z}`" label="复制坐标" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <EmptyState
        v-else
        title="没有匹配的结构"
        description="切换到「全部」查看该范围内的其他结构，或增大区块半径后重新分析。"
        action-label="清除筛选"
        @action="listType = ''"
      />

      <p v-if="note" class="mt-4 text-xs text-text-muted">{{ note }}</p>
    </section>

    <ErrorState
      v-else-if="error && !degraded"
      class="mt-4"
      :message="error"
      retryable
      @retry="analyze"
    />
  </PageContainer>
</template>
