<script setup lang="ts">
// 资源详情（需求 5.7；TODO §6 兼容性与来源信息）：
// 兼容性结论（未验证原则）、发布版本、依赖分组、来源/许可/作者、外链检查状态
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import MarkdownView from '@/components/common/MarkdownView.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import ResourceTypeBadge from '@/components/badges/ResourceTypeBadge.vue'
import VersionBadge from '@/components/badges/VersionBadge.vue'
import LoaderBadge from '@/components/badges/LoaderBadge.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { getResource } from '@/api/public'
import type { ResourceDetail } from '@/types/resource'
import { ApiError } from '@/types/api'
import { formatDate, formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { searchEntry } from '@/utils/related-search'
import { ALL_VERSIONS_LABEL } from '@/types/version'
import { loadReleaseVersions } from '@/utils/mc-versions'
import {
  compatibilitySummary,
  dependencyGroups,
  linkStatusSummary,
  safeExternalUrl,
  sortReleases,
} from '@/utils/resource'

const route = useRoute()

const slug = computed(() => (route.params.slug as string) || '')
const resource = ref<ResourceDetail | null>(null)
const loading = ref(true)
const error = ref('')

/** 兼容性结论：无可靠数据依据时明确「未验证」，不把缺失数据当作兼容。 */
const compat = computed(() =>
  resource.value
    ? compatibilitySummary(resource.value, resource.value.compatibility)
    : null,
)

/** 发布版本（同一资源的不同发布版本）。 */
const releases = computed(() => sortReleases(resource.value?.releases))

/** 依赖按「前置 Mod / 依赖 / 已知冲突」分组。 */
const depGroups = computed(() => dependencyGroups(resource.value?.dependencies))

/** 外链检查状态（客观呈现，不额外推断「安全」）。 */
const linkStatus = computed(() => linkStatusSummary(resource.value?.link_status))

// 声明「所有版本」时可按官方正式版清单展开完整列表（TODO §6-C）
const showAllVersions = ref(false)
const allReleaseVersions = ref<string[]>([])

async function toggleAllVersions() {
  showAllVersions.value = !showAllVersions.value
  if (showAllVersions.value && allReleaseVersions.value.length === 0) {
    allReleaseVersions.value = await loadReleaseVersions()
  }
}

/**
 * 关联检索入口（TODO §3）。
 * 依赖项的 resource_url 为外部直链；站内「相关整合包」无法从自由结构
 * 的 dependencies 可靠反查，故以「按名称检索资源」入口呈现，不伪造整合包直链。
 */
const relatedLinks = computed(() => {
  const name = resource.value?.name
  if (!name) return null
  return {
    modpack: searchEntry(name, 'resource'),
    item: searchEntry(name, 'item'),
    wiki: searchEntry(name, 'wiki'),
  }
})

async function loadResource() {
  loading.value = true
  error.value = ''
  resource.value = null
  try {
    resource.value = await getResource(slug.value)
    if (resource.value) {
      applyPageMeta({
        title: resource.value.name,
        description: resource.value.summary || `${resource.value.name} - 资源详情`,
        canonicalPath: `/resources/${resource.value.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(loadResource)
watch(slug, loadResource)

const linkStatusToneClass: Record<string, string> = {
  ok: 'border-grass-500/40 bg-grass-500/10 text-grass-300',
  warn: 'border-amber-500/40 bg-amber-500/10 text-amber-300',
  bad: 'border-red-500/40 bg-red-500/10 text-red-300',
  muted: 'border-border-light bg-ink-700 text-text-muted',
}
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '资源中心', to: '/resources' }, { label: resource?.name ?? slug }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadResource" />
    <EmptyState v-else-if="!resource" title="资源不存在" description="该资源可能已下架或删除。" />

    <template v-else-if="resource">
      <div class="mb-6 grid gap-6 md:grid-cols-3">
        <!-- 封面 -->
        <div class="md:col-span-1">
          <div class="overflow-hidden rounded-card border border-border-dark bg-ink-800">
            <img
              v-if="resource.cover_image_url"
              :src="resource.cover_image_url"
              :alt="resource.name"
              class="aspect-[16/9] w-full object-cover"
            />
            <img v-else src="/placeholder-cover.svg" alt="" class="aspect-[16/9] w-full object-cover" />
          </div>
        </div>

        <!-- 基本信息 -->
        <div class="md:col-span-2">
          <div class="mb-2 flex flex-wrap items-center gap-2">
            <ResourceTypeBadge :type="resource.resource_type" />
            <span v-if="resource.game_edition === 'java'" class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">Java 版</span>
          </div>
          <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">{{ resource.name }}</h1>
          <p class="mb-3 text-text-secondary">{{ resource.summary }}</p>

          <div class="mt-4 flex flex-wrap items-center gap-2">
            <a
              :href="safeExternalUrl(resource.source_url) ?? '#'"
              target="_blank"
              rel="noopener noreferrer nofollow"
              class="inline-flex items-center gap-2 rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
            >
              前往原始来源
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M13.5 6H5.25A2.25 2.25 0 003 8.25v10.5A2.25 2.25 0 005.25 21h10.5A2.25 2.25 0 0018 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25" />
              </svg>
            </a>
            <FavoriteButton kind="resource" :slug="resource.slug" />
          </div>
        </div>
      </div>

      <!-- 来源与许可信息（TODO §6） -->
      <section class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">来源与许可信息</h2>
        <dl class="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
          <div>
            <dt class="text-xs text-text-muted">作者</dt>
            <dd class="text-text-primary">{{ resource.author_name || '—' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-muted">来源平台</dt>
            <dd class="text-text-primary">{{ resource.source_platform || '—' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-muted">许可信息</dt>
            <dd class="text-text-primary">{{ resource.license || '未标明' }}</dd>
          </div>
          <div>
            <dt class="text-xs text-text-muted">外链检查</dt>
            <dd class="flex flex-wrap items-center gap-2">
              <span class="rounded border px-1.5 py-0.5 text-xs" :class="linkStatusToneClass[linkStatus.tone]">
                {{ linkStatus.label }}
              </span>
              <span v-if="resource.last_checked_at" class="text-xs text-text-muted">
                {{ formatDateTime(resource.last_checked_at) }}
              </span>
            </dd>
          </div>
        </dl>
        <p class="mt-3 text-xs text-text-muted">
          来源链接指向外部网站，本站仅做索引；文件安全与内容质量请以原始来源为准。
        </p>
      </section>

      <!-- 兼容性结论（TODO §6：无数据依据即「未验证」） -->
      <section class="card mb-6 p-5">
        <div class="mb-3 flex flex-wrap items-center gap-2">
          <h2 class="font-semibold text-text-primary">兼容性结论</h2>
          <span
            v-if="compat && !compat.verified"
            class="rounded border border-amber-500/40 bg-amber-500/10 px-1.5 py-0.5 text-xs text-amber-300"
          >
            未验证
          </span>
          <span
            v-else-if="compat"
            class="rounded border border-grass-500/40 bg-grass-500/10 px-1.5 py-0.5 text-xs text-grass-300"
          >
            有数据依据
          </span>
        </div>

        <template v-if="compat && compat.verified">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs text-text-muted">适用版本：</span>
            <span
              v-if="compat.allVersions"
              class="rounded border border-grass-500/40 bg-grass-500/10 px-2 py-0.5 text-xs text-grass-300"
            >
              {{ ALL_VERSIONS_LABEL }}
            </span>
            <VersionBadge v-for="v in compat.gameVersions" :key="v" :version="v" />
            <span v-if="!compat.allVersions && !compat.gameVersions.length" class="text-xs text-text-muted">
              未标明
            </span>
          </div>

          <!-- 通配声明：可按官方正式版清单展开完整版本列表 -->
          <div v-if="compat.allVersions" class="mt-2">
            <button
              type="button"
              class="text-xs text-grass-400 hover:underline"
              @click="toggleAllVersions"
            >
              {{
                showAllVersions
                  ? '收起完整版本清单'
                  : `查看完整正式版清单${allReleaseVersions.length ? `（${allReleaseVersions.length}）` : ''}`
              }}
            </button>
            <ul
              v-if="showAllVersions"
              class="mt-2 max-h-64 list-none overflow-y-auto rounded-md border border-border-light bg-ink-800/40 p-2 text-xs text-text-secondary"
            >
              <li v-for="v in allReleaseVersions" :key="v" class="inline-block px-1.5 py-0.5">
                {{ v }}
              </li>
            </ul>
          </div>

          <div class="mt-2 flex flex-wrap items-center gap-2">
            <span class="text-xs text-text-muted">加载器：</span>
            <template v-if="compat.loaderTypes.length">
              <LoaderBadge v-for="l in compat.loaderTypes" :key="l" :loader="l" />
            </template>
            <span v-else class="text-xs text-text-muted">无 / 不适用</span>
          </div>
          <p v-if="compat.sourceLabel" class="mt-3 text-xs text-text-muted">
            结论依据：{{ compat.sourceLabel }}
          </p>
        </template>

        <p v-else-if="compat" class="text-sm text-text-secondary">
          {{ compat.hint }}
        </p>

        <p v-if="compat?.note" class="mt-3 rounded-md border border-border-light bg-ink-700/50 p-3 text-xs text-text-secondary">
          <span class="text-text-muted">核验备注：</span>{{ compat.note }}
        </p>

        <div class="mt-3 text-xs text-text-muted">
          信息更新于 {{ formatDateTime(resource.updated_at) }}
        </div>
      </section>

      <!-- 发布版本（TODO §6：同一资源不同发布版本之间的关联） -->
      <section v-if="releases.length" class="card mb-6 p-5">
        <h2 class="mb-1 font-semibold text-text-primary">发布版本</h2>
        <p class="mb-3 text-xs text-text-muted">同一资源在不同游戏版本 / 加载器下的发布记录，可分别跳转原始发布页。</p>
        <ul class="space-y-3">
          <li
            v-for="rel in releases"
            :key="rel.id || rel.version_label"
            class="rounded-md border border-border-light bg-ink-800/50 p-3"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-mono text-sm text-text-primary">{{ rel.version_label }}</span>
              <span v-if="rel.is_latest" class="rounded bg-grass-500/15 px-1.5 py-0.5 text-xs text-grass-300">最新</span>
              <span v-if="rel.released_at" class="text-xs text-text-muted">{{ formatDate(rel.released_at) }}</span>
            </div>
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <VersionBadge v-for="v in rel.game_versions" :key="v" :version="v" />
              <LoaderBadge v-for="l in rel.loader_types" :key="l" :loader="l" />
            </div>
            <p v-if="rel.notes" class="mt-2 text-xs text-text-secondary">{{ rel.notes }}</p>
            <a
              v-if="safeExternalUrl(rel.source_url)"
              :href="safeExternalUrl(rel.source_url) ?? '#'"
              target="_blank"
              rel="noopener noreferrer nofollow"
              class="mt-2 inline-block text-xs text-grass-400 hover:underline"
            >
              查看该版本发布页
            </a>
          </li>
        </ul>
      </section>

      <!-- 介绍 -->
      <section v-if="resource.description" class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">资源介绍</h2>
        <MarkdownView :content="resource.description" />
      </section>

      <!-- 依赖 / 前置 / 冲突（TODO §6） -->
      <section v-if="depGroups.length" class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">依赖与冲突</h2>
        <div v-for="group in depGroups" :key="group.type" class="mb-4 last:mb-0">
          <h3 class="mb-2 text-sm font-medium" :class="group.type === 'conflict' ? 'text-red-300' : 'text-text-primary'">
            {{ group.label }}
          </h3>
          <ul class="space-y-2 text-sm text-text-secondary">
            <li v-for="(dep, i) in group.items" :key="i" class="rounded-md border border-border-light bg-ink-800/40 p-2.5">
              <div class="flex flex-wrap items-center gap-2">
                <span :class="dep.required ? 'text-text-primary' : 'text-text-muted'">{{ dep.name }}</span>
                <span v-if="group.type === 'conflict'" class="rounded bg-red-500/15 px-1.5 text-xs text-red-300">冲突</span>
                <template v-else>
                  <span v-if="dep.required" class="rounded bg-grass-500/15 px-1.5 text-xs text-grass-300">必需</span>
                  <span v-else class="rounded bg-ink-700 px-1.5 text-xs text-text-muted">可选</span>
                </template>
                <a
                  v-if="safeExternalUrl(dep.resource_url)"
                  :href="safeExternalUrl(dep.resource_url) ?? '#'"
                  target="_blank"
                  rel="noopener noreferrer nofollow"
                  class="text-grass-400 hover:underline"
                >
                  查看
                </a>
              </div>
              <p v-if="dep.note" class="mt-1 text-xs text-text-muted">{{ dep.note }}</p>
            </li>
          </ul>
        </div>
      </section>

      <!-- 安装说明 -->
      <section v-if="resource.install_guide" class="card mb-6 p-5">
        <h2 class="mb-3 font-semibold text-text-primary">安装说明</h2>
        <MarkdownView :content="resource.install_guide" />
      </section>

      <!-- 关联检索入口（TODO §3） -->
      <section v-if="relatedLinks" class="card mb-6 p-5">
        <h2 class="mb-1 font-semibold text-text-primary">相关整合包与资料</h2>
        <p class="mb-3 text-xs text-text-muted">
          依赖数据为自由结构，无法可靠反查站内整合包；以下为按资源名称发起的检索入口，不伪造关联。
        </p>
        <div class="flex flex-wrap gap-2">
          <RouterLink
            :to="relatedLinks.modpack"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
          >
            检索相关整合包 / 资源
          </RouterLink>
          <RouterLink
            :to="relatedLinks.item"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
          >
            检索相关物品
          </RouterLink>
          <RouterLink
            :to="relatedLinks.wiki"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
          >
            检索相关百科
          </RouterLink>
        </div>
      </section>

      <div class="flex justify-center rounded-card border border-dashed border-border-light p-4 text-xs text-text-muted">
        本站仅提供资源索引；文件安全与内容质量请以原始来源为准。
      </div>
    </template>
  </PageContainer>
</template>
