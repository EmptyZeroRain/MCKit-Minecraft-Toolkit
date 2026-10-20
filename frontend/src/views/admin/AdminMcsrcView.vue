<script setup lang="ts">
// MC 源码缓存运维（需 ops.mcsrc 权限）：查看服务端已缓存的 jar / 索引与磁盘占用，
// 支持按版本清理与整体清空。jar 与索引都是可重建的派生数据，清理不影响业务数据，
// 仅导致下次访问源码浏览器时重新下载与建索引。
import { computed, onMounted, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useUiStore } from '@/stores/ui'
import {
  listMcsrcCache,
  previewMcsrcPurge,
  purgeAllMcsrcCache,
  purgeMcsrcCache,
  rebuildMcsrcCache,
  type McsrcCacheImpact,
} from '@/api/admin-mcsrc'
import type { McsrcCacheEntry, McsrcCacheUsage } from '@/types/admin'
import { ApiError } from '@/types/api'
import { formatDateTime, formatSize, shortSha1, timeAgo } from '@/utils/format'

const ui = useUiStore()

const items = ref<McsrcCacheEntry[]>([])
const usage = ref<McsrcCacheUsage | null>(null)
const loading = ref(false)
const error = ref('')
/** 缓存占用告警阈值（字节）；0 / 未配置表示不提示 */
const alertThreshold = ref(0)
/** 正在入队重建的 key */
const rebuildingKey = ref('')

const overCapacity = computed(
  () => alertThreshold.value > 0 && (usage.value?.total_bytes ?? 0) >= alertThreshold.value,
)

const purgingKey = ref('')
const confirmTarget = ref<McsrcCacheEntry | null>(null)
const confirmAllOpen = ref(false)
const confirmBusy = ref(false)

const usageText = computed(() => {
  if (!usage.value) return ''
  return `共 ${usage.value.entries} 个版本 · jar ${formatSize(usage.value.jar_bytes)} · 索引 ${formatSize(
    usage.value.index_bytes,
  )} · 合计 ${formatSize(usage.value.total_bytes)}`
})

// 缓存总占用（简写），用于概览标题与清空确认文案。
const totalLabel = computed(() => (usage.value ? formatSize(usage.value.total_bytes) : '—'))

function keyOf(item: McsrcCacheEntry): string {
  return `${item.version}-${item.side}`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listMcsrcCache()
    items.value = res.items ?? []
    usage.value = res.usage ?? null
    alertThreshold.value = res.alert_threshold_bytes ?? 0
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

/**
 * 重建缓存：清理 → 重新下载 → 重建索引。
 *
 * 耗时数十秒，因此后端只入队、立即返回 job_id；进度与失败原因在「后台任务」页。
 */
async function rebuild(item: McsrcCacheEntry) {
  if (rebuildingKey.value) return
  rebuildingKey.value = keyOf(item)
  try {
    const res = await rebuildMcsrcCache(item.version, item.side)
    ui.success(`已入队重建 ${item.version} · ${item.side}，进度见「后台任务」（任务 ${res.job_id.slice(0, 8)}…）`)
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '重建任务入队失败')
  } finally {
    rebuildingKey.value = ''
  }
}

// 清理影响预览（预计释放空间、文件数、是否正在准备）。
const purgePreview = ref<McsrcCacheImpact | null>(null)
const previewLoading = ref(false)

async function askPurge(item: McsrcCacheEntry) {
  confirmTarget.value = item
  purgePreview.value = null
  previewLoading.value = true
  try {
    purgePreview.value = await previewMcsrcPurge(item.version, item.side)
  } catch {
    // 预览失败不阻断：仍可依据列表数据确认清理。
  } finally {
    previewLoading.value = false
  }
}

function cancelPurge() {
  confirmTarget.value = null
  purgePreview.value = null
}

const purgeMessage = computed(() => {
  const t = confirmTarget.value
  if (!t) return ''
  const p = purgePreview.value
  if (!p) {
    return `将删除 ${t.version} · ${t.side} 的 jar、索引与元数据（共 ${formatSize(
      t.total_size,
    )}）。\n下次访问该版本时会重新下载并建索引，耗时数十秒。`
  }
  if (p.preparing) {
    return `${t.version} · ${t.side} 正在下载 / 建索引，此时清理会被拒绝。\n请等待准备完成后再试。`
  }
  return `将删除 ${t.version} · ${t.side} 的 jar、索引与元数据（共 ${formatSize(
    p.total_bytes,
  )}，${p.files.length} 个文件）。\n这些数据均可重建，下次访问时会重新下载并建索引，耗时数十秒。`
})

async function doPurge() {
  const target = confirmTarget.value
  if (!target) return
  confirmBusy.value = true
  purgingKey.value = keyOf(target)
  try {
    const res = await purgeMcsrcCache(target.version, target.side)
    ui.success(`已清理 ${target.version} · ${target.side}，释放 ${formatSize(res.freed_bytes)}`)
    confirmTarget.value = null
    purgePreview.value = null
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '清理失败')
  } finally {
    confirmBusy.value = false
    purgingKey.value = ''
  }
}

