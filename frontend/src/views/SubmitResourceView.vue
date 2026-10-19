<script setup lang="ts">
// 资源投稿（用户提交结构化资源 → 人工审核 → 转为资源）。
//
// 与「问题反馈」的边界：反馈是自由文本纠错 / 举报；这里是新资源的结构化投稿，
// 字段与资源中心对齐，审核通过后由管理员一键转为 resources 记录。
// 需登录（路由 requiresUser），提交需接受授权条款（版本由后端强校验）。
import { computed, onMounted, reactive, ref } from 'vue'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import FormField from '@/components/common/FormField.vue'
import Pagination from '@/components/common/Pagination.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { listMySubmissions, submitResource } from '@/api/user'
import { ApiError } from '@/types/api'
import { useUiStore } from '@/stores/ui'
import { useUserAuthStore } from '@/stores/user-auth'
import { LOADER_LABELS, LOADER_TYPES, RESOURCE_TYPES, RESOURCE_TYPE_LABELS } from '@/types/common'
import type { GameEdition, LoaderType, ResourceType } from '@/types/common'
import {
  MAX_PENDING_SUBMISSIONS,
  SUBMISSION_BADGE_STATUS,
  SUBMISSION_LICENSE_VERSION,
  SUBMISSION_STATUS_LABELS,
  type ResourceSubmission,
} from '@/types/submission'
import { formatDateTime, joinList } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '资源投稿',
  canonicalPath: '/submit-resource',
  robot: 'noindex,nofollow',
})

const ui = useUiStore()
const userAuth = useUserAuthStore()

const form = reactive({
  name: '',
  summary: '',
  description: '',
  resource_type: 'mod' as ResourceType,
  author_name: '',
  source_platform: '',
  source_url: '',
  license: '',
  game_edition: 'java' as GameEdition,
  versions: '',
  loaders: [] as LoaderType[],
  install_guide: '',
  note: '',
  license_accepted: false,
})

const errors = reactive<Record<string, string>>({})
const submitting = ref(false)

const mine = ref<ResourceSubmission[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const totalPages = ref(1)
const loadingList = ref(false)
const listError = ref('')

const pendingCount = computed(() => mine.value.filter((s) => s.status === 'pending').length)

/** 逗号 / 空格 / 换行分隔的版本串 → 去重数组。 */
function parseVersions(raw: string): string[] {
  return Array.from(
    new Set(
      raw
        .split(/[\s,，]+/)
        .map((v) => v.trim())
        .filter(Boolean),
    ),
  )
}

function toggleLoader(l: LoaderType) {
  const i = form.loaders.indexOf(l)
  if (i >= 0) form.loaders.splice(i, 1)
  else form.loaders.push(l)
}

function validate(): boolean {
  for (const k of Object.keys(errors)) delete errors[k]
  let ok = true

  if (!form.name.trim()) {
    errors.name = '请填写资源名称'
    ok = false
  } else if (form.name.trim().length > 200) {
    errors.name = '资源名称不能超过 200 字'
    ok = false
  }
  if (!form.summary.trim()) {
    errors.summary = '请填写一句话简介'
    ok = false
  } else if (form.summary.trim().length > 500) {
    errors.summary = '简介不能超过 500 字'
    ok = false
  }
  if (!form.source_url.trim()) {
    errors.source_url = '请填写资源原始来源链接'
    ok = false
  } else if (!/^https?:\/\/\S+$/i.test(form.source_url.trim())) {
    errors.source_url = '来源链接必须以 http(s):// 开头'
    ok = false
  }
  if (!form.license_accepted) {
    errors.license = '请先阅读并接受投稿授权条款'
    ok = false
  }
  return ok
}

async function loadMine() {
  loadingList.value = true
  listError.value = ''
  try {
    const res = await listMySubmissions({ page: page.value, page_size: pageSize })
    mine.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / pageSize))
  } catch (err) {
    listError.value = err instanceof ApiError ? err.message : '加载我的投稿失败'
  } finally {
    loadingList.value = false
  }
}

