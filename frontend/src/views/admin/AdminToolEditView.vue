<script setup lang="ts">
// 工具新建/编辑（需求 6.3）
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import FormField from '@/components/common/FormField.vue'
import { useUiStore } from '@/stores/ui'
import { getTool, createTool, updateTool } from '@/api/admin-tools'
import type { ToolPayload } from '@/types/tool'
import { TOOL_CATEGORIES } from '@/types/common'
import { ApiError } from '@/types/api'
import { slugify } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const ui = useUiStore()

const isEdit = computed(() => !!route.params.id)
const id = computed(() => (route.params.id as string) || '')

const form = reactive<ToolPayload>({
  slug: '',
  name: '',
  summary: '',
  description: '',
  category: '查询类',
  icon: null,
  tags: [],
  supported_versions: [],
  input_schema: [],
  output_schema: [],
  is_featured: false,
  sort_order: 0,
  status: 'published',
})

const tagsText = ref('')
const versionsText = ref('')
const errors = reactive<Record<string, string>>({})
const submitting = ref(false)
const loading = ref(isEdit.value)

function syncArrays() {
  form.tags = tagsText.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
  form.supported_versions = versionsText.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
}

function fillArrayTexts() {
  tagsText.value = (form.tags ?? []).join(', ')
  versionsText.value = (form.supported_versions ?? []).join(', ')
}

async function loadForEdit() {
  if (!isEdit.value) return
  loading.value = true
  try {
    const item = await getTool(id.value)
    if (!item) {
      ui.error('工具不存在')
      router.replace('/admin/tools')
      return
    }
    Object.assign(form, {
      slug: item.slug,
      name: item.name,
      summary: item.summary,
      description: item.description ?? '',
      category: item.category,
      icon: item.icon ?? null,
      tags: item.tags ?? [],
      supported_versions: item.supported_versions ?? [],
      input_schema: item.input_schema ?? [],
      output_schema: item.output_schema ?? [],
      is_featured: item.is_featured,
      sort_order: item.sort_order,
      status: item.status,
    })
    fillArrayTexts()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadForEdit)

function validate(): boolean {
  errors.name = ''
  errors.slug = ''
  errors.summary = ''
  let ok = true
  if (!form.name.trim()) {
    errors.name = '请输入名称'
    ok = false
  }
  if (!form.slug.trim()) {
    errors.slug = '请输入 slug'
    ok = false
  } else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(form.slug)) {
    errors.slug = 'slug 只能包含小写字母、数字和连字符'
    ok = false
  }
  if (!form.summary.trim()) {
    errors.summary = '请输入简介'
    ok = false
  }
  return ok
}

function onNameChange() {
  if (!isEdit.value && !form.slug.trim()) form.slug = slugify(form.name)
}

async function handleSubmit() {
  syncArrays()
  if (!validate() || submitting.value) return
  submitting.value = true
  try {
    if (isEdit.value) {
      await updateTool(id.value, { ...form })
      ui.success('工具已更新')
    } else {
      await createTool({ ...form })
      ui.success('工具已创建')
    }
    router.push('/admin/tools')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '保存失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-4xl">
    <div v-if="loading" class="card h-[50vh] animate-pulse" />
    <form v-else class="card space-y-5 p-6" novalidate @submit.prevent="handleSubmit">
      <div class="grid gap-4 sm:grid-cols-2">
        <FormField label="名称" required :error="errors.name">
          <template #default="{ id }">
            <input :id="id" v-model="form.name" type="text" @change="onNameChange" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
        <FormField label="Slug" required :error="errors.slug">
          <template #default="{ id }">
            <input :id="id" v-model="form.slug" type="text" placeholder="coordinate" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
      </div>

      <FormField label="简介" required :error="errors.summary">
        <template #default="{ id }">
          <textarea :id="id" v-model="form.summary" rows="2" maxlength="500" class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <div class="grid gap-4 sm:grid-cols-3">
        <FormField label="分类" required>
          <template #default="{ id }">
            <select :id="id" v-model="form.category" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none">
              <option v-for="c in TOOL_CATEGORIES" :key="c" :value="c">{{ c }}</option>
            </select>
          </template>
        </FormField>
        <FormField label="图标（emoji）">
          <template #default="{ id }">
            <input :id="id" v-model="form.icon" type="text" placeholder="🧰" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
        <FormField label="排序权重">
          <template #default="{ id }">
            <input :id="id" v-model.number="form.sort_order" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <FormField label="标签" hint="逗号分隔">
          <template #default="{ id }">
            <input :id="id" v-model="tagsText" type="text" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
        <FormField label="支持版本" hint="逗号分隔">
          <template #default="{ id }">
            <input :id="id" v-model="versionsText" type="text" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
      </div>

      <label class="flex items-center gap-2 text-sm text-text-secondary">
        <input v-model="form.is_featured" type="checkbox" class="h-4 w-4 accent-grass-500" />
        首页推荐
      </label>

      <FormField label="详细介绍（Markdown）">
        <template #default="{ id }">
          <textarea :id="id" v-model="form.description" rows="5" class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <div class="flex gap-3 pt-2">
        <button
          type="submit"
          class="rounded-md bg-grass-600 px-6 py-2.5 font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="submitting"
        >
          {{ submitting ? '保存中…' : isEdit ? '保存修改' : '创建工具' }}
        </button>
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-6 py-2.5 text-text-primary transition hover:bg-ink-600"
          @click="router.push('/admin/tools')"
        >
          取消
        </button>
      </div>
    </form>
  </div>
</template>