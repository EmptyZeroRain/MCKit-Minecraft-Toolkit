<script setup lang="ts">
// 生物详情（TODO §9C）：生物本体、基础属性、生成条件、掉落清单、驯服与繁殖、关联物品。
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
import { getMob } from '@/api/public'
import {
  MOB_CATEGORY_LABELS,
  MOB_DIMENSION_LABELS,
  dimensionClass,
  formatChance,
  formatCount,
  formatHealth,
  type MobDetail,
  type MobDrop,
} from '@/types/mob'
import type { ItemRef } from '@/types/item'
import { resolveRelatedItem } from '@/utils/item-link'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'

const route = useRoute()
const router = useRouter()

const slug = computed(() => (route.params.slug as string) || '')
const detail = ref<MobDetail | null>(null)
const loading = ref(true)
const error = ref('')

const EDITION_LABELS: Record<string, string> = {
  java: 'Java 版',
  bedrock: '基岩版',
  both: 'Java 版 / 基岩版',
}

/** 掉落物摘要：优先用掉落自身的 item_ref，回退到详情级 resolved_items。 */
function dropLink(drop: MobDrop) {
  if (drop.item_ref) {
    return { ref: drop.item_ref, label: drop.item_ref.name_zh, url: drop.item_ref.url || null }
  }
  return resolveRelatedItem(
    detail.value?.resolved_items as Record<string, ItemRef> | undefined,
    drop.item_id,
  )
}

/** 掉落的可复制文本：`生牛肉 ×1–3（必定）`。 */
function dropText(drop: MobDrop): string {
  return `${drop.item_name_zh} ×${formatCount(drop.count_min, drop.count_max)}（${formatChance(drop.chance)}）`
}

