<script setup lang="ts">
// 内容历史版本面板（gj.md §15 / §16.2）：列表 + 字段级差异 + 回滚。
// 回滚由后端走内容正常更新路径，会追加一条 rollback 版本（历史只增不减）。
import { onMounted, ref, watch } from 'vue'
import { getRevisionDiff, listRevisions, rollbackRevision } from '@/api/admin-revisions'
import type { RevisionContentType, RevisionDiff, RevisionListItem } from '@/types/revision'
import { ApiError } from '@/types/api'
import { useUiStore } from '@/stores/ui'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  /** 内容类型（决定后端权限点与路由段） */
  contentType: RevisionContentType
  /** 内容对象 ID */
  contentId: string
}>()

const emit = defineEmits<{ (e: 'rolled-back'): void }>()

const ui = useUiStore()

const loading = ref(false)
const items = ref<RevisionListItem[]>([])
const total = ref(0)

// 展开查看差异的版本号（同一时刻只展开一个，避免大快照同时渲染）
const expandedNo = ref<number | null>(null)
const diff = ref<RevisionDiff | null>(null)
const diffLoading = ref(false)
// 正在回滚的版本号，用于禁用按钮与文案反馈
const rollingNo = ref<number | null>(null)

// 已知字段的中文名；未知字段直接展示原始 key，避免遗漏新增字段。
const FIELD_LABELS: Record<string, string> = {
  slug: 'Slug',
  name: '名称',
  title: '标题',
  summary: '简介',
  description: '详细介绍',
  content: '正文',
  content_format: '正文格式',
  content_type: '内容类型',
  category: '分类',
  icon: '图标',
  tags: '标签',
  status: '状态',
  resource_type: '资源类型',
  cover_image_url: '封面图',
  author_name: '作者',
  source_platform: '来源平台',
  source_url: '来源链接',
  license: '许可协议',
  game_edition: '游戏版本',
  supported_versions: '支持版本',
  loader_types: '加载器',
  dependencies: '依赖',
  install_guide: '安装说明',
  compatibility_note: '兼容性备注',
  game_versions: '支持版本',
  source_references: '来源参考',
  input_schema: '输入结构',
  output_schema: '输出结构',
  is_featured: '首页推荐',
  sort_order: '排序权重',
  published_at: '发布时间',
}

const ACTION_LABELS: Record<string, string> = {
  create: '创建',
  update: '更新',
  rollback: '回滚',
}

const ACTION_CLASSES: Record<string, string> = {
  create: 'bg-grass-600/20 text-grass-400',
  update: 'bg-sky-500/20 text-sky-300',
  rollback: 'bg-amber-500/20 text-amber-300',
}

function fieldLabel(field: string): string {
  return FIELD_LABELS[field] ?? field
}

// 差异值渲染：字符串原样展示（超长截断），对象/数组转 JSON，空值用占位符。
function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'string') return value === '' ? '（空）' : value
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'number') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function errMessage(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}

async function load() {
  if (!props.contentId) return
  loading.value = true
  try {
    const res = await listRevisions(props.contentType, props.contentId, 1, 50)
    items.value = res.items
    total.value = res.total
  } catch (err) {
    ui.error(errMessage(err, '加载历史版本失败'))
  } finally {
    loading.value = false
  }
}

async function toggleDiff(revisionNo: number) {
  if (expandedNo.value === revisionNo) {
    expandedNo.value = null
    diff.value = null
    return
  }
  expandedNo.value = revisionNo
  diffLoading.value = true
  diff.value = null
  try {
    diff.value = await getRevisionDiff(props.contentType, props.contentId, revisionNo)
  } catch (err) {
    ui.error(errMessage(err, '加载差异失败'))
    expandedNo.value = null
  } finally {
    diffLoading.value = false
  }
}

async function doRollback(revisionNo: number) {
  if (rollingNo.value !== null) return
  if (!window.confirm(`确认回滚到版本 #${revisionNo}？该操作会生成一个新的历史版本，不会删除既有历史。`)) {
    return
  }
  rollingNo.value = revisionNo
  try {
    const rec = await rollbackRevision(props.contentType, props.contentId, revisionNo)
    ui.success(`已回滚，生成版本 #${rec.revision_no}`)
    expandedNo.value = null
    diff.value = null
    await load()
    emit('rolled-back')
  } catch (err) {
    ui.error(errMessage(err, '回滚失败'))
  } finally {
    rollingNo.value = null
  }
}

