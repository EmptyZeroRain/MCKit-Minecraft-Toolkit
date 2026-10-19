<script setup lang="ts">
// 药水详情（TODO §9A）：效果与时长（普通/延长/增强）、酿造步骤、通用规则与关联资料。
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
import { getPotion } from '@/api/public'
import {
  BREW_STEP_LABELS,
  POTION_CATEGORY_LABELS,
  POTION_EDITION_LABELS,
  POTION_VARIANT_LABELS,
  formatDuration,
  romanNumeral,
  type PotionDetail,
  type PotionEffect,
  type PotionVariant,
} from '@/types/potion'
import type { ItemRef } from '@/types/item'
import { resolveRelatedItem } from '@/utils/item-link'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'

const route = useRoute()
const router = useRouter()

const slug = computed(() => (route.params.slug as string) || '')
const detail = ref<PotionDetail | null>(null)
const loading = ref(true)
const error = ref('')

/** 变体展示顺序：普通 → 延长 → 增强。 */
const VARIANT_ORDER: PotionVariant[] = ['normal', 'extended', 'enhanced']

/** 按变体分组的效果；空数组表示该变体不适用（如治疗药水没有延长版）。 */
const effectGroups = computed(() =>
  VARIANT_ORDER.map((variant) => ({
    variant,
    label: POTION_VARIANT_LABELS[variant],
    effects: (detail.value?.effects ?? []).filter((e) => e.variant === variant),
  })),
)

const hasAnyEffect = computed(() => (detail.value?.effects?.length ?? 0) > 0)

/** 原料物品摘要：未收录时 url 为 null，只展示纯文本 ID（不生成死链）。 */
function ingredientLink(id: string) {
  return resolveRelatedItem(
    detail.value?.ingredient_items as Record<string, ItemRef> | undefined,
    id,
  )
}

function effectLine(effect: PotionEffect): string {
  const level = romanNumeral(effect.amplifier)
  return effect.duration_seconds > 0
    ? `${level} 级 · ${formatDuration(effect.duration_seconds)}`
    : `${level} 级 · ${formatDuration(0)}`
}

function stepKindLabel(kind: string): string {
  return BREW_STEP_LABELS[kind as keyof typeof BREW_STEP_LABELS] ?? kind
}

/** 酿造步骤的可复制文本：`基底药水 ＋ 原料 → 成品`。 */
function stepText(step: PotionDetail['produced_by'][number]): string {
  const input = step.input_potion?.name_zh ?? '水瓶'
  const result = step.result_potion?.name_zh ?? '（同输入药水）'
  return `${input} ＋ ${step.ingredient_name_zh} → ${result}（${stepKindLabel(step.step_kind)}）`
}

