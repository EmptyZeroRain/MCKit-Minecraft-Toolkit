<script setup lang="ts">
// 存档诊断：解析 Minecraft Java 版存档文件并输出只读诊断。
//   - level.dat：世界基础信息（种子、版本、出生点、游戏规则…）
//   - r.X.Z.mca：区域文件区块清单 + 结构诊断报告（区分「确定错误」与「警告」）
// 安全说明：文件仅在后端内存中解析、不落盘；不修改存档，也不提供自动修复。
import { computed, ref } from 'vue'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import { inspectRegion, inspectRegionChunk, parseLevelDat } from '@/api/region'
import { ApiError } from '@/types/api'
import {
  compressionLabel,
  type LevelParseResult,
  type RegionChunkDetailResult,
  type RegionChunkEntry,
  type RegionInspectResult,
  type RegionIssue,
} from '@/types/mc'
import {
  classifySaveFile,
  formatMiB,
  validateLevelFile,
  validateRegionFile,
  type SaveFileKind,
} from '@/utils/region-file'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure, reportToolUse } from '@/utils/tool-stats'

applyPageMeta({
  title: '存档诊断',
  description:
    '上传 Minecraft Java 版存档文件：解析 level.dat 的世界信息（种子、版本、出生点），或检查 .mca 区域文件的区块结构并输出只读诊断报告。',
  canonicalPath: '/region-inspect',
})

useRecentToolsStore().record('region-inspect', '存档诊断', '辅助类')

const file = ref<File | null>(null)
const fileKind = ref<SaveFileKind | null>(null)
const regionResult = ref<RegionInspectResult | null>(null)
const levelResult = ref<LevelParseResult | null>(null)
const detail = ref<RegionChunkDetailResult | null>(null)
const loading = ref(false)
const detailLoading = ref(false)
const error = ref('')
const fieldError = ref('')
const keyword = ref('')

const sizeLabel = computed(() => (file.value ? formatMiB(file.value.size) : ''))

// 后端可能返回 null（无告警/无问题），统一归一为空数组，避免模板访问 .length 报错。
const levelWarnings = computed(() => levelResult.value?.warnings ?? [])
const regionErrors = computed(() => regionResult.value?.diagnosis?.errors ?? [])
const regionWarnings = computed(() => regionResult.value?.diagnosis?.warnings ?? [])

const filteredChunks = computed(() => {
  const list = regionResult.value?.chunks ?? []
  const q = keyword.value.trim().toLowerCase()
  if (!q) return list
  return list.filter(
    (c) =>
      `${c.global_chunk_x},${c.global_chunk_z}`.includes(q) ||
      String(c.data_version ?? '').includes(q) ||
      (c.status ?? '').toLowerCase().includes(q),
  )
})

const GAME_TYPE_LABELS: Record<number, string> = {
  0: '生存',
  1: '创造',
  2: '冒险',
  3: '旁观',
}

const DIFFICULTY_LABELS: Record<number, string> = {
  0: '和平',
  1: '简单',
  2: '普通',
  3: '困难',
}

function gameTypeLabel(v?: number): string {
  if (v === undefined) return '—'
  return GAME_TYPE_LABELS[v] ?? String(v)
}

function difficultyLabel(v?: number): string {
  if (v === undefined) return '—'
  return DIFFICULTY_LABELS[v] ?? String(v)
}

function formatTimestamp(sec: number): string {
  if (!sec) return '—'
  const d = new Date(sec * 1000)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString('zh-CN')
}

function issueLocation(issue: RegionIssue): string {
  if (issue.local_x === undefined || issue.local_z === undefined) return '文件级'
  return `区块 (${issue.local_x}, ${issue.local_z})`
}

