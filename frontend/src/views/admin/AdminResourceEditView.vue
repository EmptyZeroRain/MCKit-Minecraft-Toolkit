<script setup lang="ts">
// 资源新建/编辑（需求 6.3；TODO §6 依赖 / 前置 / 冲突与发布版本）
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import FormField from '@/components/common/FormField.vue'
import { useUiStore } from '@/stores/ui'
import {
  getResource,
  createResource,
  updateResource,
  replaceResourceReleases,
} from '@/api/admin-resources'
import type { ResourcePayload } from '@/types/resource'
import {
  DEPENDENCY_TYPES,
  type ResourceDependencyType,
} from '@/types/resource'
import {
  RESOURCE_TYPES,
  RESOURCE_TYPE_LABELS,
  LOADER_TYPES,
  LOADER_LABELS,
  RESOURCE_STATUSES,
} from '@/types/common'
import { ApiError } from '@/types/api'
import { slugify } from '@/utils/format'
import { DEPENDENCY_TYPE_LABELS, sortReleases } from '@/utils/resource'
import { ALL_VERSIONS, loadReleaseVersions } from '@/utils/mc-versions'
import {
  dependencyToForm,
  formToDependency,
  formToReleasePayload,
  releaseToForm,
  validateDependencyForms,
  validateReleaseForms,
  validateVersionText,
  type DependencyForm,
  type ReleaseForm,
} from '@/utils/resource-form'

const route = useRoute()
const router = useRouter()
const ui = useUiStore()

const isEdit = computed(() => !!route.params.id)
const id = computed(() => (route.params.id as string) || '')

const form = reactive<ResourcePayload>({
  slug: '',
  name: '',
  summary: '',
  description: '',
  resource_type: 'mod',
  cover_image_url: null,
  author_name: null,
  source_platform: null,
  source_url: '',
  license: null,
  game_edition: 'java',
  supported_versions: [],
  loader_types: [],
  dependencies: [],
  install_guide: '',
  tags: [],
  compatibility_note: '',
  status: 'draft',
})

// 简单输入辅助：数组字段以逗号分隔
const supportedVersionsText = ref('')
const tagsText = ref('')

// 依赖与发布版本以结构化行编辑，提交前统一转换为接口载荷
const dependencies = ref<DependencyForm[]>([])
const releases = ref<ReleaseForm[]>([])

// 版本候选（官方正式版清单，供 datalist 提示；失败内部回退兜底清单）
const releaseVersionOptions = ref<string[]>([])

const errors = reactive<Record<string, string>>({})
const submitting = ref(false)
const loading = ref(isEdit.value)

