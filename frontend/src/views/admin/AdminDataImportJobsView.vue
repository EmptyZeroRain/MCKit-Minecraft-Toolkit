<script setup lang="ts">
// 数据导入任务（内容健康独立 Tab）
//
// 从「数据质量监控」拆出：质量概览回答「资料现在有没有问题」，
// 导入任务回答「数据是怎么进来的、哪次同步失败过」，两者关注点不同，
// 因此各自成 Tab，避免质量页滚动过长。
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { listDataImportJobs } from '@/api/admin-data-quality'
import { ApiError } from '@/types/api'
import type { DataImportJob } from '@/types/data-quality'
import { isJobFailed } from '@/utils/data-quality'
import { timeAgo } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{ dataType: string; page: number; pageSize: number }>({
  dataType: '',
  page: 1,
  pageSize: 15,
})
const jobs = ref<DataImportJob[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

/** 展开错误明细的任务 id（同一时刻只展开一行） */
const expanded = ref('')

async function loadJobs() {
  loading.value = true
  error.value = ''
  try {
    const res = await listDataImportJobs({
      data_type: filters.dataType || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    jobs.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function onFilter() {
  filters.page = 1
  loadJobs()
}

function onPageChange(p: number) {
  filters.page = p
  loadJobs()
}

function toggleExpand(id: string) {
  expanded.value = expanded.value === id ? '' : id
}

function refresh() {
  loadJobs()
  ui.success('已刷新')
}

onMounted(loadJobs)
</script>

<template>
  <div class="space-y-5">
    <div class="flex items-center justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-text-primary">数据导入任务</h1>
        <p class="mt-1 text-sm text-text-muted">
          记录每次数据同步的来源文件与结果；失败行会汇总为错误明细，可展开逐条查看。
        </p>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary hover:bg-ink-600 disabled:opacity-50"
        :disabled="loading"
        @click="refresh"
      >
        刷新
      </button>
    </div>

    <ListShell title="导入记录" :loading="loading" :error="error" @retry="loadJobs">
      <div class="mb-4">
        <select
          v-model="filters.dataType"
          aria-label="按数据类型过滤"
          class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部类型</option>
          <option value="items">items</option>
          <option value="recipes">recipes</option>
          <option value="commands">commands</option>
          <option value="tools">tools</option>
        </select>
      </div>

      <div v-if="jobs.length" class="card overflow-x-auto">
        <table class="w-full min-w-[860px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">时间</th>
              <th scope="col" class="px-4 py-2.5">类型</th>
              <th scope="col" class="px-4 py-2.5">来源文件</th>
              <th scope="col" class="px-4 py-2.5">总数</th>
              <th scope="col" class="px-4 py-2.5">新增 / 更新</th>
              <th scope="col" class="px-4 py-2.5">失败</th>
              <th scope="col" class="px-4 py-2.5">执行者</th>
              <th scope="col" class="px-4 py-2.5 text-right">详情</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <template v-for="job in jobs" :key="job.id">
              <tr class="hover:bg-ink-800/60">
                <td class="px-4 py-2.5 text-xs text-text-muted">{{ timeAgo(job.created_at) }}</td>
                <td class="px-4 py-2.5">
                  <span class="text-text-primary">{{ job.data_type }}</span>
                  <span v-if="job.dry_run" class="ml-2 rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">dry-run</span>
                </td>
                <td class="px-4 py-2.5 max-w-[220px] truncate text-xs text-text-secondary" :title="job.source_file">
                  {{ job.source_file || '—' }}
                </td>
                <td class="px-4 py-2.5 text-text-secondary">{{ job.total_count }}</td>
                <td class="px-4 py-2.5 text-text-secondary">{{ job.created_count }} / {{ job.updated_count }}</td>
                <td class="px-4 py-2.5">
                  <span :class="isJobFailed(job) ? 'text-danger-mc' : 'text-text-muted'">{{ job.failed_count }}</span>
                </td>
                <td class="px-4 py-2.5 text-xs text-text-muted">{{ job.operator || '—' }}</td>
                <td class="px-4 py-2.5 text-right">
                  <button
                    v-if="job.error_summary?.length"
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-xs text-grass-400 hover:bg-ink-700"
                    :aria-expanded="expanded === job.id"
                    @click="toggleExpand(job.id)"
                  >
                    {{ expanded === job.id ? '收起' : `错误 (${job.error_summary.length})` }}
                  </button>
                  <span v-else class="text-xs text-text-muted">—</span>
                </td>
              </tr>
              <tr v-if="expanded === job.id" class="bg-ink-900">
                <td colspan="8" class="px-4 py-3">
                  <ul class="space-y-1 text-xs text-text-secondary">
                    <li v-for="(msg, i) in job.error_summary ?? []" :key="i">· {{ msg }}</li>
                  </ul>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无导入任务记录。</div>
    </ListShell>

    <Pagination
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />
  </div>
</template>