function resetState() {
  regionResult.value = null
  levelResult.value = null
  detail.value = null
  error.value = ''
  keyword.value = ''
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const picked = input.files?.[0] ?? null
  resetState()
  fieldError.value = ''
  file.value = picked
  fileKind.value = null
  if (!picked) return

  const kind = classifySaveFile(picked.name)
  if (!kind) {
    fieldError.value = '仅支持 level.dat 或形如 r.0.0.mca 的区域文件'
    return
  }
  const invalid = kind === 'level' ? validateLevelFile(picked.name, picked.size) : validateRegionFile(picked.name, picked.size)
  if (invalid) {
    fieldError.value = invalid
    return
  }
  fileKind.value = kind
  void analyze()
}

async function analyze() {
  if (!file.value || !fileKind.value) return
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    if (fileKind.value === 'level') {
      levelResult.value = await parseLevelDat(file.value)
    } else {
      regionResult.value = await inspectRegion(file.value)
    }
    // TODO §13：解析成功 -> 记一次「使用」。
    reportToolUse('region-inspect')
  } catch (err) {
    error.value =
      err instanceof ApiError ? err.message : '解析失败，请确认文件类型与内容是否正确'
    regionResult.value = null
    levelResult.value = null
    reportToolFailure('region-inspect')
  } finally {
    loading.value = false
  }
}