async function loadDetail() {
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    const data = await getMob(slug.value)
    detail.value = data
    if (data) {
      applyPageMeta({
        title: `${data.name_zh} · 生物资料`,
        description: data.summary || `${data.name_zh} 的生命值、掉落物与生成条件。`,
        canonicalPath: `/mobs/${data.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    reportToolFailure('mobs')
  } finally {
    loading.value = false
  }
}

onMounted(loadDetail)
watch(slug, loadDetail)

function externalUrl(url: string | null): string {
  return url && /^https?:\/\//i.test(url) ? url : '#'
}
</script>

<template>
  <PageContainer maxWidth="max-w-4xl">
    <Breadcrumbs :items="[{ label: '生物资料', to: '/mobs' }, { label: detail?.name_zh ?? slug }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="!detail"
      title="未收录该生物"
      description="本站生物资料库仅包含已核验收录的条目，未收录内容不会伪造查询结果。"
      action-label="返回生物资料"
      @action="router.push('/mobs')"
    />

    <template v-else>
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ MOB_CATEGORY_LABELS[detail.category] }}
          </span>
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ EDITION_LABELS[detail.game_edition] ?? detail.game_edition }}
          </span>
          <VersionBadge v-for="v in detail.supported_versions" :key="v" :version="v" />
        </div>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="text-2xl font-bold text-text-primary md:text-3xl">{{ detail.name_zh }}</h1>
            <p v-if="detail.name_en" class="mt-1 text-sm text-text-muted">{{ detail.name_en }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <FavoriteButton kind="mob" :slug="detail.slug" :label="detail.name_zh" :url="`/mobs/${detail.slug}`" />
            <ShareButton />
          </div>
        </div>
        <p v-if="detail.summary" class="mt-3 text-text-secondary">{{ detail.summary }}</p>
        <p v-if="detail.behavior_note" class="mt-2 text-sm text-text-secondary">{{ detail.behavior_note }}</p>
      </header>

      <!-- 基础属性 -->
      <section class="card mb-6 p-4" aria-labelledby="mob-base-title">
        <h2 id="mob-base-title" class="mb-3 text-lg font-semibold text-text-primary">基础属性</h2>
        <dl class="grid gap-3 text-sm sm:grid-cols-2">
          <div>
            <dt class="text-text-muted">生命值</dt>
            <dd class="text-text-primary">{{ formatHealth(detail.health) }}</dd>
          </div>
          <div>
            <dt class="text-text-muted">出现维度</dt>
            <dd class="mt-1 flex flex-wrap gap-1">
              <span
                v-for="dim in detail.dimensions"
                :key="dim"
                class="rounded border px-2 py-0.5 text-xs"
                :class="dimensionClass(dim)"
              >
                {{ MOB_DIMENSION_LABELS[dim] }}
              </span>
              <span v-if="!detail.dimensions.length" class="text-text-muted">—</span>
            </dd>
          </div>
          <div v-if="detail.attack_note">
            <dt class="text-text-muted">攻击方式</dt>
            <dd class="text-text-secondary">{{ detail.attack_note }}</dd>
          </div>
          <div v-if="detail.biomes_note">
            <dt class="text-text-muted">生物群系</dt>
            <dd class="text-text-secondary">{{ detail.biomes_note }}</dd>
          </div>
        </dl>
      </section>

      <!-- 生成条件 -->
      <section v-if="detail.spawn_note" class="card mb-6 p-4" aria-labelledby="mob-spawn-title">
        <h2 id="mob-spawn-title" class="mb-2 text-lg font-semibold text-text-primary">生成条件</h2>
        <p class="text-sm text-text-secondary">{{ detail.spawn_note }}</p>
      </section>

      <!-- 掉落 -->
      <section class="card mb-6 p-4" aria-labelledby="mob-drops-title">
        <h2 id="mob-drops-title" class="mb-3 text-lg font-semibold text-text-primary">掉落</h2>
        <p v-if="!detail.drops.length" class="text-sm text-text-muted">该生物没有已收录的掉落物。</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark text-xs text-text-muted">
              <tr>
                <th scope="col" class="py-2 pr-4 font-medium">掉落物</th>
                <th scope="col" class="py-2 pr-4 font-medium">数量</th>
                <th scope="col" class="py-2 pr-4 font-medium">概率</th>
                <th scope="col" class="py-2 font-medium">说明</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="drop in detail.drops" :key="drop.id">
                <td class="py-2 pr-4">
                  <RouterLink
                    v-if="dropLink(drop).url"
                    :to="dropLink(drop).url as string"
                    class="font-medium text-grass-300 hover:underline"
                  >
                    {{ drop.item_name_zh }}
                  </RouterLink>
                  <span v-else class="text-text-primary">{{ drop.item_name_zh }}</span>
                  <code v-if="!dropLink(drop).url" class="mt-0.5 block break-all font-mono text-xs text-text-muted">
                    {{ drop.item_id }}
                  </code>
                </td>
                <td class="py-2 pr-4 whitespace-nowrap text-text-secondary">
                  {{ formatCount(drop.count_min, drop.count_max) }}
                </td>
                <td class="py-2 pr-4 whitespace-nowrap text-text-secondary">{{ formatChance(drop.chance) }}</td>
                <td class="py-2">
                  <span v-if="drop.conditions" class="block text-xs text-text-secondary">{{ drop.conditions }}</span>
                  <span v-if="drop.note" class="block text-xs text-text-muted">{{ drop.note }}</span>
                  <CopyButton
                    class="mt-1"
                    tool="mobs"
                    :text="dropText(drop)"
                    label="复制掉落"
                    success-message="已复制掉落信息"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 驯服与繁殖 -->
      <section
        v-if="detail.taming_note || detail.breeding_note"
        class="card mb-6 p-4"
        aria-labelledby="mob-tame-title"
      >
        <h2 id="mob-tame-title" class="mb-3 text-lg font-semibold text-text-primary">驯服与繁殖</h2>
        <dl class="space-y-2 text-sm">
          <div v-if="detail.taming_note">
            <dt class="text-text-muted">驯服</dt>
            <dd class="text-text-secondary">{{ detail.taming_note }}</dd>
          </div>
          <div v-if="detail.breeding_note">
            <dt class="text-text-muted">繁殖</dt>
            <dd class="text-text-secondary">{{ detail.breeding_note }}</dd>
          </div>
        </dl>
      </section>

      <!-- 来源与核验 -->
      <section class="card p-4 text-sm" aria-labelledby="mob-source-title">
        <h2 id="mob-source-title" class="mb-2 font-semibold text-text-primary">来源与核验</h2>
        <dl class="space-y-1 text-text-secondary">
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">资料核验日期：</dt>
            <dd>{{ formatDateTime(detail.last_verified_at) || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">数据版本标注：</dt>
            <dd>{{ detail.data_version || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">来源：</dt>
            <dd>
              <a
                :href="externalUrl(detail.source_url)"
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
          攻击伤害会随难度与版本变化，故以文字说明；生命值为 Java 版数值（1 点 = 半颗心）。
        </p>
      </section>
    </template>
  </PageContainer>
</template>
