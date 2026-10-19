<script setup lang="ts">
// 百科详情（需求 3.1 /wiki/:slug）
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import MarkdownView from '@/components/common/MarkdownView.vue'
import FavoriteButton from '@/components/common/FavoriteButton.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import { getWiki } from '@/api/public'
import type { WikiEntry } from '@/types/wiki'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'
import { searchEntry } from '@/utils/related-search'

const route = useRoute()
const slug = computed(() => (route.params.slug as string) || '')

const entry = ref<WikiEntry | null>(null)
const loading = ref(true)
const error = ref('')

/**
 * 关联模块检索入口（TODO §3）。
 * 百科与物品/工具/资源之间没有可靠的元数据关联，故提供「以条目标题检索」入口，
 * 由用户自行确认，不伪造直链。
 */
const relatedLinks = computed(() => {
  const name = entry.value?.title
  if (!name) return null
  return {
    item: searchEntry(name, 'item'),
    tool: searchEntry(name, 'tool'),
    resource: searchEntry(name, 'resource'),
  }
})

async function loadEntry() {
  loading.value = true
  error.value = ''
  entry.value = null
  try {
    entry.value = await getWiki(slug.value)
    if (entry.value) {
      applyPageMeta({
        title: entry.value.title,
        description: entry.value.summary || `${entry.value.title} - 百科`,
        canonicalPath: `/wiki/${entry.value.slug}`,
      })
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(loadEntry)
watch(slug, loadEntry)
</script>

<template>
  <PageContainer maxWidth="max-w-3xl">
    <Breadcrumbs :items="[{ label: '百科', to: '/tools' }, { label: entry?.title ?? slug }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadEntry" />

    <article v-else-if="entry" class="card p-6 md:p-8">
      <header class="mb-6 border-b border-border-dark pb-5">
        <h1 class="mb-3 text-2xl font-bold text-text-primary md:text-3xl">{{ entry.title }}</h1>
        <p v-if="entry.summary" class="mb-3 text-text-secondary">{{ entry.summary }}</p>
        <div class="mb-3">
          <FavoriteButton
            kind="wiki"
            :slug="entry.slug"
            :label="entry.title"
            :url="`/wiki/${entry.slug}`"
          />
        </div>
        <div class="flex flex-wrap items-center gap-2 text-xs text-text-muted">
          <span v-if="entry.content_type" class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5">
            {{ entry.content_type }}
          </span>
          <span
            v-for="v in entry.game_versions ?? []"
            :key="v"
            class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5"
          >
            {{ v }}
          </span>
          <span v-for="tag in entry.tags ?? []" :key="tag" class="rounded bg-ink-700 px-1.5 py-0.5">
            #{{ tag }}
          </span>
        </div>
      </header>

      <MarkdownView :content="entry.content" />

      <!-- 关联检索入口（TODO §3：无可靠元数据时不伪造关联） -->
      <section v-if="relatedLinks" class="mt-8 rounded-card border border-border-dark bg-ink-800 p-4">
        <h2 class="mb-1 text-sm font-semibold text-text-primary">在其它模块中查找</h2>
        <p class="mb-3 text-xs text-text-muted">
          以下为按条目标题发起的检索入口；本站不对百科与物品/工具/资源做无数据依据的关联。
        </p>
        <div class="flex flex-wrap gap-2">
          <RouterLink
            :to="relatedLinks.item"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
          >
            检索相关物品
          </RouterLink>
          <RouterLink
            :to="relatedLinks.tool"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
          >
            检索相关工具
          </RouterLink>
          <RouterLink
            :to="relatedLinks.resource"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:text-grass-300"
          >
            检索相关资源
          </RouterLink>
        </div>
      </section>

      <footer class="mt-8 border-t border-border-dark pt-4 text-xs text-text-muted">
        <div class="mb-2">最后更新：{{ formatDateTime(entry.updated_at) }}</div>
        <div v-if="entry.source_references?.length">
          <div class="mb-1 font-medium">来源参考：</div>
          <ul class="list-disc space-y-1 pl-5">
            <li v-for="(ref, i) in entry.source_references" :key="i">
              <a
                :href="ref.url"
                target="_blank"
                rel="noopener noreferrer nofollow"
                class="text-grass-400 hover:underline"
              >
                {{ ref.title }}
              </a>
            </li>
          </ul>
        </div>
      </footer>
    </article>
  </PageContainer>
</template>