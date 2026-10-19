<script setup lang="ts">
// 整合包详情（TODO §16）：包内 Mod 清单、目标版本与作者、来源与许可。
//   清单项**已收录才给出 Mod 详情链接**，未收录回退纯文本名（不生成死链）。
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
import { getModpack } from '@/api/public'
import {
  MOD_LOADER_LABELS,
  MOD_REVIEW_STATUS_LABELS,
  isExternalUrl,
  loaderClass,
  modpackModLabel,
  modpackModUrl,
  type ModpackDetail,
} from '@/types/mods'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { reportToolFailure } from '@/utils/tool-stats'

const route = useRoute()
const router = useRouter()

const slug = computed(() => (route.params.slug as string) || '')
const pack = ref<ModpackDetail | null>(null)
const loading = ref(true)
const error = ref('')

/** 已收录的清单项数量，用于说明链接覆盖情况。 */
const linkedCount = computed(() => pack.value?.mods.filter((m) => m.mod_ref).length ?? 0)

async function loadDetail() {
  loading.value = true
  error.value = ''
  pack.value = null
  try {
    const data = await getModpack(slug.value)
    pack.value = data
    if (data) {
      applyPageMeta({
        title: `${data.name_zh} · 整合包`,
        description: data.summary || `${data.name_zh} 的包内 Mod 清单与版本信息。`,
        canonicalPath: `/modpacks/${data.slug}`,
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
      :items="[{ label: 'Mod 与整合包', to: '/mods' }, { label: pack?.name_zh ?? slug }]"
    />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadDetail" />
    <EmptyState
      v-else-if="!pack"
      title="未收录该整合包"
      description="本站只展示已通过内容审核（published）的整合包，未通过审核或未收录的内容不会对外可见。"
      action-label="返回 Mod 列表"
      @action="goBack"
    />

    <template v-else>
      <header class="mb-6">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <span
            v-if="pack.loader"
            class="rounded border px-2 py-0.5 text-xs"
            :class="loaderClass(pack.loader)"
          >
            {{ MOD_LOADER_LABELS[pack.loader] }}
          </span>
          <VersionBadge v-if="pack.mc_version" :version="pack.mc_version" />
        </div>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h1 class="text-2xl font-bold text-text-primary md:text-3xl">{{ pack.name_zh }}</h1>
            <p v-if="pack.name_en" class="mt-1 text-sm text-text-muted">{{ pack.name_en }}</p>
            <p v-if="pack.author" class="mt-1 text-xs text-text-muted">作者：{{ pack.author }}</p>
            <p v-if="pack.aliases.length" class="mt-1 text-xs text-text-muted">
              别名：{{ pack.aliases.join('、') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <FavoriteButton
              kind="modpack"
              :slug="pack.slug"
              :label="pack.name_zh"
              :url="`/modpacks/${pack.slug}`"
            />
            <ShareButton />
          </div>
        </div>
        <p v-if="pack.summary" class="mt-3 text-text-secondary">{{ pack.summary }}</p>
      </header>

      <section v-if="pack.description" class="card mb-6 p-4" aria-labelledby="pack-desc-title">
        <h2 id="pack-desc-title" class="mb-3 text-lg font-semibold text-text-primary">说明</h2>
        <MarkdownView :content="pack.description" />
      </section>

      <!-- 包内 Mod 清单 -->
      <section class="card mb-6 p-4" aria-labelledby="pack-mods-title">
        <h2 id="pack-mods-title" class="mb-3 text-lg font-semibold text-text-primary">包内 Mod 清单</h2>
        <p v-if="!pack.mods.length" class="text-sm text-text-muted">该整合包没有已收录的 Mod 清单。</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark text-xs text-text-muted">
              <tr>
                <th scope="col" class="py-2 pr-4 font-medium">Mod</th>
                <th scope="col" class="py-2 pr-4 font-medium">版本</th>
                <th scope="col" class="py-2 font-medium">备注</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in pack.mods" :key="item.id">
                <td class="py-2 pr-4">
                  <RouterLink
                    v-if="modpackModUrl(item)"
                    :to="modpackModUrl(item) as string"
                    class="font-medium text-grass-300 hover:underline"
                  >
                    {{ modpackModLabel(item) }}
                  </RouterLink>
                  <span v-else class="text-text-primary">{{ modpackModLabel(item) }}</span>
                  <code
                    v-if="!modpackModUrl(item) && item.mod_name"
                    class="mt-0.5 block break-all font-mono text-xs text-text-muted"
                  >
                    {{ item.mod_name }}
                  </code>
                </td>
                <td class="py-2 pr-4 whitespace-nowrap text-text-secondary">{{ item.version || '—' }}</td>
                <td class="py-2 text-text-secondary">{{ item.note || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="mt-2 text-xs text-text-muted">
          共 {{ pack.mods.length }} 项，其中 {{ linkedCount }} 项已收录并可直接跳转；未收录项以纯文本展示，不生成 Mod 死链。
        </p>
      </section>

      <!-- 来源与许可 -->
      <section class="card p-4 text-sm" aria-labelledby="pack-source-title">
        <h2 id="pack-source-title" class="mb-2 font-semibold text-text-primary">来源与许可</h2>
        <dl class="space-y-1 text-text-secondary">
          <div v-if="isExternalUrl(pack.homepage_url)" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">主页：</dt>
            <dd>
              <a
                :href="pack.homepage_url as string"
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
                v-if="isExternalUrl(pack.source_url)"
                :href="pack.source_url as string"
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
            <dd>{{ pack.license || '未标注' }}</dd>
          </div>
          <div v-if="pack.license_note" class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">授权说明：</dt>
            <dd>{{ pack.license_note }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">资料核验日期：</dt>
            <dd>{{ formatDateTime(pack.last_verified_at) || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">数据版本标注：</dt>
            <dd>{{ pack.data_version || '未标注' }}</dd>
          </div>
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-text-muted">内容审核：</dt>
            <dd>{{ MOD_REVIEW_STATUS_LABELS[pack.review_status] }}</dd>
          </div>
        </dl>
        <div class="mt-3 flex flex-wrap gap-3">
          <CopyButton
            tool="mods"
            :text="pack.slug"
            label="复制 slug"
            success-message="已复制整合包 slug"
          />
        </div>
      </section>
    </template>
  </PageContainer>
</template>
