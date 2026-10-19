<script setup lang="ts">
// 全局搜索：下拉即时搜索结果（需求 5.1）
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { search } from '@/api/public'
import SearchResults from '@/components/search/SearchResults.vue'
import type { SearchResultItem } from '@/types/search'

const props = defineProps<{ autofocus?: boolean }>()

const emit = defineEmits<{ done: [] }>()

const router = useRouter()

const keyword = ref('')
const open = ref(false)
const loading = ref(false)
const results = ref<SearchResultItem[]>([])
const inputEl = ref<HTMLInputElement | null>(null)

let debounceTimer: ReturnType<typeof setTimeout> | undefined

watch(keyword, (val) => {
  debounceTimer && clearTimeout(debounceTimer)
  if (!val.trim()) {
    results.value = []
    open.value = false
    return
  }
  debounceTimer = setTimeout(runSearch, 300)
})

async function runSearch() {
  const q = keyword.value.trim()
  if (!q) return
  loading.value = true
  try {
    const res = await search({ q, page: 1, page_size: 6 })
    results.value = res.items
    open.value = true
  } catch (err) {
    results.value = []
    open.value = false
    // 全局搜索失败不打断用户，静默降级
    console.warn('search failed', err)
  } finally {
    loading.value = false
  }
}

function goResult(item: SearchResultItem) {
  open.value = false
  router.push(item.url)
  emit('done')
}

function goFullResults() {
  const q = keyword.value.trim()
  if (!q) return
  open.value = false
  router.push({ path: '/search', query: { q } })
  emit('done')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    open.value = false
    inputEl.value?.blur()
  }
  if (e.key === 'Enter') {
    goFullResults()
  }
}

function onClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.global-search-root')) {
    open.value = false
  }
}

onMounted(() => {
  if (props.autofocus) inputEl.value?.focus()
  document.addEventListener('click', onClickOutside)
})
onBeforeUnmount(() => document.removeEventListener('click', onClickOutside))
</script>

<template>
  <div class="global-search-root relative">
    <div class="relative">
      <svg
        class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path stroke-linecap="round" d="M21 21l-4.35-4.35M17 10.5a6.5 6.5 0 11-13 0 6.5 6.5 0 0113 0z" />
      </svg>
      <input
        ref="inputEl"
        v-model="keyword"
        type="search"
        role="combobox"
        :aria-expanded="open"
        aria-label="全站搜索"
        maxlength="80"
        placeholder="搜索工具、资源、百科、物品…"
        class="h-10 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-8 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
        @keydown="onKeydown"
      />
      <span v-if="loading" class="absolute right-3 top-1/2 -translate-y-1/2">
        <span class="block h-3.5 w-3.5 animate-spin rounded-full border-2 border-grass-500 border-t-transparent" />
      </span>
    </div>

    <div
      v-if="open && results.length"
      class="absolute left-0 right-0 top-12 z-50 overflow-hidden rounded-lg border border-border-light bg-ink-800 shadow-2xl shadow-black/50"
      role="listbox"
    >
      <ul class="max-h-80 overflow-y-auto">
        <SearchResults
          :items="results"
          :keyword="keyword"
          variant="compact"
          @select="goResult"
        />
      </ul>
      <button
        type="button"
        class="block w-full border-t border-border-dark bg-ink-900 px-3 py-2 text-center text-xs text-grass-400 hover:bg-ink-700"
        @click="goFullResults"
      >
        「{{ keyword.trim() }}」的全部搜索结果 →
      </button>
    </div>

    <div
      v-if="open && !loading && !results.length && keyword.trim()"
      class="absolute left-0 right-0 top-12 z-50 rounded-lg border border-border-light bg-ink-800 p-4 text-center shadow-2xl"
    >
      <p class="text-sm text-text-secondary">未找到「{{ keyword.trim() }}」相关内容</p>
      <button class="mt-2 text-xs text-grass-400 hover:underline" @click="goFullResults">
        前往完整搜索结果
      </button>
    </div>
  </div>
</template>