async function loadDetail() {
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    const data = await getPotion(slug.value)
    detail.value = data
    if (data) {
      applyPageMeta({
        title: `${data.name_zh} · 药水与酿造`,
        description: data.summary || `${data.name_zh} 的效果、持续时间与酿造步骤。`,
        canonicalPath: `/potions/${data.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    reportToolFailure('potions')
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
    <Breadcrumbs :items="[{ label: '药水与酿造', to: '/potions' }, { label: detail?.name_zh ?? slug }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="!detail"
      title="未收录该药水"
      description="本站药水库仅包含已核验收录的药水，未收录内容不会伪造查询结果。"
      action-label="返回药水查询"
      @action="router.push('/potions')"
    />

    <template v-else>
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ POTION_CATEGORY_LABELS[detail.category] }}
          </span>
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ POTION_EDITION_LABELS[detail.game_edition] }}
          </span>
          <VersionBadge v-for="v in detail.supported_versions" :key="v" :version="v" />
          <span v-if="!detail.is_brewable" class="text-xs text-amber-mc">无法通过酿造获得</span>
        </div>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="text-2xl font-bold text-text-primary md:text-3xl">{{ detail.name_zh }}</h1>
            <p v-if="detail.name_en" class="mt-1 text-sm text-text-muted">{{ detail.name_en }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <FavoriteButton kind="potion" :slug="detail.slug" :label="detail.name_zh" :url="`/potions/${detail.slug}`" />
            <ShareButton />
          </div>
        </div>
        <p v-if="detail.summary" class="mt-3 text-text-secondary">{{ detail.summary }}</p>
        <p v-if="detail.effect_note" class="mt-2 text-sm text-text-muted">{{ detail.effect_note }}</p>
      </header>

      <!-- 效果与持续时间 -->
      <section class="card mb-6 p-4" aria-labelledby="potion-effects-title">
        <h2 id="potion-effects-title" class="mb-3 text-lg font-semibold text-text-primary">
          效果与持续时间
        </h2>
        <p v-if="!hasAnyEffect" class="text-sm text-text-muted">该药水本身没有生效效果。</p>
        <div v-else class="space-y-4">
          <div v-for="group in effectGroups" :key="group.variant">
            <h3 class="mb-2 text-sm font-semibold text-text-secondary">{{ group.label }}</h3>
            <p v-if="!group.effects.length" class="text-xs text-text-muted">该变体不适用。</p>
            <div v-else class="overflow-x-auto">
              <table class="w-full text-left text-sm">
                <thead class="border-b border-border-dark text-xs text-text-muted">
                  <tr>
                    <th scope="col" class="py-2 pr-4 font-medium">效果</th>
                    <th scope="col" class="py-2 pr-4 font-medium">等级 / 时长</th>
                    <th scope="col" class="py-2 font-medium">说明</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border-dark">
                  <tr v-for="(effect, i) in group.effects" :key="`${group.variant}-${i}`">
                    <td class="py-2 pr-4">
                      <span class="text-text-primary">{{ effect.name_zh }}</span>
                      <span v-if="effect.name_en" class="ml-1 text-xs text-text-muted">{{ effect.name_en }}</span>
                    </td>
                    <td class="py-2 pr-4 whitespace-nowrap text-text-secondary">{{ effectLine(effect) }}</td>
                    <td class="py-2 text-text-secondary">{{ effect.description }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
        <p class="mt-3 text-xs text-text-muted">
          延长：{{ detail.is_extendable ? '可用红石粉延长持续时间' : '不适用' }}；增强：{{
            detail.is_enhanceable ? '可用荧石粉提升等级' : '不适用'
          }}。
        </p>
      </section>

      <!-- 如何酿出该药水 -->
      <section class="card mb-6 p-4" aria-labelledby="potion-produced-title">
        <h2 id="potion-produced-title" class="mb-3 text-lg font-semibold text-text-primary">
          酿造步骤
        </h2>
        <p v-if="!detail.produced_by.length" class="text-sm text-text-muted">
          该药水无法通过酿造获得。
        </p>
        <ul v-else class="space-y-3">
          <li
            v-for="step in detail.produced_by"
            :key="step.id"
            class="rounded-md border border-border-dark bg-ink-900 p-3"
          >
            <div class="mb-1 flex flex-wrap items-center gap-2 text-xs text-text-muted">
              <span class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5">
                {{ stepKindLabel(step.step_kind) }}
              </span>
              <VersionBadge v-for="v in step.supported_versions" :key="v" :version="v" />
              <CopyButton
                tool="potions"
                :text="stepText(step)"
                label="复制步骤"
                success-message="已复制酿造步骤"
              />
            </div>
            <p class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-text-primary">
              <RouterLink
                v-if="step.input_potion"
                :to="`/potions/${step.input_potion.slug}`"
                class="text-grass-300 hover:underline"
              >
                {{ step.input_potion.name_zh }}
              </RouterLink>
              <span v-else class="text-text-secondary">水瓶</span>
              <span class="text-text-muted">＋</span>
              <RouterLink
                v-if="ingredientLink(step.ingredient_item_id).url"
                :to="ingredientLink(step.ingredient_item_id).url as string"
                class="text-grass-300 hover:underline"
              >
                {{ step.ingredient_name_zh }}
              </RouterLink>
              <span v-else>
                {{ step.ingredient_name_zh }}
                <code class="ml-1 break-all font-mono text-xs text-text-muted">{{ step.ingredient_item_id }}</code>
              </span>
              <span class="text-text-muted">→</span>
              <RouterLink
                v-if="step.result_potion"
                :to="`/potions/${step.result_potion.slug}`"
                class="font-medium text-grass-300 hover:underline"
              >
                {{ step.result_potion.name_zh }}
              </RouterLink>
            </p>
            <p v-if="step.note" class="mt-1 text-xs text-text-secondary">{{ step.note }}</p>
          </li>
        </ul>
      </section>

      <!-- 后续步骤：该药水还能变成什么 -->
      <section v-if="detail.follow_ups.length" class="card mb-6 p-4" aria-labelledby="potion-followups-title">
        <h2 id="potion-followups-title" class="mb-3 text-lg font-semibold text-text-primary">后续变化</h2>
        <ul class="space-y-2 text-sm">
          <li v-for="step in detail.follow_ups" :key="step.id" class="flex flex-wrap items-center gap-x-2">
            <span class="text-text-secondary">加入</span>
            <span class="text-text-primary">{{ step.ingredient_name_zh }}</span>
            <span class="text-text-muted">→</span>
            <RouterLink
              v-if="step.result_potion"
              :to="`/potions/${step.result_potion.slug}`"
              class="text-grass-300 hover:underline"
            >
              {{ step.result_potion.name_zh }}
            </RouterLink>
            <span class="text-xs text-text-muted">{{ stepKindLabel(step.step_kind) }}</span>
          </li>
        </ul>
      </section>

      <!-- 通用规则 -->
      <section v-if="detail.rules.length" class="card mb-6 p-4" aria-labelledby="potion-rules-title">
        <h2 id="potion-rules-title" class="mb-3 text-lg font-semibold text-text-primary">通用酿造规则</h2>
        <ul class="space-y-2 text-sm">
          <li v-for="rule in detail.rules" :key="rule.id" class="flex flex-wrap items-baseline gap-x-2">
            <span class="font-medium text-text-primary">{{ rule.ingredient_name_zh }}</span>
            <RouterLink
              v-if="ingredientLink(rule.ingredient_item_id).url"
              :to="ingredientLink(rule.ingredient_item_id).url as string"
              class="text-xs text-grass-300 hover:underline"
            >
              物品详情
            </RouterLink>
            <span class="text-text-secondary">— {{ rule.note }}</span>
          </li>
        </ul>
      </section>

      <!-- 来源与核验 -->
      <section class="card p-4 text-sm" aria-labelledby="potion-source-title">
        <h2 id="potion-source-title" class="mb-2 font-semibold text-text-primary">来源与核验</h2>
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
                Minecraft Wiki（{{ detail.name_en || detail.slug }}）
              </a>
            </dd>
          </div>
        </dl>
        <p class="mt-2 text-xs text-text-muted">
          数值以 Minecraft Wiki 公开资料为准；Java 版与基岩版存在差异时已在说明中标注。
        </p>
      </section>
    </template>
  </PageContainer>
</template>