onMounted(load)
watch(() => [props.contentType, props.contentId], load)

// 供父组件在保存后主动刷新（保存会新增一条历史版本）。
defineExpose({ reload: load })
</script>

<template>
  <section class="card space-y-4 p-6">
    <header class="flex items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-medium text-text-primary">历史版本</h2>
        <p class="mt-0.5 text-xs text-text-secondary">
          共 {{ total }} 个版本 · 回滚会追加新版本，历史不会被覆盖或删除
        </p>
      </div>
      <button
        type="button"
        class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-xs text-text-primary transition hover:bg-ink-600 disabled:opacity-50"
        :disabled="loading"
        @click="load"
      >
        刷新
      </button>
    </header>

    <p v-if="loading" class="text-sm text-text-secondary">加载中…</p>
    <p v-else-if="items.length === 0" class="text-sm text-text-secondary">暂无历史版本</p>

    <ul v-else class="divide-y divide-border-dark/60">
      <li v-for="item in items" :key="item.id" class="py-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="font-mono text-sm text-text-primary">#{{ item.revision_no }}</span>
          <span
            class="rounded px-2 py-0.5 text-xs"
            :class="ACTION_CLASSES[item.action] ?? 'bg-ink-700 text-text-secondary'"
          >
            {{ ACTION_LABELS[item.action] ?? item.action }}
          </span>
          <span class="text-xs text-text-secondary">
            {{ item.actor_name || '未知操作者' }} · {{ formatDateTime(item.created_at) }}
          </span>
          <div class="ml-auto flex items-center gap-2">
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-3 py-1 text-xs text-text-primary transition hover:bg-ink-600"
              @click="toggleDiff(item.revision_no)"
            >
              {{ expandedNo === item.revision_no ? '收起差异' : '查看差异' }}
            </button>
            <button
              type="button"
              class="rounded-md border border-amber-500/40 px-3 py-1 text-xs text-amber-300 transition hover:bg-amber-500/10 disabled:opacity-50"
              :disabled="rollingNo !== null"
              @click="doRollback(item.revision_no)"
            >
              {{ rollingNo === item.revision_no ? '回滚中…' : '回滚' }}
            </button>
          </div>
        </div>

        <p v-if="item.note" class="mt-1 text-xs text-text-secondary">备注：{{ item.note }}</p>
        <div v-if="item.change_summary?.length" class="mt-1 flex flex-wrap gap-1">
          <span
            v-for="field in item.change_summary"
            :key="field"
            class="rounded bg-ink-800 px-1.5 py-0.5 text-[11px] text-text-secondary"
          >
            {{ fieldLabel(field) }}
          </span>
        </div>

        <!-- 差异详情：字段级 before → after -->
        <div v-if="expandedNo === item.revision_no" class="mt-3 rounded-md bg-ink-900/60 p-3">
          <p v-if="diffLoading" class="text-xs text-text-secondary">加载差异中…</p>
          <template v-else-if="diff">
            <p v-if="diff.changes.length === 0" class="text-xs text-text-secondary">
              该版本为初始版本或没有字段变化
            </p>
            <table v-else class="w-full table-fixed border-collapse text-xs">
              <thead>
                <tr class="text-left text-text-secondary">
                  <th class="w-32 pb-1 font-normal">字段</th>
                  <th class="pb-1 font-normal">回滚前</th>
                  <th class="pb-1 font-normal">该版本</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="change in diff.changes" :key="change.field" class="align-top">
                  <td class="py-1 pr-2 text-text-secondary">{{ fieldLabel(change.field) }}</td>
                  <td class="py-1 pr-2 break-all text-text-secondary line-through">
                    {{ formatValue(change.before) }}
                  </td>
                  <td class="py-1 break-all text-text-primary">{{ formatValue(change.after) }}</td>
                </tr>
              </tbody>
            </table>
          </template>
        </div>
      </li>
    </ul>
  </section>
</template>
