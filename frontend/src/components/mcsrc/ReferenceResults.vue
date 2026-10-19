<script setup lang="ts">
// 查找引用结果视图：按「引用点所在类」分组，点击站点跳转到调用方成员。
//
// 数据来自服务端对整份 jar 的反向扫描（见 /mcsrc/classes/references）：站点已由服务端
// 排序去重，这里只做分组与呈现。分组顺序沿用服务端给出的顺序（先类名、后成员）。
import { computed } from 'vue'
import {
  formatReferenceSite,
  referenceTargetLabel,
  type McsrcReferences,
  type McsrcReferenceSite,
} from '@/types/mcsrc'

const props = defineProps<{
  /** 查找结果；null 表示尚未加载 */
  result: McsrcReferences | null
  /** 是否加载中 */
  loading?: boolean
  /** 错误文案 */
  error?: string
}>()

const emit = defineEmits<{
  /** 点击某个引用点，请求打开调用方成员 */
  open: [site: McsrcReferenceSite]
}>()

interface ReferenceGroup {
  className: string
  sites: McsrcReferenceSite[]
}

/** 按引用点所在类分组 */
const groups = computed<ReferenceGroup[]>(() => {
  const map = new Map<string, McsrcReferenceSite[]>()
  for (const site of props.result?.sites ?? []) {
    const list = map.get(site.from_class)
    if (list) list.push(site)
    else map.set(site.from_class, [site])
  }
  return [...map.entries()].map(([className, sites]) => ({ className, sites }))
})

/** 被查找的目标（类 / 成员）的可读标签 */
const targetLabel = computed(() => (props.result ? referenceTargetLabel(props.result) : ''))
</script>

<template>
  <div class="p-3" data-test="mcsrc-references">
    <div class="mb-2 flex flex-wrap items-center gap-2 text-xs">
      <span class="break-all font-mono text-text-primary">{{ targetLabel }}</span>
      <span v-if="result" class="text-text-muted">共 {{ result.total }} 处引用</span>
    </div>

    <div v-if="loading" role="status" aria-live="polite" class="text-sm text-text-muted">
      正在扫描 jar 查找引用…（首次较慢，之后为毫秒级）
    </div>
    <p v-else-if="error" role="alert" class="text-sm text-danger-mc">{{ error }}</p>
    <p v-else-if="result && !result.sites.length" class="text-sm text-text-muted">未找到引用。</p>
    <template v-else-if="result">
      <p
        v-if="result.truncated"
        class="mb-2 rounded border border-amber-mc/40 bg-amber-mc/10 px-2 py-1 text-[10px] text-amber-mc"
      >
        引用点过多，仅展示前 {{ result.sites.length }} 处。
      </p>
      <div v-for="group in groups" :key="group.className" class="mb-3">
        <div class="break-all font-mono text-xs font-semibold text-grass-300">{{ group.className }}</div>
        <ul class="mt-1 space-y-0.5 pl-4">
          <li
            v-for="site in group.sites"
            :key="`${site.from_kind}:${site.from_name}:${site.from_descriptor}`"
          >
            <button
              type="button"
              data-test="mcsrc-reference-site"
              class="w-full truncate text-left font-mono text-[11px] text-text-secondary transition hover:text-text-primary"
              :title="formatReferenceSite(site)"
              @click="emit('open', site)"
            >
              {{ formatReferenceSite(site) }}
            </button>
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>