async function handleSubmit() {
  if (!validate() || submitting.value) return
  submitting.value = true
  try {
    await submitResource({
      name: form.name.trim(),
      summary: form.summary.trim(),
      description: form.description.trim() || undefined,
      resource_type: form.resource_type,
      author_name: form.author_name.trim() || undefined,
      source_platform: form.source_platform.trim() || undefined,
      source_url: form.source_url.trim(),
      license: form.license.trim() || undefined,
      game_edition: form.game_edition,
      supported_versions: parseVersions(form.versions),
      loader_types: form.loaders,
      install_guide: form.install_guide.trim() || undefined,
      note: form.note.trim() || undefined,
      license_version: SUBMISSION_LICENSE_VERSION,
    })
    ui.success('投稿已提交，等待管理员审核')
    // 重置易变字段，保留来源 / 类型等便于连续投稿。
    form.name = ''
    form.summary = ''
    form.description = ''
    form.source_url = ''
    form.versions = ''
    form.install_guide = ''
    form.note = ''
    form.license_accepted = false
    page.value = 1
    await loadMine()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '投稿失败，请稍后重试')
  } finally {
    submitting.value = false
  }
}

function onPageChange(p: number) {
  page.value = p
  loadMine()
}

onMounted(loadMine)
</script>

<template>
  <PageContainer maxWidth="max-w-3xl">
    <Breadcrumbs :items="[{ label: '资源投稿' }]" />
    <div class="mb-6">
      <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">资源投稿</h1>
      <p class="text-text-secondary">
        提交你推荐的 Mod / 整合包 / 资源包等资源，经管理员审核通过后由本站整理入库。
        请填写资源的<strong class="text-text-primary">原始来源链接</strong>，并确认拥有转载授权。
      </p>
    </div>

    <p v-if="!userAuth.isLoggedIn" class="mb-6 rounded-card border border-amber-mc/40 bg-amber-mc/10 px-4 py-3 text-sm text-amber-mc">
      投稿需要登录，请先
      <RouterLink class="underline" to="/login?redirect=/submit-resource">登录 / 注册</RouterLink>。
    </p>

    <form class="card space-y-5 p-6" novalidate @submit.prevent="handleSubmit">
      <FormField label="资源名称" required :error="errors.name">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.name"
            type="text"
            placeholder="如：JEI 物品管理器"
            maxlength="200"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <FormField label="一句话简介" required :error="errors.summary">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.summary"
            type="text"
            placeholder="用一句话说明这个资源的作用"
            maxlength="500"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
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
              <option v-for="t in RESOURCE_TYPES" :key="t" :value="t">
                {{ RESOURCE_TYPE_LABELS[t] }}
              </option>
            </select>
          </template>
        </FormField>

        <FormField label="游戏版本" hint="当前以 Java 版为主">
          <template #default="{ id }">
            <select
              :id="id"
              v-model="form.game_edition"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option value="java">Java 版</option>
              <option value="bedrock">基岩版</option>
            </select>
          </template>
        </FormField>
      </div>

      <FormField label="原始来源链接" required :error="errors.source_url" hint="资源作者发布页 / 官方下载页，仅接受 http(s) 链接">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.source_url"
            type="url"
            placeholder="https://…"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <div class="grid gap-4 sm:grid-cols-2">
        <FormField label="作者 / 团队（可选）">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.author_name"
              type="text"
              placeholder="资源作者或团队名"
              maxlength="160"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>

        <FormField label="来源平台（可选）" hint="如 CurseForge / Modrinth / GitHub">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.source_platform"
              type="text"
              placeholder="CurseForge"
              maxlength="100"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
      </div>

      <FormField label="支持的游戏版本（可选）" hint="多个版本用逗号或空格分隔，如 1.20.1 1.20.4">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.versions"
            type="text"
            placeholder="1.20.1 1.20.4"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <div>
        <label class="mb-1.5 block text-sm font-medium text-text-primary">适用加载器（可选）</label>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="l in LOADER_TYPES"
            :key="l"
            type="button"
            class="rounded-full border px-3 py-1.5 text-xs transition"
            :class="
              form.loaders.includes(l)
                ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
            "
            @click="toggleLoader(l)"
          >
            {{ LOADER_LABELS[l] }}
          </button>
        </div>
      </div>

      <FormField label="许可协议（可选）" hint="如 MIT / GPL-3.0 / 作者授权转载；不确定可留空由管理员核验">
        <template #default="{ id }">
          <input
            :id="id"
            v-model="form.license"
            type="text"
            placeholder="MIT"
            maxlength="160"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <FormField label="资源描述（可选）" hint="介绍功能、玩法或注意事项，审核通过后会作为资源正文">
        <template #default="{ id }">
          <textarea
            :id="id"
            v-model="form.description"
            rows="5"
            placeholder="介绍这个资源的功能与特色"
            class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <FormField label="安装说明（可选）">
        <template #default="{ id }">
          <textarea
            :id="id"
            v-model="form.install_guide"
            rows="3"
            placeholder="安装 / 使用步骤"
            class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <FormField label="给审核者的说明（可选）" hint="如来源授权情况、与其他资源的关系">
        <template #default="{ id }">
          <textarea
            :id="id"
            v-model="form.note"
            rows="2"
            maxlength="2000"
            class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </template>
      </FormField>

      <div class="rounded-md border border-border-dark bg-ink-900/60 p-4 text-sm">
        <label class="flex items-start gap-2 text-text-secondary">
          <input v-model="form.license_accepted" type="checkbox" class="mt-0.5 h-4 w-4 accent-grass-500" />
          <span>
            我确认投稿内容为本人原创或已获得作者授权，并同意本站按开源许可整理、转载与再发布该资源
            （授权条款版本 {{ SUBMISSION_LICENSE_VERSION }}）。
          </span>
        </label>
        <p v-if="errors.license" class="mt-2 text-xs text-danger-mc">{{ errors.license }}</p>
      </div>

      <button
        type="submit"
        class="w-full rounded-md bg-grass-600 py-3 font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="submitting || !userAuth.isLoggedIn"
      >
        {{ submitting ? '提交中…' : '提交投稿' }}
      </button>
    </form>

    <!-- 我的投稿 -->
    <section class="mt-10">
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-lg font-bold text-text-primary">我的投稿（{{ total }}）</h2>
        <span class="text-xs text-text-muted">
          待审核 {{ pendingCount }} / {{ MAX_PENDING_SUBMISSIONS }}
        </span>
      </div>

      <p v-if="listError" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc" role="alert">
        {{ listError }}
      </p>

      <div v-if="mine.length" class="space-y-3">
        <article v-for="item in mine" :key="item.id" class="card p-4">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <div class="min-w-0">
              <h3 class="font-medium text-text-primary">{{ item.name }}</h3>
              <p class="mt-0.5 text-xs text-text-muted">
                {{ RESOURCE_TYPE_LABELS[item.resource_type] }} ·
                {{ joinList(item.loader_types.map((l) => LOADER_LABELS[l])) || '未标注加载器' }} ·
                提交于 {{ formatDateTime(item.created_at) }}
              </p>
            </div>
            <StatusBadge
              :status="SUBMISSION_BADGE_STATUS[item.status]"
              :label="SUBMISSION_STATUS_LABELS[item.status]"
            />
          </div>
          <p class="mt-2 text-sm text-text-secondary">{{ item.summary }}</p>
          <a
            :href="item.source_url"
            target="_blank"
            rel="noopener noreferrer nofollow"
            class="mt-1 block break-all font-mono text-xs text-grass-300 hover:underline"
          >
            {{ item.source_url }}
          </a>
          <p v-if="item.review_note" class="mt-2 rounded border border-border-dark bg-ink-900/60 px-3 py-2 text-xs text-text-secondary">
            审核备注：{{ item.review_note }}
            <template v-if="item.reviewed_at"> · {{ formatDateTime(item.reviewed_at) }}</template>
          </p>
          <p v-if="item.status === 'approved'" class="mt-2 text-xs text-grass-400">
            已通过审核，资源已创建为草稿，管理员核验后会出现在资源中心。
          </p>
        </article>
      </div>
      <EmptyState
        v-else-if="!loadingList"
        title="还没有投稿"
        description="填写上方表单即可提交你的第一个资源。"
      />

      <Pagination
        v-if="total > pageSize"
        class="mt-4"
        :page="page"
        :page-size="pageSize"
        :total="total"
        :total-pages="totalPages"
        @change="onPageChange"
      />
    </section>
  </PageContainer>
</template>
