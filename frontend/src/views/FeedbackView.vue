<script setup lang="ts">
// 反馈表单（需求 5.10）：类型、标题、内容、关联 URL、联系方式
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import FormField from '@/components/common/FormField.vue'
import { submitFeedback } from '@/api/public'
import { useUiStore } from '@/stores/ui'
import type { FeedbackTargetKind, FeedbackType } from '@/types/common'
import {
  FEEDBACK_LICENSE_TYPES,
  FEEDBACK_LICENSE_VERSION,
  FEEDBACK_TARGET_KINDS,
  FEEDBACK_TARGET_KIND_LABELS,
  FEEDBACK_TYPES,
  FEEDBACK_TYPE_LABELS,
} from '@/types/common'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '问题反馈',
  canonicalPath: '/feedback',
})

const ui = useUiStore()
const router = useRouter()

const form = reactive({
  feedback_type: 'bug' as FeedbackType,
  title: '',
  content: '',
  related_url: '',
  contact: '',
  target_kind: 'other' as FeedbackTargetKind,
  target_ref: '',
  license_accepted: false,
})

const errors = reactive<Record<string, string>>({})
const submitting = ref(false)

/** 当前类型是否为「向本站贡献内容」的投稿（需接受授权条款）。 */
const needsLicense = computed(() => FEEDBACK_LICENSE_TYPES.includes(form.feedback_type))

function validate(): boolean {
  errors.title = ''
  errors.content = ''
  errors.related_url = ''
  errors.license = ''
  let ok = true

  if (!form.title.trim()) {
    errors.title = '请填写标题'
    ok = false
  } else if (form.title.trim().length > 200) {
    errors.title = '标题不能超过 200 字'
    ok = false
  }
  if (!form.content.trim()) {
    errors.content = '请填写反馈内容'
    ok = false
  } else if (form.content.trim().length > 2000) {
    errors.content = '内容不能超过 2000 字'
    ok = false
  }
  if (form.related_url && !/^https?:\/\/\S+$/i.test(form.related_url.trim())) {
    errors.related_url = '关联 URL 必须以 http(s):// 开头'
    ok = false
  }
  if (needsLicense.value && !form.license_accepted) {
    errors.license = '请先阅读并接受投稿授权条款'
    ok = false
  }
  return ok
}

async function handleSubmit() {
  if (!validate() || submitting.value) return
  submitting.value = true
  try {
    await submitFeedback({
      feedback_type: form.feedback_type,
      title: form.title.trim(),
      content: form.content.trim(),
      related_url: form.related_url.trim() || undefined,
      contact: form.contact.trim() || undefined,
      target_kind: form.target_kind,
      target_ref: form.target_ref.trim() || undefined,
      license_version: needsLicense.value ? FEEDBACK_LICENSE_VERSION : undefined,
    })
    ui.success('反馈已提交，感谢您的帮助！')
    router.push('/')
  } catch (err) {
    ui.error(err instanceof Error ? err.message : '提交失败，请稍后重试')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <PageContainer maxWidth="max-w-2xl">
    <Breadcrumbs :items="[{ label: '问题反馈' }]" />
    <div class="mb-6">
      <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">问题反馈</h1>
      <p class="text-text-secondary">遇到的问题、失效链接或侵权通知都可以在这里提交。联系信息为选填。</p>
    </div>

    <form class="card space-y-5 p-6" novalidate @submit.prevent="handleSubmit">
      <div>
        <label class="mb-1.5 block text-sm font-medium text-text-primary">反馈类型</label>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="t in FEEDBACK_TYPES"
            :key="t"
            type="button"
            class="rounded-full border px-3 py-1.5 text-xs transition"
            :class="
              form.feedback_type === t
                ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
            "
            @click="form.feedback_type = t"
          >
            {{ FEEDBACK_TYPE_LABELS[t] }}
          </button>
        </div>
      </div>

      <FormField label="标题" required :error="errors.title">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.title"
            type="text"
            placeholder="简要描述你的问题"
            maxlength="200"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <FormField label="反馈内容" required :error="errors.content">
        <template #default="{ id }">
          <textarea
            :id="id"
            v-model="form.content"
            rows="5"
            placeholder="请尽量详细描述问题发生的步骤或现象"
            maxlength="2000"
            class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <FormField label="关联资源 URL（可选）" :error="errors.related_url" hint="如举报某资源失效，可粘贴该资源的原始来源链接">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.related_url"
            type="url"
            placeholder="https://…"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <div class="grid gap-4 sm:grid-cols-2">
        <FormField label="关联资料类型（可选）" hint="纠错 / 补充信息时指出问题所在的资料条目">
          <template #default="{ id }">
            <select
              :id="id"
              v-model="form.target_kind"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option v-for="k in FEEDBACK_TARGET_KINDS" :key="k" :value="k">
                {{ FEEDBACK_TARGET_KIND_LABELS[k] }}
              </option>
            </select>
          </template>
        </FormField>

        <FormField label="关联标识（可选）" hint="如物品 ID（minecraft:cow）或资料 slug">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.target_ref"
              type="text"
              placeholder="minecraft:… 或 slug"
              maxlength="200"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
      </div>

      <FormField label="联系方式（可选）" hint="需要跟进处理结果时填写邮箱或其他联系方式">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.contact"
            type="text"
            placeholder="邮箱 / 其他联系方式"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <div v-if="needsLicense" class="rounded-md border border-border-dark bg-ink-900/60 p-4 text-sm">
        <label class="flex items-start gap-2 text-text-secondary">
          <input v-model="form.license_accepted" type="checkbox" class="mt-0.5 h-4 w-4 accent-grass-500" />
          <span>
            我确认投稿内容为本人原创或已获得授权，并同意本站按开源许可整理、转载与再发布该内容
            （授权条款版本 {{ FEEDBACK_LICENSE_VERSION }}）。
          </span>
        </label>
        <p v-if="errors.license" class="mt-2 text-xs text-red-400">{{ errors.license }}</p>
      </div>

      <button
        type="submit"
        class="w-full rounded-md bg-grass-600 py-3 font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="submitting"
      >
        {{ submitting ? '提交中…' : '提交反馈' }}
      </button>
    </form>
  </PageContainer>
</template>