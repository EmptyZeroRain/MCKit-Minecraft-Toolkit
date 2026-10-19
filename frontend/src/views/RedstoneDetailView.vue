<script setup lang="ts">
// 红石资料详情（TODO §15）：同一组件按路由 props.kind 渲染元件或机器设计。
//   元件：工作机制、信号说明、适用版本与来源核验；
//   设计：材料清单（含数量区间与关联物品）、搭建步骤、来源与授权（版权红线）。
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
import { getRedstoneComponent, getRedstoneDesign } from '@/api/public'
import {
  GAME_EDITION_LABELS,
  REDSTONE_COMPONENT_CATEGORY_LABELS,
  REDSTONE_DESIGN_PURPOSE_LABELS,
  REDSTONE_DIFFICULTY_LABELS,
  REDSTONE_REVIEW_STATUS_LABELS,
  componentCategoryClass,
  designPurposeClass,
  difficultyClass,
  formatMaterialCount,
  isExternalUrl,
  type RedstoneComponent,
  type RedstoneDesignDetail,
} from '@/types/redstone'
import type { ItemRef } from '@/types/item'
import { resolveRelatedItem } from '@/utils/item-link'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'

const props = withDefaults(defineProps<{ kind?: 'component' | 'design' }>(), {
  kind: 'component',
})

const route = useRoute()
const router = useRouter()

const slug = computed(() => (route.params.slug as string) || '')
const isComponent = computed(() => props.kind === 'component')

const component = ref<RedstoneComponent | null>(null)
const design = ref<RedstoneDesignDetail | null>(null)
const loading = ref(true)
const error = ref('')

/** 材料行：已收录则给出物品链接，未收录回退纯文本（不生成死链）。 */
function materialLink(material: RedstoneDesignDetail['materials'][number]) {
  if (material.item_ref) {
    return { label: material.item_ref.name_zh || material.name_zh, url: material.item_ref.url || null }
  }
  return resolveRelatedItem(
    design.value?.resolved_items as Record<string, ItemRef> | undefined,
    material.item_id,
  )
}

/** 材料的可复制文本：`圆石 ×16–64`。 */
function materialText(material: RedstoneDesignDetail['materials'][number]): string {
  const name = material.name_zh || materialLink(material).label || material.item_id
  return `${name} ×${formatMaterialCount(material.count_min, material.count_max)}`
}

