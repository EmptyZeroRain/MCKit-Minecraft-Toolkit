<script setup lang="ts">
// 村民职业详情（TODO §9B）：职业本体、工作站、等级规则、分组交易与关联物品。
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
import { getVillager } from '@/api/public'
import {
  VILLAGER_KIND_LABELS,
  VILLAGER_TRADE_GROUP_LABELS,
  formatCostCount,
  type VillagerProfessionDetail,
  type VillagerTrade,
} from '@/types/villager'
import type { ItemRef } from '@/types/item'
import { resolveRelatedItem } from '@/utils/item-link'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'

const route = useRoute()
const router = useRouter()

const slug = computed(() => (route.params.slug as string) || '')
const detail = ref<VillagerProfessionDetail | null>(null)
const loading = ref(true)
const error = ref('')

const EDITION_LABELS: Record<string, string> = {
  java: 'Java 版',
  bedrock: '基岩版',
  both: 'Java 版 / 基岩版',
}

/** 工作站物品摘要：未收录时 url 为 null，只展示纯文本 ID（不生成死链）。 */
const workstationLink = computed(() =>
  resolveRelatedItem(
    detail.value?.resolved_items as Record<string, ItemRef> | undefined,
    detail.value?.workstation_item_id ?? '',
  ),
)

/** 成本物品摘要：优先用交易自身的 cost_item_refs，回退到详情级 resolved_items。 */
function costLink(trade: VillagerTrade, itemId: string) {
  return resolveRelatedItem(
    trade.cost_item_refs ?? (detail.value?.resolved_items as Record<string, ItemRef> | undefined),
    itemId,
  )
}

/** 结果物品摘要。 */
function resultLink(itemId: string) {
  return resolveRelatedItem(detail.value?.resolved_items as Record<string, ItemRef> | undefined, itemId)
}

/** 交易成本展示文本：`煤炭 ×15 ＋ 绿宝石 ×1`。 */
function costText(trade: VillagerTrade): string {
  return trade.cost_items.map((c) => `${c.name_zh} ×${formatCostCount(c)}`).join(' ＋ ')
}

/** 交易的可复制文本：`村民想要：… → 玩家获得：…`。 */
function tradeText(trade: VillagerTrade): string {
  return `村民想要：${costText(trade)} → 玩家获得：${trade.result_item_name_zh} ×${trade.result_count}`
}

/** 分组标题：等级组展示等级名，其余展示分组名。 */
function groupTitle(group: VillagerProfessionDetail['groups'][number]): string {
  if (group.trade_group === 'level') return `${group.level_name_zh}（${group.required_experience} 经验）`
  return VILLAGER_TRADE_GROUP_LABELS[group.trade_group]
}