function syncArrays() {
  form.supported_versions = supportedVersionsText.value
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
  form.tags = tagsText.value
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function fillArrayTexts() {
  supportedVersionsText.value = (form.supported_versions ?? []).join(', ')
  tagsText.value = (form.tags ?? []).join(', ')
}

async function loadForEdit() {
  if (!isEdit.value) return
  loading.value = true
  try {
    const item = await getResource(id.value)
    if (!item) {
      ui.error('资源不存在')
      router.replace('/admin/resources')
      return
    }
    Object.assign(form, {
      slug: item.slug,
      name: item.name,
      summary: item.summary,
      description: item.description ?? '',
      resource_type: item.resource_type,
      cover_image_url: item.cover_image_url ?? null,
      author_name: item.author_name ?? null,
      source_platform: item.source_platform ?? null,
      source_url: item.source_url ?? '',
      license: item.license ?? null,
      game_edition: item.game_edition,
      supported_versions: item.supported_versions ?? [],
      loader_types: item.loader_types ?? [],
      dependencies: item.dependencies ?? [],
      install_guide: item.install_guide ?? '',
      tags: item.tags ?? [],
      compatibility_note: item.compatibility_note ?? '',
      status: item.status,
    })
    fillArrayTexts()
    dependencies.value = (item.dependencies ?? []).map(dependencyToForm)
    releases.value = sortReleases(item.releases).map(releaseToForm)
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadForEdit)

// 版本候选加载（与详情页/筛选共用同一数据源与兜底策略）。
onMounted(async () => {
  releaseVersionOptions.value = await loadReleaseVersions()
})

function addDependency() {
  dependencies.value.push({ name: '', type: 'dependency', resource_url: '', required: false, note: '' })
}

function removeDependency(index: number) {
  dependencies.value.splice(index, 1)
}

function addRelease() {
  releases.value.push({
    version_label: '',
    game_versions_text: '',
    loader_types_text: '',
    source_url: '',
    released_at: '',
    notes: '',
    is_latest: releases.value.length === 0,
  })
}

function removeRelease(index: number) {
  releases.value.splice(index, 1)
}

/** 「最新」为互斥标记：勾选一个即取消其余，避免出现多个最新。 */
function setLatest(index: number) {
  releases.value.forEach((r, i) => {
    r.is_latest = i === index
  })
}

function validate(): boolean {
  errors.name = ''
  errors.slug = ''
  errors.summary = ''
  errors.source_url = ''
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
  if (!form.source_url.trim()) {
    errors.source_url = '原始来源 URL 必填'
    ok = false
  } else if (!/^https?:\/\/\S+$/i.test(form.source_url.trim())) {
    errors.source_url = '必须是以 http(s):// 开头的链接'
    ok = false
  }
  return ok
}

function onNameChange() {
  if (!isEdit.value && !form.slug.trim()) {
    form.slug = slugify(form.name)
  }
}

async function handleSubmit() {
  syncArrays()
  if (!validate() || submitting.value) return

  const versionError = validateVersionText(supportedVersionsText.value)
  if (versionError) {
    ui.error(versionError)
    return
  }
  const depError = validateDependencyForms(dependencies.value)
  if (depError) {
    ui.error(depError)
    return
  }
  const releaseError = validateReleaseForms(releases.value)
  if (releaseError) {
    ui.error(releaseError)
    return
  }
  form.dependencies = dependencies.value.map(formToDependency)

  submitting.value = true
  try {
    let resourceId = id.value
    if (isEdit.value) {
      await updateResource(id.value, { ...form })
      ui.success('资源已更新')
    } else {
      const created = await createResource({ ...form })
      resourceId = created.id
      ui.success('资源已创建')
    }
    // 发布版本以完整列表为准整体替换（含清空），保证与表单一致。
    await replaceResourceReleases(resourceId, releases.value.map(formToReleasePayload))
    router.push('/admin/resources')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '保存失败')
  } finally {
    submitting.value = false
  }
}

const dependencyTypeOptions = DEPENDENCY_TYPES.map((t: ResourceDependencyType) => ({
  value: t,
  label: DEPENDENCY_TYPE_LABELS[t],
}))
</script>

<template>
  <div class="mx-auto max-w-4xl">
    <div v-if="loading" class="card h-[50vh] animate-pulse" />
    <form v-else class="card space-y-5 p-6" novalidate @submit.prevent="handleSubmit">
      <div class="grid gap-4 sm:grid-cols-2">
        <FormField label="名称" required :error="errors.name">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.name"
              type="text"
              @change="onNameChange"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>

        <FormField label="Slug" required :error="errors.slug" hint="URL 友好标识，唯一">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.slug"
              type="text"
              placeholder="my-mod"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
      </div>

      <FormField label="简介" required :error="errors.summary">
        <template #default="{ id }">
          <textarea
            :id="id"
            v-model="form.summary"
            rows="2"
            maxlength="500"
            class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <div class="grid gap-4 sm:grid-cols-2">
        <FormField label="资源类型" required>
          <template #default="{ id }">
            <select
              :id="id"
              v-model="form.resource_type"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option v-for="t in RESOURCE_TYPES" :key="t" :value="t">{{ RESOURCE_TYPE_LABELS[t] }}</option>
            </select>
          </template>
        </FormField>

        <FormField label="状态">
          <template #default="{ id }">
            <select
              :id="id"
              v-model="form.status"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option v-for="s in RESOURCE_STATUSES" :key="s" :value="s">{{ s }}</option>
            </select>
          </template>
        </FormField>
      </div>

      <FormField label="原始来源 URL" required :error="errors.source_url" hint="必须为 http(s):// 外部链接，仅做索引跳转">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.source_url"
            type="url"
            placeholder="https://www.curseforge.com/…"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <div class="grid gap-4 sm:grid-cols-3">
        <FormField label="作者">
          <template #default="{ id }">
            <input :id="id" v-model="form.author_name" type="text" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
        <FormField label="来源平台">
          <template #default="{ id }">
            <input :id="id" v-model="form.source_platform" type="text" placeholder="CurseForge / Modrinth…" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
        <FormField label="许可证">
          <template #default="{ id }">
            <input :id="id" v-model="form.license" type="text" placeholder="MIT / GPL…" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <FormField
          label="支持版本"
          :hint="`逗号分隔，如 1.20.1, 1.21；填 ${ALL_VERSIONS} 表示适用于全部版本；留空表示未标明（详情页显示「未验证」）`"
        >
          <template #default="{ id }">
            <input
              :id="id"
              v-model="supportedVersionsText"
              type="text"
              list="mc-version-options"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <FormField label="标签" hint="逗号分隔">
          <template #default="{ id }">
            <input :id="id" v-model="tagsText" type="text" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
          </template>
        </FormField>
      </div>

      <datalist id="mc-version-options">
        <option v-for="v in releaseVersionOptions" :key="v" :value="v" />
      </datalist>

      <FormField label="加载器">
        <template #default="{ id }">
          <div :id="id" class="flex flex-wrap gap-2">
            <label
              v-for="l in LOADER_TYPES"
              :key="l"
              class="inline-flex cursor-pointer items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs transition"
              :class="
                form.loader_types.includes(l)
                  ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                  : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
              "
            >
              <input
                v-model="form.loader_types"
                type="checkbox"
                :value="l"
                class="sr-only"
              />
              {{ LOADER_LABELS[l] }}
            </label>
          </div>
        </template>
      </FormField>

      <FormField label="兼容性核验备注（可选）" hint="说明结论依据或例外情况，详情页原样展示；不填写不影响展示">
        <template #default="{ id }">
          <textarea
            :id="id"
            v-model="form.compatibility_note"
            rows="2"
            maxlength="1000"
            class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <!-- 依赖 / 前置 Mod / 已知冲突（TODO §6） -->
      <FormField label="依赖与冲突" hint="区分「前置 Mod / 依赖 / 已知冲突」，详情页会分组展示">
        <template #default="{ id }">
          <div :id="id" class="space-y-3">
            <div
              v-for="(dep, i) in dependencies"
              :key="i"
              class="rounded-md border border-border-dark bg-ink-800/40 p-3"
            >
              <div class="grid gap-3 sm:grid-cols-2">
                <label class="block">
                  <span class="mb-1 block text-xs text-text-muted">名称</span>
                  <input v-model="dep.name" type="text" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
                <label class="block">
                  <span class="mb-1 block text-xs text-text-muted">关系</span>
                  <select v-model="dep.type" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none">
                    <option v-for="opt in dependencyTypeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                  </select>
                </label>
                <label class="block sm:col-span-2">
                  <span class="mb-1 block text-xs text-text-muted">链接（可选，http(s)）</span>
                  <input v-model="dep.resource_url" type="url" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
                <label class="block sm:col-span-2">
                  <span class="mb-1 block text-xs text-text-muted">说明（可选）</span>
                  <input v-model="dep.note" type="text" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
              </div>
              <div class="mt-3 flex items-center justify-between">
                <label v-if="dep.type !== 'conflict'" class="inline-flex items-center gap-2 text-xs text-text-secondary">
                  <input v-model="dep.required" type="checkbox" class="h-3.5 w-3.5" />
                  必需（缺少则无法运行）
                </label>
                <span v-else class="text-xs text-text-muted">冲突项不标记必需</span>
                <button type="button" class="text-xs text-red-300 hover:underline" @click="removeDependency(i)">移除</button>
              </div>
            </div>
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
              @click="addDependency"
            >
              + 添加依赖
            </button>
          </div>
        </template>
      </FormField>

      <!-- 发布版本（TODO §6） -->
      <FormField label="发布版本" hint="同一资源在不同游戏版本 / 加载器下的发布记录；留空表示清空已有记录">
        <template #default="{ id }">
          <div :id="id" class="space-y-3">
            <div
              v-for="(rel, i) in releases"
              :key="i"
              class="rounded-md border border-border-dark bg-ink-800/40 p-3"
            >
              <div class="grid gap-3 sm:grid-cols-2">
                <label class="block">
                  <span class="mb-1 block text-xs text-text-muted">版本标签</span>
                  <input v-model="rel.version_label" type="text" placeholder="15.2.0.51" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
                <label class="block">
                  <span class="mb-1 block text-xs text-text-muted">发布时间（可选）</span>
                  <input v-model="rel.released_at" type="date" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
                <label class="block">
                  <span class="mb-1 block text-xs text-text-muted">适用游戏版本（逗号分隔）</span>
                  <input v-model="rel.game_versions_text" type="text" list="mc-version-options" :placeholder="`1.20.1, 1.21 或 ${ALL_VERSIONS}`" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
                <label class="block">
                  <span class="mb-1 block text-xs text-text-muted">加载器（逗号分隔）</span>
                  <input v-model="rel.loader_types_text" type="text" :placeholder="LOADER_TYPES.join(', ')" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
                <label class="block sm:col-span-2">
                  <span class="mb-1 block text-xs text-text-muted">发布页链接（必填，http(s)）</span>
                  <input v-model="rel.source_url" type="url" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
                <label class="block sm:col-span-2">
                  <span class="mb-1 block text-xs text-text-muted">备注（可选）</span>
                  <input v-model="rel.notes" type="text" class="h-9 w-full rounded-md border border-border-dark bg-ink-900 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
                </label>
              </div>
              <div class="mt-3 flex items-center justify-between">
                <label class="inline-flex items-center gap-2 text-xs text-text-secondary">
                  <input
                    type="checkbox"
                    class="h-3.5 w-3.5"
                    :checked="rel.is_latest"
                    @change="setLatest(i)"
                  />
                  标记为最新发布（互斥）
                </label>
                <button type="button" class="text-xs text-red-300 hover:underline" @click="removeRelease(i)">移除</button>
              </div>
            </div>
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
              @click="addRelease"
            >
              + 添加发布版本
            </button>
          </div>
        </template>
      </FormField>

      <FormField label="封面 URL（可选）">
        <template #default="{ id }">
          <input :id="id" v-model="form.cover_image_url" type="url" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <FormField label="详细介绍（Markdown）">
        <template #default="{ id }">
          <textarea :id="id" v-model="form.description" rows="6" class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <FormField label="安装说明（Markdown）">
        <template #default="{ id }">
          <textarea :id="id" v-model="form.install_guide" rows="4" class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </template>
      </FormField>

      <div class="flex gap-3 pt-2">
        <button
          type="submit"
          class="rounded-md bg-grass-600 px-6 py-2.5 font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="submitting"
        >
          {{ submitting ? '保存中…' : isEdit ? '保存修改' : '创建资源' }}
        </button>
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-6 py-2.5 text-text-primary transition hover:bg-ink-600"
          @click="router.push('/admin/resources')"
        >
          取消
        </button>
      </div>
    </form>
  </div>
</template>
