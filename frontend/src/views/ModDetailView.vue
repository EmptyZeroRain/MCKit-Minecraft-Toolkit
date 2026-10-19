<script setup lang="ts">
// Mod 详情（TODO §16）：发布版本、依赖与冲突、反向依赖、来源与许可。
//   依赖 / 冲突只呈现**已记录**关系，逐条标注来源与验证状态，本站不做自动冲突分析；
//   下载入口仅在「官方授权入口 + 已说明授权」同时满足时展示（版权红线）。
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import VersionBadge from '@/components/badges/VersionBadge.vue'
import MarkdownView from '@/components/common/MarkdownView.vue'
import { getMod } from '@/api/public'
import {
  GAME_EDITION_LABELS,
  MOD_CATEGORY_LABELS,
  MOD_DEPENDENCY_KINDS,
  MOD_DEPENDENCY_KIND_LABELS,
  MOD_LOADER_LABELS,
  MOD_RELEASE_TYPE_LABELS,
  MOD_REVIEW_STATUS_LABELS,
  authorizedDownloadUrl,
  categoryClass,
  dependencyKindClass,
  dependencyTargetLabel,
  isExternalUrl,
  loaderClass,
  releaseTypeClass,
  verifiedClass,
  verifiedLabel,
  type ModDetail,
} from '@/types/mods'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'

const route = useRoute()
const router = useRouter()

const slug = computed(() => (route.params.slug as string) || '')
const mod = ref<ModDetail | null>(null)
const loading = ref(true)
const error = ref('')

/** 依赖按类型分组（必需 → 可选 → 内嵌 → 不兼容），空组不展示。 */
const dependencyGroups = computed(() => {
  const current = mod.value
  if (!current) return []
  return MOD_DEPENDENCY_KINDS.map((kind) => ({
    kind,
    label: MOD_DEPENDENCY_KIND_LABELS[kind],
    items: current.dependencies.filter((d) => d.dep_kind === kind),
  })).filter((group) => group.items.length > 0)
})

/** 版本的可下载链接：无授权说明时返回 null，回退为 download_note 文字说明。 */
function versionDownloadUrl(version: ModDetail['versions'][number]): string | null {
  if (!mod.value) return null
  return authorizedDownloadUrl(version, mod.value)
}