async function openChunk(chunk: RegionChunkEntry) {
  if (!file.value || fileKind.value !== 'region') return
  detailLoading.value = true
  error.value = ''
  try {
    detail.value = await inspectRegionChunk(file.value, chunk.local_x, chunk.local_z)
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '区块详情加载失败'
  } finally {
    detailLoading.value = false
  }
}
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: '存档诊断' }]" />
    <div class="mb-6">
      <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">存档诊断</h1>
      <p class="text-text-secondary">
        上传 <code class="code-block px-1.5 py-0.5">level.dat</code> 查看世界信息，或上传
        <code class="code-block px-1.5 py-0.5">r.X.Z.mca</code> 检查区域文件结构。
        文件仅在后端内存中解析，不会保存，也不提供自动修复。
      </p>
    </div>

    <!-- 上传 -->
    <section class="card mb-6 p-5">
      <label for="save-file" class="mb-1.5 block text-sm font-medium text-text-primary">
        存档文件（level.dat 或 r.X.Z.mca，≤ 64 MiB）
      </label>
      <input
        id="save-file"
        type="file"
        accept=".dat,.mca"
        class="block w-full cursor-pointer rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-secondary file:mr-3 file:rounded file:border-0 file:bg-ink-700 file:px-3 file:py-1.5 file:text-text-primary"
        @change="onFileChange"
      />
      <p v-if="fieldError" role="alert" class="mt-2 text-xs text-danger-mc">{{ fieldError }}</p>
      <p v-else-if="file" class="mt-2 text-xs text-text-muted">
        已选择：{{ file.name }}（{{ sizeLabel }}）
      </p>
    </section>

    <div
      v-if="error"
      class="mb-6 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      {{ error }}
    </div>

    <LoadingSkeleton v-if="loading" :count="4" />

    <!-- ===== level.dat 结果 ===== -->
    <template v-else-if="levelResult">
      <section class="card mb-6 p-5">
        <h2 class="mb-3 text-lg font-bold text-text-primary">世界信息</h2>
        <dl class="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
          <div>
            <dt class="text-text-muted">世界名称</dt>
            <dd class="text-text-primary">{{ levelResult.world.level_name ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">数据版本</dt>
            <dd class="text-text-primary">{{ levelResult.world.data_version ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">游戏版本</dt>
            <dd class="text-text-primary">{{ levelResult.world.version?.name ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">最近游玩</dt>
            <dd class="text-text-primary">{{ levelResult.world.last_played ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">种子</dt>
            <dd class="font-mono text-text-primary">{{ levelResult.world.random_seed ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">出生点</dt>
            <dd class="font-mono text-text-primary">
              {{
                levelResult.world.spawn_x !== undefined
                  ? `${levelResult.world.spawn_x}, ${levelResult.world.spawn_y ?? '?'}, ${levelResult.world.spawn_z ?? '?'}`
                  : '—'
              }}
            </dd>
          </div>
          <div>
            <dt class="text-text-muted">游戏模式</dt>
            <dd class="text-text-primary">{{ gameTypeLabel(levelResult.world.game_type) }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">难度</dt>
            <dd class="text-text-primary">{{ difficultyLabel(levelResult.world.difficulty) }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">世界生成器</dt>
            <dd class="text-text-primary">{{ levelResult.world.generator_name ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">极限模式</dt>
            <dd class="text-text-primary">
              {{ levelResult.world.hardcore === undefined ? '—' : levelResult.world.hardcore ? '是' : '否' }}
            </dd>
          </div>
          <div>
            <dt class="text-text-muted">允许作弊</dt>
            <dd class="text-text-primary">
              {{ levelResult.world.allow_commands === undefined ? '—' : levelResult.world.allow_commands ? '是' : '否' }}
            </dd>
          </div>
          <div>
            <dt class="text-text-muted">游戏时间（tick）</dt>
            <dd class="font-mono text-text-primary">{{ levelResult.world.time ?? '—' }}</dd>
          </div>
        </dl>
      </section>

      <!-- 诊断告警 -->
      <section class="mb-6">
        <h2 class="mb-3 text-lg font-bold text-text-primary">
          诊断告警（{{ levelWarnings.length }}）
        </h2>
        <div
          v-if="levelWarnings.length"
          class="card divide-y divide-border-dark overflow-hidden"
        >
          <div
            v-for="w in levelWarnings"
            :key="w.code"
            class="flex items-start gap-3 px-4 py-3 text-sm"
          >
            <span class="mt-0.5 shrink-0 rounded bg-amber-mc/20 px-1.5 py-0.5 text-xs text-amber-mc">告警</span>
            <div>
              <code class="text-xs text-text-muted">{{ w.code }}</code>
              <p class="text-text-secondary">{{ w.message }}</p>
            </div>
          </div>
        </div>
        <p v-else class="text-sm text-text-muted">未发现明显异常。</p>
        <p v-if="levelResult.note" class="mt-2 text-xs text-text-muted">{{ levelResult.note }}</p>
      </section>

      <!-- 游戏规则 -->
      <section v-if="levelResult.world.game_rules" class="card mb-6 p-5">
        <h2 class="mb-3 text-lg font-bold text-text-primary">
          游戏规则（{{ Object.keys(levelResult.world.game_rules).length }}）
        </h2>
        <dl class="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
          <div
            v-for="(value, key) in levelResult.world.game_rules"
            :key="key"
            class="flex items-center justify-between rounded-md border border-border-dark bg-ink-900 px-3 py-2"
          >
            <dt class="truncate font-mono text-xs text-text-muted">{{ key }}</dt>
            <dd class="ml-2 shrink-0 font-mono text-text-primary">{{ value }}</dd>
          </div>
        </dl>
      </section>

      <details class="card p-5">
        <summary class="cursor-pointer text-sm text-grass-400">
          顶层字段名（{{ levelResult.top_level_keys.length }} 个）
        </summary>
        <p class="mt-2 break-all font-mono text-xs text-text-muted">
          {{ levelResult.top_level_keys.join('、') }}
        </p>
      </details>
    </template>

    <!-- ===== .mca 结果 ===== -->
    <template v-else-if="regionResult">
      <!-- 概览 -->
      <section class="card mb-6 p-5">
        <h2 class="mb-3 text-lg font-bold text-text-primary">区域概览</h2>
        <dl class="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
          <div>
            <dt class="text-text-muted">区域坐标</dt>
            <dd class="font-mono text-text-primary">{{ regionResult.region_x }}, {{ regionResult.region_z }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">区块数量</dt>
            <dd class="text-text-primary">{{ regionResult.chunk_count }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">文件大小</dt>
            <dd class="text-text-primary">{{ formatMiB(regionResult.stats?.file_size ?? 0) }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">扇区占用</dt>
            <dd class="text-text-primary">
              {{ regionResult.stats?.sectors_declared ?? '—' }} / {{ regionResult.stats?.sector_capacity ?? '—' }}
            </dd>
          </div>
          <div>
            <dt class="text-text-muted">空位置项</dt>
            <dd class="text-text-primary">{{ regionResult.stats?.empty_slots ?? '—' }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">最早更新</dt>
            <dd class="text-text-primary">{{ formatTimestamp(regionResult.stats?.oldest_timestamp ?? 0) }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">最近更新</dt>
            <dd class="text-text-primary">{{ formatTimestamp(regionResult.stats?.newest_timestamp ?? 0) }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">压缩方式</dt>
            <dd class="text-text-primary">
              {{
                regionResult.stats
                  ? Object.entries(regionResult.stats.compression)
                      .map(([k, v]) => `${k}×${v}`)
                      .join('、') || '—'
                  : '—'
              }}
            </dd>
          </div>
        </dl>
        <p v-if="regionResult.timed_out" class="mt-3 text-xs text-amber-mc">
          逐区块 NBT 解析达到时限，部分区块未读取顶层字段（结构诊断不受影响）。
        </p>
      </section>

      <!-- 诊断报告 -->
      <section class="mb-6">
        <div class="mb-3 flex items-center gap-3">
          <h2 class="text-lg font-bold text-text-primary">诊断报告</h2>
          <span class="rounded bg-danger-mc/20 px-2 py-0.5 text-xs text-danger-mc">
            确定错误 {{ regionErrors.length }}
          </span>
          <span class="rounded bg-amber-mc/20 px-2 py-0.5 text-xs text-amber-mc">
            警告 {{ regionWarnings.length }}
          </span>
        </div>

        <div
          v-if="regionErrors.length"
          class="card mb-3 divide-y divide-border-dark overflow-hidden"
        >
          <div
            v-for="(issue, i) in regionErrors"
            :key="`e${i}`"
            class="flex items-start gap-3 px-4 py-3 text-sm"
          >
            <span class="mt-0.5 shrink-0 rounded bg-danger-mc/20 px-1.5 py-0.5 text-xs text-danger-mc">错误</span>
            <div>
              <code class="text-xs text-text-muted">{{ issue.code }} · {{ issueLocation(issue) }}</code>
              <p class="text-text-secondary">{{ issue.message }}</p>
            </div>
          </div>
        </div>

        <div
          v-if="regionWarnings.length"
          class="card divide-y divide-border-dark overflow-hidden"
        >
          <div
            v-for="(issue, i) in regionWarnings"
            :key="`w${i}`"
            class="flex items-start gap-3 px-4 py-3 text-sm"
          >
            <span class="mt-0.5 shrink-0 rounded bg-amber-mc/20 px-1.5 py-0.5 text-xs text-amber-mc">警告</span>
            <div>
              <code class="text-xs text-text-muted">{{ issue.code }} · {{ issueLocation(issue) }}</code>
              <p class="text-text-secondary">{{ issue.message }}</p>
            </div>
          </div>
        </div>

        <p
          v-if="!regionErrors.length && !regionWarnings.length"
          class="text-sm text-text-muted"
        >
          未发现明显的结构异常。
        </p>
        <p class="mt-2 text-xs text-text-muted">
          诊断仅做只读检查，不代表已修复；工具不提供自动修复，请先用游戏或备份验证后再处理。
        </p>
      </section>

      <!-- 区块列表 -->
      <section>
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-lg font-bold text-text-primary">区块列表</h2>
          <input
            v-model="keyword"
            type="search"
            aria-label="筛选区块"
            placeholder="筛选：区块坐标 / 数据版本 / 状态"
            class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </div>

        <div v-if="filteredChunks.length" class="card overflow-hidden">
          <div class="max-h-[28rem] overflow-auto">
            <table class="w-full text-left text-sm">
              <thead class="sticky top-0 border-b border-border-dark bg-ink-800 text-xs text-text-muted">
                <tr>
                  <th scope="col" class="px-3 py-2.5 font-medium">全局区块</th>
                  <th scope="col" class="hidden px-3 py-2.5 font-medium md:table-cell">本地</th>
                  <th scope="col" class="px-3 py-2.5 font-medium">数据版本</th>
                  <th scope="col" class="hidden px-3 py-2.5 font-medium lg:table-cell">状态</th>
                  <th scope="col" class="hidden px-3 py-2.5 font-medium lg:table-cell">压缩</th>
                  <th scope="col" class="hidden px-3 py-2.5 font-medium lg:table-cell">更新时间</th>
                  <th scope="col" class="px-3 py-2.5 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border-dark">
                <tr v-for="c in filteredChunks" :key="`${c.local_x},${c.local_z}`" class="hover:bg-ink-800/60">
                  <td class="px-3 py-2 font-mono text-text-primary">{{ c.global_chunk_x }}, {{ c.global_chunk_z }}</td>
                  <td class="hidden px-3 py-2 font-mono text-text-muted md:table-cell">{{ c.local_x }}, {{ c.local_z }}</td>
                  <td class="px-3 py-2 text-text-secondary">{{ c.data_version ?? '—' }}</td>
                  <td class="hidden px-3 py-2 text-text-secondary lg:table-cell">{{ c.status ?? '—' }}</td>
                  <td class="hidden px-3 py-2 text-text-muted lg:table-cell">{{ compressionLabel(c.compression) }}</td>
                  <td class="hidden px-3 py-2 text-xs text-text-muted lg:table-cell">{{ formatTimestamp(c.timestamp) }}</td>
                  <td class="px-3 py-2 text-right">
                    <button
                      type="button"
                      class="rounded-md border border-border-light bg-ink-700 px-2.5 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
                      @click="openChunk(c)"
                    >
                      查看 NBT
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <EmptyState
          v-else
          title="没有匹配的区块"
          description="该区域文件没有区块，或筛选条件没有命中。"
          action-label="清除筛选"
          @action="keyword = ''"
        />
      </section>

      <!-- 区块详情 -->
      <section v-if="detailLoading" class="mt-6">
        <LoadingSkeleton :count="2" />
      </section>
      <section v-else-if="detail" class="card mt-6 p-5">
        <div class="mb-3 flex flex-wrap items-center gap-3">
          <h2 class="text-lg font-bold text-text-primary">区块 NBT 摘要</h2>
          <code class="code-block px-2 py-0.5 text-xs text-grass-300">
            {{ detail.global_chunk_x }}, {{ detail.global_chunk_z }}
          </code>
          <span class="text-xs text-text-muted">压缩：{{ compressionLabel(detail.compression) }}</span>
          <CopyButton tool="region-inspect"
            class="ml-auto"
            :text="JSON.stringify(detail.fields, null, 2)"
            label="复制字段"
          />
        </div>

        <dl class="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="(value, key) in detail.fields" :key="key" class="rounded-md border border-border-dark bg-ink-900 px-3 py-2">
            <dt class="text-xs text-text-muted">{{ key }}</dt>
            <dd class="font-mono text-text-primary">{{ value }}</dd>
          </div>
        </dl>
        <p v-if="!Object.keys(detail.fields).length" class="text-sm text-text-muted">
          该区块没有可展示的顶层标量字段。
        </p>

        <details class="mt-4">
          <summary class="cursor-pointer text-sm text-grass-400">
            顶层字段名（{{ detail.top_level_keys.length }} 个）
          </summary>
          <p class="mt-2 break-all font-mono text-xs text-text-muted">
            {{ detail.top_level_keys.join('、') }}
          </p>
        </details>
      </section>
    </template>

    <ErrorState
      v-else
      title="等待上传存档文件"
      message="选择 level.dat 或 r.X.Z.mca 文件后，这里会显示世界信息或区域诊断报告。"
    />
  </PageContainer>
</template>
