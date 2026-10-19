<script setup lang="ts">
// 百科新建/编辑（需求 6.3）
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import FormField from '@/components/common/FormField.vue'
import { useUiStore } from '@/stores/ui'
import { getWiki, createWiki, updateWiki } from '@/api/admin-wiki'
import type { WikiPayload } from '@/types/wiki'
import { ApiError } from '@/types/api'
import { slugify } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const ui = useUiStore()

const isEdit = computed(() => !!route.params.id)
const id = computed(() => (route.params.id as string) || '')

const form = reactive<WikiPayload>({
  slug: '',
  title: '',
  summary: '',
  content: '',
  content_format: 'markdown',
  content_type: 'guide',
  game_edition: 'java',
  game_versions: [],
  tags: [],
  source_references: [],
  status: 'draft',
})

const tagsText = ref('')
const errors = reactive<Record<string, string>>({})
const submitting = ref(false)
const loading = ref(isEdit.value)

function syncArrays() {
  form.tags = tagsText.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
}

function fillArrayTexts() {
  tagsText.value = (form.tags ?? []).join(', ')
}

async function loadForEdit() {
  if (!isEdit.value) return
  loading.value = true
  try {
    const item = await getWiki(id.value)
    if (!item) {
      ui.error('百科条目不存在')
      router.replace('/admin/wiki')
      return
    }
    Object.assign(form, {
      slug: item.slug,
      title: item.title,
      summary: item.summary ?? '',
      content: item.content ?? '',
      content_format: item.content_format,
      content_type: item.content_type,
      game_edition: item.game_edition,
      game_versions: item.game_versions ?? [],
      tags: item.tags ?? [],
      source_references: item.source_references ?? [],
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
  errors.title = ''
  errors.slug = ''
  errors.content = ''
  let ok = true
  if (!form.title.trim()) {
    errors.title = '请输入标题'
    ok = false
  }
  if (!form.slug.trim()) {
    errors.slug = '请输入 slug'
    ok = false
  } else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(form.slug)) {
    errors.slug = 'slug 只能包含小写字母、数字和连字符'
    ok = false
  }
  if (!form.content.trim()) {
    errors.content = '请输入正文内容'
    ok = false
  }
  return ok
}

function onTitleChange() {
  if (!isEdit.value && !form.slug.trim()) form.slug = slugify(form.title)
}

async function handleSubmit() {
  syncArrays()
  if (!validate() || submitting.value) return
  submitting.value = true
  try {
    if (isEdit.value) {
      await updateWiki(id.value, { ...form })
      ui.success('百科已更新')
    } else {
      await createWiki({ ...form })
      ui.success('百科已创建')
    }
    router.push('/admin/wiki')
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
        <FormField label="标题" required :error="errors.title">
          <template #default="{ id }">
            <input :id="id" v-model="form.title" type="text" @change="onTitleChange" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
        <FormField label="Slug" required :error="errors.slug">
          <template #default="{ id }">
            <input :id="id" v-model="form.slug" type="text" placeholder="redstone-basics" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
      </div>

      <FormField label="摘要">
        <template #default="{ id }">
          <textarea :id="id" v-model="form.summary" rows="2" maxlength="500" class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <div class="grid gap-4 sm:grid-cols-3">
        <FormField label="内容类型">
          <template #default="{ id }">
            <select :id="id" v-model="form.content_type" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none">
              <option value="guide">指南</option>
              <option value="concept">概念</option>
              <option value="tutorial">教程</option>
              <option value="reference">参考</option>
              <option value="other">其他</option>
            </select>
          </template>
        </FormField>
        <FormField label="格式">
          <template #default="{ id }">
            <select :id="id" v-model="form.content_format" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none">
              <option value="markdown">Markdown（推荐）</option>
              <option value="html" disabled>HTML（已禁用）</option>
            </select>
          </template>
        </FormField>
        <FormField label="状态">
          <template #default="{ id }">
            <select :id="id" v-model="form.status" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none">
              <option value="draft">草稿</option>
              <option value="published">已发布</option>
            </select>
          </template>
        </FormField>
      </div>

      <FormField label="标签" hint="逗号分隔">
        <template #default="{ id }">
          <input :id="id" v-model="tagsText" type="text" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <FormField label="正文（Markdown）" required :error="errors.content" hint="原始 HTML 会被安全过滤（防 XSS）">
        <template #default="{ id }">
          <textarea :id="id" v-model="form.content" rows="14" class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <div class="flex gap-3 pt-2">
        <button
          type="submit"
          class="rounded-md bg-grass-600 px-6 py-2.5 font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="submitting"
        >
          {{ submitting ? '保存中…' : isEdit ? '保存修改' : '创建百科' }}
        </button>
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-6 py-2.5 text-text-primary transition hover:bg-ink-600"
          @click="router.push('/admin/wiki')"
        >
          取消
        </button>
      </div>
    </form>
  </div>
</template>