async function doPurgeAll() {
  confirmBusy.value = true
  try {
    const res = await purgeAllMcsrcCache()
    ui.success(`已清空 ${res.purged} 个版本，释放 ${formatSize(res.freed_bytes)}`)
    confirmAllOpen.value = false
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '清空失败')
  } finally {
    confirmBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <header>
      <h1 class="text-xl font-bold text-text-primary md:text-2xl">MC 源码缓存</h1>
      <p class="mt-1 text-sm text-text-secondary">
        服务端按版本缓存的官方 jar 与类索引（供
        <RouterLink to="/mcsrc" class="text-grass-300 hover:underline">源码浏览器</RouterLink>
        使用）。二者均为<strong>可重建的派生数据</strong>：清理只意味着下次访问时重新下载与建索引，
        不影响任何业务数据。清理动作会写入审计日志。
      </p>
      <p v-if="usageText" class="mt-2 text-xs text-text-muted">{{ usageText }}</p>
      <p
        v-if="overCapacity"
        class="mt-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-300"
      >
        缓存占用已达到告警阈值 {{ formatSize(alertThreshold) }}，运营告警中心会产生容量告警。
        可清理不再需要的版本，或调高 MCSRC_CACHE_ALERT_MB。
      </p>
    </header>

    <div class="card flex flex-wrap items-center justify-between gap-3 p-4">
      <p class="text-sm text-text-secondary">
        缓存当前占用 <span class="font-medium text-text-primary">{{ totalLabel }}</span>
        <span class="text-text-muted">（磁盘上限由部署侧控制，缓存只增不减，需人工清理）</span>
      </p>
      <div class="flex gap-2">
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary transition hover:bg-ink-600 disabled:opacity-60"
          :disabled="loading"
          @click="load"
        >
          刷新
        </button>
        <button
          type="button"
          class="rounded-md border border-danger-mc bg-danger-mc/10 px-4 py-2 text-sm font-medium text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-60"
          :disabled="!items.length || loading"
          @click="confirmAllOpen = true"
        >
          清空全部缓存
        </button>
      </div>
    </div>

    <ListShell title="已缓存版本" :loading="loading" :error="error" @retry="load">
      <div class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">版本</th>
                <th scope="col" class="px-4 py-3 font-medium">索引</th>
                <th scope="col" class="px-4 py-3 font-medium">jar 体积</th>
                <th scope="col" class="px-4 py-3 font-medium">索引体积</th>
                <th scope="col" class="px-4 py-3 font-medium">合计</th>
                <th scope="col" class="px-4 py-3 font-medium">缓存时间</th>
                <th scope="col" class="px-4 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in items" :key="keyOf(item)" class="align-top hover:bg-ink-800/60">
                <td class="px-4 py-3">
                  <div class="font-medium text-text-primary">{{ item.version }}</div>
                  <div class="mt-0.5 text-xs text-text-muted">
                    {{ item.side === 'server' ? '服务端' : '客户端' }} · sha1
                    {{ shortSha1(item.sha1) }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span
                    v-if="item.indexed"
                    class="inline-flex rounded border border-grass-500/40 bg-grass-500/10 px-1.5 py-0.5 text-xs text-grass-300"
                  >
                    已建索引
                  </span>
                  <span
                    v-else
                    class="inline-flex rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted"
                  >
                    仅 jar
                  </span>
                  <div v-if="item.indexed" class="mt-0.5 text-xs text-text-muted">
                    {{ item.class_count }} 个类
                    <span v-if="item.index_built_at">· {{ timeAgo(item.index_built_at) }}</span>
                  </div>
                </td>
                <td class="px-4 py-3 text-text-secondary">{{ formatSize(item.size) }}</td>
                <td class="px-4 py-3 text-text-secondary">{{ formatSize(item.index_size) }}</td>
                <td class="px-4 py-3 text-text-secondary">{{ formatSize(item.total_size) }}</td>
                <td class="px-4 py-3 text-xs text-text-muted" :title="formatDateTime(item.fetched_at)">
                  {{ timeAgo(item.fetched_at) || '—' }}
                </td>
                <td class="px-4 py-3">
                  <div class="flex flex-wrap gap-2">
                    <button
                      type="button"
                      class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                      :disabled="rebuildingKey === keyOf(item)"
                      :title="'清理后重新下载并重建索引（以后台任务执行）'"
                      @click="rebuild(item)"
                    >
                      {{ rebuildingKey === keyOf(item) ? '入队中…' : '重建' }}
                    </button>
                    <button
                      type="button"
                      class="rounded border border-border-light bg-ink-700 px-2 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/10 disabled:opacity-50"
                      :disabled="purgingKey === keyOf(item)"
                      @click="askPurge(item)"
                    >
                      {{ purgingKey === keyOf(item) ? '清理中…' : '清理' }}
                    </button>
                  </div>
                  <RouterLink to="/admin/ops?tab=jobs" class="mt-1 inline-block text-xs text-text-muted hover:underline">
                    查看任务
                  </RouterLink>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <p v-if="!items.length" class="px-4 py-8 text-center text-sm text-text-muted">
          暂无缓存——访客首次访问源码浏览器时会自动触发下载与建索引。
        </p>
      </div>
    </ListShell>

    <ConfirmDialog
      :open="confirmTarget !== null"
      title="清理该版本的源码缓存？"
      :message="purgeMessage"
      confirm-text="清理"
      dangerous
      :confirm-disabled="confirmBusy || previewLoading || purgePreview?.preparing === true"
      @confirm="doPurge"
      @cancel="cancelPurge"
    />

    <ConfirmDialog
      :open="confirmAllOpen"
      title="清空全部源码缓存？"
      :message="`将删除全部 ${items.length} 个已缓存版本（共 ${totalLabel}）的 jar、索引与元数据。\n此后所有访客访问源码浏览器都会重新触发下载与建索引，短时间内会明显变慢。`"
      confirm-text="清空全部"
      dangerous
      :confirm-disabled="confirmBusy"
      @confirm="doPurgeAll"
      @cancel="confirmAllOpen = false"
    />
  </div>
</template>