async function loadDetail() {
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    const data = await getVillager(slug.value)
    detail.value = data
    if (data) {
      applyPageMeta({
        title: `${data.name_zh} · 村民交易`,
        description: data.summary || `${data.name_zh} 的工作站、交易等级与交易表。`,
        canonicalPath: `/villagers/${data.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
    reportToolFailure('villagers')
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
    <Breadcrumbs :items="[{ label: '村民交易', to: '/villagers' }, { label: detail?.name_zh ?? slug }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="!detail"
      title="未收录该村民职业"
      description="本站村民交易库仅包含已核验收录的职业，未收录内容不会伪造查询结果。"
      action-label="返回村民交易"
      @action="router.push('/villagers')"
    />

    <template v-else>
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-muted">
            {{ VILLAGER_KIND_LABELS[detail.kind] }}
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
            <FavoriteButton
              kind="villager"
              :slug="detail.slug"
              :label="detail.name_zh"
              :url="`/villagers/${detail.slug}`"
            />
            <ShareButton />
          </div>
        </div>
        <p v-if="detail.summary" class="mt-3 text-text-secondary">{{ detail.summary }}</p>
        <p v-if="detail.description" class="mt-2 text-sm text-text-secondary">{{ detail.description }}</p>
        <p v-if="detail.trade_note" class="mt-2 text-sm text-text-muted">{{ detail.trade_note }}</p>
      </header>

      <!-- 工作站 -->
      <section v-if="detail.workstation_item_id" class="card mb-6 p-4" aria-labelledby="villager-station-title">
        <h2 id="villager-station-title" class="mb-2 text-lg font-semibold text-text-primary">工作站</h2>
        <p class="flex flex-wrap items-center gap-x-2 text-sm">
          <RouterLink
            v-if="workstationLink.url"
            :to="workstationLink.url as string"
            class="font-medium text-grass-300 hover:underline"
          >
            {{ detail.workstation_name_zh }}
          </RouterLink>
          <span v-else class="font-medium text-text-primary">{{ detail.workstation_name_zh }}</span>
          <code class="break-all font-mono text-xs text-text-muted">{{ detail.workstation_item_id }}</code>
        </p>
        <p class="mt-2 text-xs text-text-muted">
          失业村民认领工作站后会转为该职业；工作站被摧毁后重新变为失业。
        </p>
      </section>

      <!-- 等级规则 -->
      <section v-if="detail.levels.length" class="card mb-6 p-4" aria-labelledby="villager-levels-title">
        <h2 id="villager-levels-title" class="mb-3 text-lg font-semibold text-text-primary">等级规则</h2>
        <ul class="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
          <li v-for="level in detail.levels" :key="level.id" class="rounded-md border border-border-dark p-3">
            <div class="flex items-baseline justify-between gap-2">
              <span class="font-medium text-text-primary">{{ level.name_zh }}</span>
              <span class="text-xs text-text-muted">{{ level.required_experience }} 经验</span>
            </div>
            <p class="mt-1 text-xs text-text-secondary">{{ level.badge_note }}</p>
          </li>
        </ul>
      </section>

      <!-- 交易表（按等级 / 分组） -->
      <section class="card mb-6 p-4" aria-labelledby="villager-trades-title">
        <h2 id="villager-trades-title" class="mb-3 text-lg font-semibold text-text-primary">交易表</h2>
        <p v-if="!detail.groups.length" class="text-sm text-text-muted">该职业没有交易表。</p>
        <div v-else class="space-y-6">
          <div v-for="group in detail.groups" :key="`${group.trade_group}-${group.level}`">
            <h3 class="mb-2 text-sm font-semibold text-text-secondary">{{ groupTitle(group) }}</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-left text-sm">
                <thead class="border-b border-border-dark text-xs text-text-muted">
                  <tr>
                    <th scope="col" class="py-2 pr-4 font-medium">村民想要</th>
                    <th scope="col" class="py-2 pr-4 font-medium">玩家获得</th>
                    <th scope="col" class="py-2 pr-4 font-medium">库存</th>
                    <th scope="col" class="hidden py-2 pr-4 font-medium sm:table-cell">经验</th>
                    <th scope="col" class="py-2 font-medium">备注</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border-dark">
                  <tr v-for="trade in group.trades" :key="trade.id">
                    <td class="py-2 pr-4">
                      <span class="flex flex-wrap items-center gap-x-1">
                        <template v-for="(cost, i) in trade.cost_items" :key="`${trade.id}-cost-${i}`">
                          <span v-if="i > 0" class="text-text-muted">＋</span>
                          <RouterLink
                            v-if="costLink(trade, cost.item_id).url"
                            :to="costLink(trade, cost.item_id).url as string"
                            class="text-grass-300 hover:underline"
                          >
                            {{ cost.name_zh }}
                          </RouterLink>
                          <span v-else class="text-text-primary">{{ cost.name_zh }}</span>
                          <span class="text-text-secondary">×{{ formatCostCount(cost) }}</span>
                        </template>
                      </span>
                      <code class="mt-0.5 block break-all font-mono text-xs text-text-muted">
                        {{ trade.cost_items.map((c) => c.item_id).join(' + ') }}
                      </code>
                    </td>
                    <td class="py-2 pr-4">
                      <RouterLink
                        v-if="resultLink(trade.result_item_id).url"
                        :to="resultLink(trade.result_item_id).url as string"
                        class="font-medium text-grass-300 hover:underline"
                      >
                        {{ trade.result_item_name_zh }}
                      </RouterLink>
                      <span v-else class="text-text-primary">{{ trade.result_item_name_zh }}</span>
                      <span class="text-text-secondary"> ×{{ trade.result_count }}</span>
                      <code
                        v-if="!resultLink(trade.result_item_id).url"
                        class="mt-0.5 block break-all font-mono text-xs text-text-muted"
                      >
                        {{ trade.result_item_id }}
                      </code>
                    </td>
                    <td class="py-2 pr-4 whitespace-nowrap text-text-secondary">{{ trade.max_uses }}</td>
                    <td class="hidden py-2 pr-4 whitespace-nowrap text-text-secondary sm:table-cell">
                      {{ trade.villager_experience }}
                    </td>
                    <td class="py-2">
                      <span v-if="trade.variant_note" class="block text-xs text-text-secondary">
                        {{ trade.variant_note }}
                      </span>
                      <span v-if="trade.note" class="block text-xs text-text-muted">{{ trade.note }}</span>
                      <VersionBadge
                        v-for="v in trade.supported_versions"
                        :key="`${trade.id}-${v}`"
                        :version="v"
                        class="ml-1"
                      />
                      <CopyButton
                        class="mt-1"
                        tool="villagers"
                        :text="tradeText(trade)"
                        label="复制交易"
                        success-message="已复制交易"
                      />
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </section>

      <!-- 来源与核验 -->
      <section class="card p-4 text-sm" aria-labelledby="villager-source-title">
        <h2 id="villager-source-title" class="mb-2 font-semibold text-text-primary">来源与核验</h2>
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
                Minecraft Wiki（Trading）
              </a>
            </dd>
          </div>
        </dl>
        <p class="mt-2 text-xs text-text-muted">
          价格会随交易次数上涨、随声望（治愈僵尸村民）下降；附魔内容随机，无法指定。
        </p>
      </section>
    </template>
  </PageContainer>
</template>
