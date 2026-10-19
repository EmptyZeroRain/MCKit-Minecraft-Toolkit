<script setup lang="ts">
// 备注（Javadoc）编辑器：类备注或成员备注。
//
// 存储走浏览器 localStorage（见 `@/mcsrc/notes`）：匿名可用、不落库、不上传，
// 备注只存在于使用者本机。这里只负责「读 → 编辑 → 保存 / 删除」的界面交互。
import { computed, ref, watch } from 'vue'
import { readClassNote, readMemberNote, writeNote } from '@/mcsrc/notes'
import type { McsrcNoteTarget } from '@/types/mcsrc'

const props = defineProps<{
  target: McsrcNoteTarget
  /** 标题，如「类备注」「字段备注」 */
  label: string
}>()

const emit = defineEmits<{
  /** 保存后触发，携带是否仍有内容 */
  saved: [hasNote: boolean]
}>()

function read(): string {
  return props.target.member ? readMemberNote(props.target) : readClassNote(props.target)
}

const text = ref(read())
/** 最近一次保存的文本，用于判断是否有未保存改动 */
const savedText = ref(text.value)

// 切换到其它类 / 成员时重新读取（同一组件可能被复用）。
watch(
  () => [
    props.target.version,
    props.target.side,
    props.target.className,
    props.target.member?.kind,
    props.target.member?.name,
    props.target.member?.descriptor,
  ],
  () => {
    text.value = read()
    savedText.value = text.value
  },
)

const dirty = computed(() => text.value !== savedText.value)

function save() {
  writeNote(props.target, text.value)
  savedText.value = text.value
  emit('saved', !!text.value.trim())
}

function remove() {
  text.value = ''
  save()
}
</script>

<template>
  <div class="rounded-md border border-border-dark bg-ink-950/60 p-2" data-test="mcsrc-notes-editor">
    <div class="mb-1 flex items-center justify-between gap-2">
      <span class="text-[11px] font-semibold text-text-secondary">{{ label }}</span>
      <span class="text-[10px] text-text-muted">仅保存在本机浏览器</span>
    </div>
    <textarea
      v-model="text"
      rows="3"
      :aria-label="label"
      placeholder="写点备注…（支持纯文本，不会上传）"
      class="w-full resize-y rounded border border-border-dark bg-ink-900 px-2 py-1 font-mono text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
    />
    <div class="mt-1 flex items-center gap-2">
      <button
        type="button"
        data-test="mcsrc-notes-save"
        class="rounded border border-grass-500/40 bg-grass-500/10 px-2 py-0.5 text-[10px] text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-40"
        :disabled="!dirty"
        @click="save"
      >
        保存
      </button>
      <button
        type="button"
        data-test="mcsrc-notes-delete"
        class="rounded border border-border-dark px-2 py-0.5 text-[10px] text-text-secondary transition hover:bg-ink-700 disabled:opacity-40"
        :disabled="!savedText"
        @click="remove"
      >
        删除
      </button>
      <span v-if="dirty" class="text-[10px] text-amber-mc">未保存</span>
    </div>
  </div>
</template>
