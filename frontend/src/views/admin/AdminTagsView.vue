<script setup lang="ts">
// 标签管理（需求 6.3）
import { onMounted, reactive, ref } from 'vue'
import FormField from '@/components/common/FormField.vue'
import { useUiStore } from '@/stores/ui'
import { listTags, createTag, updateTag, deleteTag } from '@/api/admin-meta'
import type { Tag } from '@/types/category'
import { ApiError } from '@/types/api'

const ui = useUiStore()
const items = ref<Tag[]>([])
const loading = ref(false)
const error = ref('')
const editing = ref<Tag | null>(null)
const form = reactive({ name: '', slug: '' })
const submitting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listTags({ page: 1, page_size: 100 })
    items.value = res.items
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function startEdit(item: Tag) {
  editing.value = item
  form.name = item.name
  form.slug = item.slug
}

function resetForm() {
  editing.value = null
  form.name = ''
  form.slug = ''
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
      await updateTag(editing.value.id, { ...form })
      ui.success('标签已更新')
    } else {
      await createTag({ ...form })
      ui.success('标签已创建')
    }
    resetForm()
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

async function handleDelete(item: Tag) {
  try {
    await deleteTag(item.id)
    ui.success('标签已删除')
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="grid gap-6 lg:grid-cols-2">
    <div class="card p-5">
      <h2 class="mb-4 font-semibold text-text-primary">{{ editing ? `编辑标签：${editing.name}` : '新建标签' }}</h2>
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

    <div class="card overflow-hidden">
      <div class="border-b border-border-dark px-4 py-3">
        <h2 class="font-semibold text-text-primary">标签列表</h2>
      </div>
      <div v-if="error" role="alert" class="p-6 text-sm text-danger-mc">{{ error }}</div>
      <ul v-else-if="items.length" class="max-h-[60vh] divide-y divide-border-dark overflow-y-auto">
        <li v-for="item in items" :key="item.id" class="flex items-center justify-between px-4 py-2.5 text-sm hover:bg-ink-800/60">
          <div>
            <span class="text-text-primary">{{ item.name }}</span>
            <span class="ml-2 font-mono text-xs text-text-muted">{{ item.slug }}</span>
          </div>
          <div class="flex gap-1.5 text-xs">
            <button type="button" class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700" @click="startEdit(item)">编辑</button>
            <button type="button" class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10" @click="handleDelete(item)">删除</button>
          </div>
        </li>
      </ul>
      <div v-else-if="!loading" class="p-10 text-center text-sm text-text-muted">暂无标签。</div>
    </div>
  </div>
</template>