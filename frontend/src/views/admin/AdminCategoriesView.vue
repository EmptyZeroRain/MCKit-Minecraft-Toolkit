<script setup lang="ts">
// 分类管理（需求 6.3）
import { onMounted, reactive, ref } from 'vue'
import FormField from '@/components/common/FormField.vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import { useUiStore } from '@/stores/ui'
import {
  listCategories,
  createCategory,
  updateCategory,
  deleteCategory,
} from '@/api/admin-meta'
import type { Category } from '@/types/category'
import type { AdminTableColumn } from '@/types/admin'
import { ApiError } from '@/types/api'

const ui = useUiStore()

const columns: AdminTableColumn[] = [
  { key: 'name', label: '名称' },
  { key: 'slug', label: 'Slug' },
  { key: 'content_type', label: '类型' },
  { key: 'actions', label: '操作', align: 'right', width: '140px' },
]

const items = ref<Category[]>([])
const loading = ref(false)
const error = ref('')

const filterType = ref<'' | 'tool' | 'resource' | 'wiki'>('')

const editing = ref<Category | null>(null)
const form = reactive({
  name: '',
  slug: '',
  content_type: 'tool' as 'tool' | 'resource' | 'wiki',
  sort_order: 0,
})
const submitting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = await listCategories({ content_type: filterType.value || undefined })
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function startEdit(item: Category) {
  editing.value = item
  form.name = item.name
  form.slug = item.slug
  form.content_type = item.content_type
  form.sort_order = item.sort_order
}

function resetForm() {
  editing.value = null
  form.name = ''
  form.slug = ''
  form.content_type = 'tool'
  form.sort_order = 0
}

async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim() || !form.slug.trim()) {
    ui.warning('请填写名称与 slug')
    return
  }
  submitting.value = true
  try {
    if (editing.value) {
      await updateCategory(editing.value.id, { ...form })
      ui.success('分类已更新')
    } else {
      await createCategory({ ...form })
      ui.success('分类已创建')
    }
    resetForm()
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

async function handleDelete(item: Category) {
  try {
    await deleteCategory(item.id)
    ui.success('分类已删除')
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="grid gap-6 lg:grid-cols-5">
    <!-- 表单 -->
    <div class="lg:col-span-2">
      <div class="card p-5">
        <h2 class="mb-4 font-semibold text-text-primary">
          {{ editing ? `编辑分类：${editing.name}` : '新建分类' }}
        </h2>
        <form class="space-y-4" novalidate @submit.prevent="handleSubmit">
          <FormField label="名称" required>
            <template #default="{ id }">
              <input :id="id" v-model="form.name" type="text" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
            </template>
          </FormField>
          <FormField label="Slug" required>
            <template #default="{ id }">
              <input :id="id" v-model="form.slug" type="text" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
            </template>
          </FormField>
          <FormField label="内容类型">
            <template #default="{ id }">
              <select :id="id" v-model="form.content_type" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none">
                <option value="tool">工具</option>
                <option value="resource">资源</option>
                <option value="wiki">百科</option>
              </select>
            </template>
          </FormField>
          <FormField label="排序权重">
            <template #default="{ id }">
              <input :id="id" v-model.number="form.sort_order" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
            </template>
          </FormField>
          <div class="flex gap-2">
            <button
              type="submit"
              class="flex-1 rounded-md bg-grass-600 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
              :disabled="submitting"
            >
              {{ submitting ? '保存中…' : editing ? '保存修改' : '创建' }}
            </button>
            <button
              v-if="editing"
              type="button"
              class="rounded-md border border-border-light px-4 text-sm text-text-secondary hover:bg-ink-700"
              @click="resetForm"
            >
              取消
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- 列表 -->
    <div class="lg:col-span-3">
      <div class="card overflow-hidden">
        <div class="flex items-center justify-between border-b border-border-dark px-4 py-3">
          <h2 class="font-semibold text-text-primary">分类列表</h2>
          <select
            v-model="filterType"
            class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            @change="load"
          >
            <option value="">全部类型</option>
            <option value="tool">工具</option>
            <option value="resource">资源</option>
            <option value="wiki">百科</option>
          </select>
        </div>

        <div v-if="error" role="alert" class="p-6 text-sm text-danger-mc">{{ error }}</div>
        <AdminTable
          v-else
          :columns="columns"
          :rows="items"
          :loading="loading"
          row-key="id"
          plain
          empty-text="暂无分类。"
        >
          <template #cell-slug="{ row }">
            <span class="font-mono text-xs text-text-secondary">{{ row.slug }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex justify-end gap-1.5 text-xs">
              <button type="button" class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700" @click="startEdit(row)">编辑</button>
              <button type="button" class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10" @click="handleDelete(row)">删除</button>
            </div>
          </template>
        </AdminTable>
      </div>
    </div>
  </div>
</template>