async function loadDetail() {
  loading.value = true
  error.value = ''
  component.value = null
  design.value = null
  try {
    if (isComponent.value) {
      const data = await getRedstoneComponent(slug.value)
      component.value = data
      if (data) {
        applyPageMeta({
          title: `${data.name_zh} · 红石元件`,
          description: data.summary || `${data.name_zh} 的工作机制与信号说明。`,
          canonicalPath: `/redstone/components/${data.slug}`,
        })
      }
    } else {
      const data = await getRedstoneDesign(slug.value)
      design.value = data
      if (data) {
        applyPageMeta({
          title: `${data.name_zh} · 机器设计`,
          description: data.summary || `${data.name_zh} 的材料清单与搭建步骤。`,
          canonicalPath: `/redstone/designs/${data.slug}`,
        })
      }
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    reportToolFailure('redstone')
  } finally {
    loading.value = false
  }
}

onMounted(loadDetail)
watch([slug, () => props.kind], loadDetail)

function goBack() {
  router.push('/redstone')
}
</script>

<template>
  <PageContainer maxWidth="max-w-4xl">
    <Breadcrumbs
      :items="[
        { label: '红石与机器设计', to: '/redstone' },
        {
          label:
            (isComponent ? component?.name_zh : design?.name_zh) ??
            slug,
        },
      ]"
    />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="isComponent && !component"
      title="未收录该红石元件"
      description="本站红石资料库仅包含已核验收录的条目，未收录内容不会伪造查询结果。"
      action-label="返回红石资料"
      @action="goBack"
    />
    <EmptyState
      v-else-if="!isComponent && !design"
      title="未找到该机器设计"
      description="本站只展示已通过内容审核（published）的机器设计，未通过审核或未收录的内容不会对外可见。"
      action-label="返回红石资料"
      @action="goBack"
    />

    <!-- 元件详情 -->
    <template v-else-if="isComponent && component">
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span class="rounded border px-2 py-0.5 text-xs" :class="componentCategoryClass(component.category)">
            {{ REDSTONE_COMPONENT_CATEGORY_LABELS[component.category] }}
          </span>
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ GAME_EDITION_LABELS[component.game_edition] }}
          </span>
          <VersionBadge v-for="v in component.supported_versions" :key="v" :version="v" />
        </div>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="text-2xl font-bold text-text-primary md:text-3xl">{{ component.name_zh }}</h1>
            <p v-if="component.name_en" class="mt-1 text-sm text-text-muted">{{ component.name_en }}</p>
            <p v-if="component.aliases.length" class="mt-1 text-xs text-text-muted">
              别名：{{ component.aliases.join('、') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <ShareButton />
          </div>
        </div>
        <p v-if="component.summary" class="mt-3 text-text-secondary">{{ component.summary }}</p>
      </header>

      <section v-if="component.mechanism_note" class="card mb-6 p-4" aria-labelledby="rs-mech-title">
        <h2 id="rs-mech-title" class="mb-2 text-lg font-semibold text-text-primary">核心工作机制</h2>
        <p class="text-sm text-text-secondary">{{ component.mechanism_note }}</p>
      </section>

      <section v-if="component.signal_note" class="card mb-6 p-4" aria-labelledby="rs-signal-title">
        <h2 id="rs-signal-title" class="mb-2 text-lg font-semibold text-text-primary">信号说明</h2>
        <p class="text-sm text-text-secondary">{{ component.signal_note }}</p>
      </section>

      <section class="card p-4 text-sm" aria-labelledby="rs-source-title">
        <h2 id="rs-source-title" class="mb-2 font-semibold text-text-primary">来源与核验</h2>
        <dl class="space-y-1 text-text-secondary">
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">资料核验日期：</dt>
            <dd>{{ formatDateTime(component.last_verified_at) || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">数据版本标注：</dt>
            <dd>{{ component.data_version || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">来源：</dt>
            <dd>
              <a
                :href="isExternalUrl(component.source_url) ? (component.source_url as string) : '#'"
                target="_blank"
                rel="noopener noreferrer nofollow"
                class="text-grass-300 hover:underline"
              >
                Minecraft Wiki
              </a>
            </dd>
          </div>
        </dl>
        <p class="mt-2 text-xs text-text-muted">
          信号强度、延迟等数值会随版本与连接方式变化，故统一以文字说明，不写死未经核验的数字。
        </p>
      </section>
    </template>

    <!-- 设计详情 -->
    <template v-else-if="!isComponent && design">
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span class="rounded border px-2 py-0.5 text-xs" :class="designPurposeClass(design.purpose)">
            {{ REDSTONE_DESIGN_PURPOSE_LABELS[design.purpose] }}
          </span>
          <span class="rounded border px-2 py-0.5 text-xs" :class="difficultyClass(design.difficulty)">
            难度：{{ REDSTONE_DIFFICULTY_LABELS[design.difficulty] }}
          </span>
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ GAME_EDITION_LABELS[design.game_edition] }}
          </span>
          <VersionBadge v-for="v in design.supported_versions" :key="v" :version="v" />
        </div>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="text-2xl font-bold text-text-primary md:text-3xl">{{ design.name_zh }}</h1>
            <p v-if="design.name_en" class="mt-1 text-sm text-text-muted">{{ design.name_en }}</p>
            <p v-if="design.aliases.length" class="mt-1 text-xs text-text-muted">
              别名：{{ design.aliases.join('、') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <FavoriteButton
              kind="redstone_design"
              :slug="design.slug"
              :label="design.name_zh"
              :url="`/redstone/designs/${design.slug}`"
            />
            <ShareButton />
          </div>
        </div>
        <p v-if="design.summary" class="mt-3 text-text-secondary">{{ design.summary }}</p>
      </header>

      <!-- 材料清单 -->
      <section class="card mb-6 p-4" aria-labelledby="rs-materials-title">
        <h2 id="rs-materials-title" class="mb-3 text-lg font-semibold text-text-primary">材料清单</h2>
        <p v-if="!design.materials.length" class="text-sm text-text-muted">该设计没有已收录的材料清单。</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark text-xs text-text-muted">
              <tr>
                <th scope="col" class="py-2 pr-4 font-medium">材料</th>
                <th scope="col" class="py-2 pr-4 font-medium">数量</th>
                <th scope="col" class="py-2 font-medium">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="(material, idx) in design.materials" :key="`${material.item_id}-${idx}`">
                <td class="py-2 pr-4">
                  <RouterLink
                    v-if="materialLink(material).url"
                    :to="materialLink(material).url as string"
                    class="font-medium text-grass-300 hover:underline"
                  >
                    {{ material.name_zh || materialLink(material).label }}
                  </RouterLink>
                  <span v-else class="text-text-primary">
                    {{ material.name_zh || material.item_id }}
                  </span>
                  <code
                    v-if="!materialLink(material).url && material.item_id"
                    class="mt-0.5 block break-all font-mono text-xs text-text-muted"
                  >
                    {{ material.item_id }}
                  </code>
                </td>
                <td class="py-2 pr-4 whitespace-nowrap text-text-secondary">
                  ×{{ formatMaterialCount(material.count_min, material.count_max) }}
                </td>
                <td class="py-2">
                  <CopyButton
                    tool="redstone"
                    :text="materialText(material)"
                    label="复制材料"
                    success-message="已复制材料信息"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="mt-2 text-xs text-text-muted">
          数量以区间给出（未收录精确配方），实际用量随建造规模变化；未收录物品以中文名展示，不生成物品死链。
        </p>
      </section>

      <!-- 搭建步骤 -->
      <section v-if="design.steps_md" class="card mb-6 p-4" aria-labelledby="rs-steps-title">
        <div class="mb-3 flex items-center justify-between gap-3">
          <h2 id="rs-steps-title" class="text-lg font-semibold text-text-primary">搭建步骤</h2>
          <CopyButton
            tool="redstone"
            :text="design.steps_md"
            label="复制步骤（Markdown）"
            success-message="已复制搭建步骤"
          />
        </div>
        <MarkdownView :content="design.steps_md" />
      </section>

      <!-- 来源与授权 -->
      <section class="card p-4 text-sm" aria-labelledby="rs-design-source-title">
        <h2 id="rs-design-source-title" class="mb-2 font-semibold text-text-primary">来源与授权</h2>
        <dl class="space-y-1 text-text-secondary">
          <div v-if="design.author" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">作者 / 整理：</dt>
            <dd>
              <a
                v-if="isExternalUrl(design.author_url)"
                :href="design.author_url as string"
                target="_blank"
                rel="noopener noreferrer nofollow"
                class="text-grass-300 hover:underline"
              >
                {{ design.author }}
              </a>
              <span v-else>{{ design.author }}</span>
            </dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">资料核验日期：</dt>
            <dd>{{ formatDateTime(design.last_verified_at) || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">数据版本标注：</dt>
            <dd>{{ design.data_version || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">来源：</dt>
            <dd>
              <a
                :href="isExternalUrl(design.source_url) ? (design.source_url as string) : '#'"
                target="_blank"
                rel="noopener noreferrer nofollow"
                class="text-grass-300 hover:underline"
              >
                Minecraft Wiki
              </a>
            </dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">授权协议：</dt>
            <dd>{{ design.license || '未标注' }}</dd>
          </div>
          <div v-if="design.license_note" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">授权说明：</dt>
            <dd>{{ design.license_note }}</dd>
          </div>
          <div v-if="isExternalUrl(design.world_download_url)" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">存档下载：</dt>
            <dd>
              <a
                :href="design.world_download_url as string"
                target="_blank"
                rel="noopener noreferrer nofollow"
                class="text-grass-300 hover:underline"
              >
                前往下载（{{ design.license || '授权信息见上' }}）
              </a>
            </dd>
          </div>
          <div v-else-if="design.world_download_note" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">存档说明：</dt>
            <dd>{{ design.world_download_note }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">内容审核：</dt>
            <dd>{{ REDSTONE_REVIEW_STATUS_LABELS[design.review_status] }}</dd>
          </div>
        </dl>
        <div class="mt-3 flex flex-wrap gap-3 text-xs">
          <a
            v-if="isExternalUrl(design.screenshot_url)"
            :href="design.screenshot_url as string"
            target="_blank"
            rel="noopener noreferrer nofollow"
            class="text-grass-300 hover:underline"
          >
            查看截图
          </a>
          <a
            v-if="isExternalUrl(design.video_url)"
            :href="design.video_url as string"
            target="_blank"
            rel="noopener noreferrer nofollow"
            class="text-grass-300 hover:underline"
          >
            观看视频
          </a>
        </div>
        <p class="mt-2 text-xs text-text-muted">
          本站不为未授权的存档提供下载链接，也不伪造下载直链；存档下载仅在明确标注授权协议时展示。
        </p>
      </section>
    </template>
  </PageContainer>
</template>