async function loadDetail() {
  loading.value = true
  error.value = ''
  mod.value = null
  try {
    const data = await getMod(slug.value)
    mod.value = data
    if (data) {
      applyPageMeta({
        title: `${data.name_zh} · Mod`,
        description: data.summary || `${data.name_zh} 的版本、依赖与兼容性信息。`,
        canonicalPath: `/mods/${data.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    reportToolFailure('mods')
  } finally {
    loading.value = false
  }
}

onMounted(loadDetail)
watch(slug, loadDetail)

function goBack() {
  router.push('/mods')
}
</script>

<template>
  <PageContainer maxWidth="max-w-4xl">
    <Breadcrumbs
      :items="[{ label: 'Mod 与整合包', to: '/mods' }, { label: mod?.name_zh ?? slug }]"
    />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="!mod"
      title="未收录该 Mod"
      description="本站只展示已通过内容审核（published）的 Mod，未通过审核或未收录的内容不会对外可见。"
      action-label="返回 Mod 列表"
      @action="goBack"
    />

    <template v-else>
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span
            v-for="loader in mod.loaders"
            :key="loader"
            class="rounded border px-2 py-0.5 text-xs"
            :class="loaderClass(loader)"
          >
            {{ MOD_LOADER_LABELS[loader] }}
          </span>
          <span
            v-for="category in mod.categories"
            :key="category"
            class="rounded border px-2 py-0.5 text-xs"
            :class="categoryClass(category)"
          >
            {{ MOD_CATEGORY_LABELS[category] }}
          </span>
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ GAME_EDITION_LABELS[mod.game_edition] }}
          </span>
          <VersionBadge v-for="v in mod.mc_versions" :key="v" :version="v" />
        </div>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="text-2xl font-bold text-text-primary md:text-3xl">{{ mod.name_zh }}</h1>
            <p v-if="mod.name_en" class="mt-1 text-sm text-text-muted">{{ mod.name_en }}</p>
            <p v-if="mod.aliases.length" class="mt-1 text-xs text-text-muted">
              别名：{{ mod.aliases.join('、') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <FavoriteButton kind="mod" :slug="mod.slug" :label="mod.name_zh" :url="`/mods/${mod.slug}`" />
            <ShareButton />
          </div>
        </div>
        <p v-if="mod.summary" class="mt-3 text-text-secondary">{{ mod.summary }}</p>
      </header>

      <section v-if="mod.description" class="card mb-6 p-4" aria-labelledby="mod-desc-title">
        <h2 id="mod-desc-title" class="mb-3 text-lg font-semibold text-text-primary">说明</h2>
        <MarkdownView :content="mod.description" />
      </section>

      <!-- 发布版本 -->
      <section class="card mb-6 p-4" aria-labelledby="mod-versions-title">
        <h2 id="mod-versions-title" class="mb-3 text-lg font-semibold text-text-primary">发布版本</h2>
        <p v-if="!mod.versions.length" class="text-sm text-text-muted">该 Mod 没有已收录的发布版本。</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark text-xs text-text-muted">
              <tr>
                <th scope="col" class="py-2 pr-4 font-medium">版本</th>
                <th scope="col" class="py-2 pr-4 font-medium">适用版本</th>
                <th scope="col" class="py-2 pr-4 font-medium">加载器</th>
                <th scope="col" class="py-2 pr-4 font-medium">下载入口</th>
                <th scope="col" class="py-2 font-medium">核验</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="version in mod.versions" :key="version.id">
                <td class="py-2 pr-4 align-top">
                  <span class="font-medium text-text-primary">{{ version.version }}</span>
                  <span
                    class="ml-2 rounded border px-1.5 py-0.5 text-xs"
                    :class="releaseTypeClass(version.release_type)"
                  >
                    {{ MOD_RELEASE_TYPE_LABELS[version.release_type] }}
                  </span>
                  <code v-if="version.file_name" class="mt-0.5 block break-all font-mono text-xs text-text-muted">
                    {{ version.file_name }}
                  </code>
                </td>
                <td class="py-2 pr-4 align-top text-text-secondary">
                  {{ version.mc_versions.join('、') || '—' }}
                </td>
                <td class="py-2 pr-4 align-top text-text-secondary">
                  {{ version.loaders.map((l) => MOD_LOADER_LABELS[l]).join('、') || '—' }}
                </td>
                <td class="py-2 pr-4 align-top">
                  <a
                    v-if="versionDownloadUrl(version)"
                    :href="versionDownloadUrl(version) as string"
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    class="text-grass-300 hover:underline"
                  >
                    前往下载（官方 / 授权入口）
                  </a>
                  <span v-else class="text-text-muted">
                    {{ version.download_note || '未提供授权下载入口' }}
                  </span>
                  <a
                    v-if="isExternalUrl(version.source_url)"
                    :href="version.source_url as string"
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    class="mt-0.5 block text-xs text-grass-300 hover:underline"
                  >
                    版本来源
                  </a>
                </td>
                <td class="py-2 align-top">
                  <span class="rounded border px-2 py-0.5 text-xs" :class="verifiedClass(version.verified)">
                    {{ verifiedLabel(version.verified) }}
                  </span>
                  <span v-if="version.verify_note" class="mt-0.5 block text-xs text-text-muted">
                    {{ version.verify_note }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="mt-2 text-xs text-text-muted">
          下载仅指向官方 / 作者授权入口，本站不镜像分发文件；无授权说明的版本只给出文字说明，不生成下载链接。
        </p>
      </section>

      <!-- 依赖与冲突 -->
      <section class="card mb-6 p-4" aria-labelledby="mod-deps-title">
        <h2 id="mod-deps-title" class="mb-3 text-lg font-semibold text-text-primary">依赖与冲突</h2>
        <p v-if="!dependencyGroups.length" class="text-sm text-text-muted">
          该 Mod 没有已记录的依赖或冲突关系。
        </p>
        <div v-else class="space-y-4">
          <div v-for="group in dependencyGroups" :key="group.kind">
            <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-text-primary">
              <span class="rounded border px-2 py-0.5 text-xs" :class="dependencyKindClass(group.kind)">
                {{ group.label }}
              </span>
              <span class="text-xs font-normal text-text-muted">{{ group.items.length }} 条</span>
            </h3>
            <ul class="space-y-2">
              <li
                v-for="dep in group.items"
                :key="dep.id"
                class="rounded-md border border-border-dark bg-ink-800 p-3 text-sm"
              >
                <div class="flex flex-wrap items-center gap-2">
                  <RouterLink
                    v-if="dep.target_mod"
                    :to="dep.target_mod.url"
                    class="font-medium text-grass-300 hover:underline"
                  >
                    {{ dependencyTargetLabel(dep) }}
                  </RouterLink>
                  <span v-else class="font-medium text-text-primary">{{ dependencyTargetLabel(dep) }}</span>
                  <span class="rounded border px-2 py-0.5 text-xs" :class="verifiedClass(dep.verified)">
                    {{ verifiedLabel(dep.verified) }}
                  </span>
                </div>
                <p v-if="dep.version_note" class="mt-1 text-xs text-text-muted">
                  适用版本：{{ dep.version_note }}
                </p>
                <p v-if="dep.note" class="mt-1 text-text-secondary">{{ dep.note }}</p>
                <a
                  v-if="isExternalUrl(dep.source_url)"
                  :href="dep.source_url as string"
                  target="_blank"
                  rel="noopener noreferrer nofollow"
                  class="mt-1 inline-block text-xs text-grass-300 hover:underline"
                >
                  来源
                </a>
              </li>
            </ul>
          </div>
        </div>
        <p class="mt-2 text-xs text-text-muted">
          本站只呈现已记录并标注来源的依赖 / 冲突，未做自动冲突分析；请以各 Mod 官方说明为准。
        </p>
      </section>

      <!-- 反向依赖 -->
      <section v-if="mod.dependents.length" class="card mb-6 p-4" aria-labelledby="mod-dependents-title">
        <h2 id="mod-dependents-title" class="mb-3 text-lg font-semibold text-text-primary">
          谁依赖了它
        </h2>
        <ul class="space-y-2">
          <li
            v-for="(dep, idx) in mod.dependents"
            :key="`${dep.mod.slug}-${idx}`"
            class="rounded-md border border-border-dark bg-ink-800 p-3 text-sm"
          >
            <div class="flex flex-wrap items-center gap-2">
              <RouterLink :to="dep.mod.url" class="font-medium text-grass-300 hover:underline">
                {{ dep.mod.name_zh }}
              </RouterLink>
              <span class="rounded border px-2 py-0.5 text-xs" :class="dependencyKindClass(dep.dep_kind)">
                {{ MOD_DEPENDENCY_KIND_LABELS[dep.dep_kind] }}
              </span>
              <span class="rounded border px-2 py-0.5 text-xs" :class="verifiedClass(dep.verified)">
                {{ verifiedLabel(dep.verified) }}
              </span>
            </div>
            <p v-if="dep.version_note" class="mt-1 text-xs text-text-muted">
              适用版本：{{ dep.version_note }}
            </p>
            <p v-if="dep.note" class="mt-1 text-text-secondary">{{ dep.note }}</p>
            <a
              v-if="isExternalUrl(dep.source_url)"
              :href="dep.source_url as string"
              target="_blank"
              rel="noopener noreferrer nofollow"
              class="mt-1 inline-block text-xs text-grass-300 hover:underline"
            >
              来源
            </a>
          </li>
        </ul>
      </section>

      <!-- 来源与许可 -->
      <section class="card p-4 text-sm" aria-labelledby="mod-source-title">
        <h2 id="mod-source-title" class="mb-2 font-semibold text-text-primary">来源与许可</h2>
        <dl class="space-y-1 text-text-secondary">
          <div v-if="isExternalUrl(mod.homepage_url)" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">主页：</dt>
            <dd>
              <a
                :href="mod.homepage_url as string"
                target="_blank"
                rel="noopener noreferrer nofollow"
                class="text-grass-300 hover:underline"
              >
                前往主页
              </a>
            </dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">来源：</dt>
            <dd>
              <a
                v-if="isExternalUrl(mod.source_url)"
                :href="mod.source_url as string"
                target="_blank"
                rel="noopener noreferrer nofollow"
                class="text-grass-300 hover:underline"
              >
                查看来源
              </a>
              <span v-else>未标注</span>
            </dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">授权协议：</dt>
            <dd>{{ mod.license || '未标注' }}</dd>
          </div>
          <div v-if="mod.license_note" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">授权说明：</dt>
            <dd>{{ mod.license_note }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">资料核验日期：</dt>
            <dd>{{ formatDateTime(mod.last_verified_at) || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">数据版本标注：</dt>
            <dd>{{ mod.data_version || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">内容审核：</dt>
            <dd>{{ MOD_REVIEW_STATUS_LABELS[mod.review_status] }}</dd>
          </div>
        </dl>
        <div class="mt-3 flex flex-wrap gap-3">
          <CopyButton
            tool="mods"
            :text="mod.slug"
            label="复制 slug"
            success-message="已复制 Mod slug"
          />
        </div>
        <p class="mt-2 text-xs text-text-muted">
          本站不为未授权内容提供下载链接，也不伪造下载直链；下载入口仅在明确标注授权协议时展示。
        </p>
      </section>
    </template>
  </PageContainer>
</template>
