<script setup lang="ts">
// 开发文档详情（TODO §21）：要点正文 + 关键接口速查 + 官方文档与来源链接 + 同平台相关条目。
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import MarkdownView from '@/components/common/MarkdownView.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import VersionBadge from '@/components/badges/VersionBadge.vue'
import { getDevDoc } from '@/api/dev'
import {
  apiEntryText,
  devApiKindLabel,
  devCategoryLabel,
  devLoaderLabel,
  type DevDocDetail,
} from '@/types/devdoc'
import { ApiError } from '@/types/api'
import { formatDate, formatVersionLabel, formatVersionList } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'

const route = useRoute()
const router = useRouter()

const slug = computed(() => (route.params.slug as string) || '')
const detail = ref<DevDocDetail | null>(null)
const loading = ref(true)
const error = ref('')

/** 外部链接安全兜底：非 http(s) 一律降级为不可点击。 */
function externalUrl(url: string | null | undefined): string {
  return url && /^https?:\/\//i.test(url) ? url : '#'
}

/** 接口速查可复制文本：整篇接口一次复制，便于粘进代码或笔记。 */
const apiEntriesText = computed(() =>
  (detail.value?.doc.api_entries ?? []).map((e) => apiEntryText(e)).join('\n'),
)

/** 正文可复制文本（Markdown 原文，不是渲染后的 HTML）。 */
const bodyText = computed(() => detail.value?.doc.body_md ?? '')

async function loadDetail() {
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    const data = await getDevDoc(slug.value)
    detail.value = data
    if (data?.doc) {
      applyPageMeta({
        title: `${data.doc.title_zh} · 开发文档`,
        description: data.doc.summary || `${data.doc.title_zh} 开发要点与关键接口。`,
        canonicalPath: `/dev/${data.doc.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    reportToolFailure('dev-docs')
  } finally {
    loading.value = false
  }
}

onMounted(loadDetail)
watch(slug, loadDetail)
</script>

<template>
  <PageContainer maxWidth="max-w-4xl">
    <Breadcrumbs :items="[{ label: '开发文档', to: '/dev' }, { label: detail?.doc.title_zh ?? slug }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="!detail"
      title="未收录该开发文档"
      description="本站开发文档仅包含已核验条目，未收录内容不会伪造查询结果。"
      action-label="返回开发文档"
      @action="router.push('/dev')"
    />

    <template v-else>
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2 text-xs">
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-text-muted">
            {{ devLoaderLabel(detail.doc.loader) }}
          </span>
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-text-muted">
            {{ devCategoryLabel(detail.doc.category) }}
          </span>
          <VersionBadge v-for="v in detail.doc.mc_versions" :key="v" :version="formatVersionLabel(v)" />
        </div>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 class="mb-1 text-2xl font-bold text-text-primary md:text-3xl">
              {{ detail.doc.title_zh }}
            </h1>
            <p v-if="detail.doc.title_en" class="text-sm text-text-muted">{{ detail.doc.title_en }}</p>
          </div>
          <ShareButton />
        </div>
        <p v-if="detail.doc.summary" class="mt-3 text-text-secondary">{{ detail.doc.summary }}</p>
      </header>

      <!-- 关键接口速查（结构化存储，可整段复制） -->
      <section
        v-if="detail.doc.api_entries.length"
        class="card mb-6 p-4"
        aria-labelledby="dev-api-title"
      >
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 id="dev-api-title" class="text-sm font-semibold text-text-primary">关键接口速查</h2>
          <CopyButton
            tool="dev-docs"
            :text="apiEntriesText"
            label="复制全部接口"
            success-message="已复制全部接口"
          />
        </div>
        <ul class="divide-y divide-border-dark text-sm">
          <li
            v-for="entry in detail.doc.api_entries"
            :key="entry.name + entry.signature"
            class="flex flex-wrap items-baseline gap-x-2 gap-y-1 py-2"
          >
            <span class="rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">
              {{ devApiKindLabel(entry.kind) }}
            </span>
            <code class="font-mono text-xs text-grass-300">{{ entry.name }}</code>
            <code v-if="entry.signature" class="font-mono text-xs text-text-secondary">
              {{ entry.signature }}
            </code>
            <span v-if="entry.note" class="text-xs text-text-muted">{{ entry.note }}</span>
          </li>
        </ul>
      </section>

      <!-- 正文 -->
      <section class="card mb-6 p-5" aria-labelledby="dev-body-title">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 id="dev-body-title" class="text-sm font-semibold text-text-primary">要点与示例</h2>
          <CopyButton
            tool="dev-docs"
            :text="bodyText"
            label="复制正文（Markdown）"
            success-message="已复制正文"
          />
        </div>
        <MarkdownView :content="detail.doc.body_md" />
      </section>

      <!-- 来源与校验信息（TODO §9：校验资料来源与版本适用范围） -->
      <section class="card mb-6 p-4 text-sm" aria-labelledby="dev-source-title">
        <h2 id="dev-source-title" class="mb-3 text-sm font-semibold text-text-primary">资料来源与校验</h2>
        <dl class="grid gap-2 sm:grid-cols-2">
          <div class="flex flex-wrap items-baseline gap-2">
            <dt class="text-text-muted">官方文档</dt>
            <dd>
              <a
                :href="externalUrl(detail.doc.official_url)"
                class="text-grass-300 hover:underline"
                target="_blank"
                rel="noopener noreferrer"
              >
                {{ detail.doc.official_url || '—' }}
              </a>
            </dd>
          </div>
          <div class="flex flex-wrap items-baseline gap-2">
            <dt class="text-text-muted">可核验来源</dt>
            <dd>
              <a
                :href="externalUrl(detail.doc.source_url)"
                class="text-grass-300 hover:underline"
                target="_blank"
                rel="noopener noreferrer"
              >
                {{ detail.doc.source_url || '—' }}
              </a>
            </dd>
          </div>
          <div class="flex flex-wrap items-baseline gap-2">
            <dt class="text-text-muted">适用版本</dt>
            <dd class="text-text-secondary">{{ formatVersionList(detail.doc.mc_versions, 8) }}</dd>
          </div>
          <div class="flex flex-wrap items-baseline gap-2">
            <dt class="text-text-muted">数据版本</dt>
            <dd class="text-text-secondary">{{ detail.doc.data_version || '—' }}</dd>
          </div>
          <div class="flex flex-wrap items-baseline gap-2">
            <dt class="text-text-muted">最近校验</dt>
            <dd class="text-text-secondary">{{ formatDate(detail.doc.last_verified_at) }}</dd>
          </div>
          <div class="flex flex-wrap items-baseline gap-2">
            <dt class="text-text-muted">兼容平台</dt>
            <dd class="text-text-secondary">
              {{ detail.doc.loaders.map(devLoaderLabel).join('、') || devLoaderLabel(detail.doc.loader) }}
            </dd>
          </div>
        </dl>
        <p class="mt-3 text-xs text-text-muted">
          文档要点为本站提炼，可能存在滞后；版本敏感细节请以官方文档与目标版本的实测结果为准。
        </p>
      </section>

      <!-- 相关条目 -->
      <section v-if="detail.related.length" aria-labelledby="dev-related-title">
        <h2 id="dev-related-title" class="mb-3 text-sm font-semibold text-text-primary">
          同平台其它条目
        </h2>
        <ul class="grid gap-3 sm:grid-cols-2">
          <li v-for="item in detail.related" :key="item.slug" class="card p-4">
            <RouterLink
              :to="{ name: 'dev-doc-detail', params: { slug: item.slug } }"
              class="text-sm font-medium text-grass-300 hover:underline"
            >
              {{ item.title_zh }}
            </RouterLink>
            <p class="mt-1 line-clamp-2 text-xs text-text-secondary">{{ item.summary }}</p>
          </li>
        </ul>
      </section>
    </template>
  </PageContainer>
</template>
