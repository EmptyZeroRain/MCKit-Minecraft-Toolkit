<script setup lang="ts">
// 可视化指令生成器（TODO §7）
//
// 表单化配置 → 校验 → 生成指令文本（一键复制）。
// 按版本区分「旧式 NBT」与「新版数据组件」语法；本页不执行任何命令、不连接服务器。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import FormField from '@/components/common/FormField.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import VersionNotice from '@/components/common/VersionNotice.vue'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { loadReleaseVersions } from '@/utils/mc-versions'
import { checkFeaturesSupport } from '@/utils/tool-versions'
import {
  EFFECTS,
  ENCHANTMENTS,
  GAMEMODES,
  GAMEMODE_LABELS,
  type CommandResult,
  type Gamemode,
  buildEffect,
  buildGamemode,
  buildGive,
  buildSetblock,
  buildSummon,
  buildTp,
  effectFormForVersion,
  syntaxForVersion,
  withSlash,
} from '@/utils/command-generator'
import { applyPageMeta } from '@/utils/seo'
import { changedOnly, queryBool, queryString } from '@/utils/share'

applyPageMeta({
  title: '可视化指令生成器',
  description:
    'MC 指令生成器：表单化配置 /give、/summon、/effect、/setblock、/tp、/gamemode，按 Java 版版本自动区分旧式 NBT 与新版数据组件语法，一键复制。',
  canonicalPath: '/command-generator',
})

useRecentToolsStore().record('command-generator', '指令生成器', '生成类')

const route = useRoute()
const router = useRouter()

type TabKey = 'give' | 'summon' | 'effect' | 'setblock' | 'tp' | 'gamemode'

const tabs: Array<{ key: TabKey; label: string; icon: string; hint: string }> = [
  { key: 'give', label: '/give', icon: '🎁', hint: '给予物品（支持附魔、自定义名称等）' },
  { key: 'summon', label: '/summon', icon: '🧟', hint: '生成实体（数据标签为 NBT）' },
  { key: 'effect', label: '/effect', icon: '✨', hint: '施加状态效果' },
  { key: 'setblock', label: '/setblock', icon: '🧱', hint: '放置方块（支持方块状态）' },
  { key: 'tp', label: '/tp', icon: '📍', hint: '传送实体或玩家' },
  { key: 'gamemode', label: '/gamemode', icon: '🎮', hint: '切换游戏模式' },
]

const TAB_KEYS = tabs.map((t) => t.key)

/** 解析 `tab` 查询参数（非法值回退 give）。 */
function normalizeTab(raw: string): TabKey {
  return (TAB_KEYS as string[]).includes(raw) ? (raw as TabKey) : 'give'
}

// URL 分享（TODO §14）：从查询参数恢复初始指令类型与版本，保证分享链接可复现。
const activeTab = ref<TabKey>(normalizeTab(queryString(route.query.tab)))

// ===== 版本 =====
const version = ref(queryString(route.query.v, '1.21'))
const releaseVersions = ref<string[]>([])

onMounted(async () => {
  releaseVersions.value = await loadReleaseVersions()
})

const syntax = computed(() => syntaxForVersion(version.value))
const effectForm = computed(() => effectFormForVersion(version.value))

// 版本特性提示（TODO §2）：只在与当前指令类型相关时提示，措辞统一。
const versionNotices = computed(() => {
  const keys =
    activeTab.value === 'give'
      ? ['data_components']
      : activeTab.value === 'effect'
        ? ['effect_give']
        : []
  return checkFeaturesSupport(keys, version.value)
})

// ===== 表单状态 =====
const give = reactive({
  target: '@s',
  item: 'diamond_sword',
  count: '1',
  enchantments: [] as Array<{ id: string; level: string }>,
  customName: '',
  unbreakable: false,
  damage: '',
})

const summon = reactive({
  entity: 'zombie',
  pos: { x: '~', y: '~', z: '~' },
  customName: '',
  noAI: false,
  noGravity: false,
  silent: false,
  persistenceRequired: false,
  glowing: false,
})

const effect = reactive({
  target: '@s',
  effect: 'speed',
  duration: '30',
  amplifier: '0',
  hideParticles: false,
})

const setblock = reactive({
  pos: { x: '0', y: '64', z: '0' },
  block: 'oak_planks',
  states: [] as Array<{ key: string; value: string }>,
  mode: 'replace' as 'replace' | 'destroy' | 'keep',
})

const tp = reactive({
  target: '@s',
  mode: 'coords' as 'coords' | 'entity',
  pos: { x: '0', y: '64', z: '0' },
  destination: '@e[type=zombie,limit=1]',
  yaw: '',
  pitch: '',
})

const gamemode = reactive({
  mode: 'creative' as Gamemode,
  target: '',
})

// ===== URL 分享（TODO §14）=====
// 把「指令类型 + 版本 + 该类型的表单」写入地址栏：分享链接打开即复现同一条指令。
// 布尔值用 1 表示真、空串表示假；附魔 / 方块状态等数组以 JSON 编码。

/** 布尔 → 查询值（真写 1，假写空串以便被 changedOnly 丢弃）。 */
function boolParam(v: boolean): string {
  return v ? '1' : ''
}

/** 当前指令类型的表单快照（扁平字符串，便于写入 URL）。 */
function tabState(): Record<string, string> {
  switch (activeTab.value) {
    case 'give':
      return {
        target: give.target,
        item: give.item,
        count: give.count,
        custom_name: give.customName,
        unbreakable: boolParam(give.unbreakable),
        damage: give.damage,
        ench: give.enchantments.length ? JSON.stringify(give.enchantments) : '',
      }
    case 'summon':
      return {
        entity: summon.entity,
        x: summon.pos.x,
        y: summon.pos.y,
        z: summon.pos.z,
        custom_name: summon.customName,
        no_ai: boolParam(summon.noAI),
        no_gravity: boolParam(summon.noGravity),
        silent: boolParam(summon.silent),
        persistent: boolParam(summon.persistenceRequired),
        glowing: boolParam(summon.glowing),
      }
    case 'effect':
      return {
        target: effect.target,
        effect: effect.effect,
        duration: effect.duration,
        amplifier: effect.amplifier,
        hide_particles: boolParam(effect.hideParticles),
      }
    case 'setblock':
      return {
        x: setblock.pos.x,
        y: setblock.pos.y,
        z: setblock.pos.z,
        block: setblock.block,
        states: setblock.states.length ? JSON.stringify(setblock.states) : '',
        mode: setblock.mode,
      }
    case 'tp':
      return {
        target: tp.target,
        mode: tp.mode,
        x: tp.pos.x,
        y: tp.pos.y,
        z: tp.pos.z,
        destination: tp.destination,
        yaw: tp.yaw,
        pitch: tp.pitch,
      }
    default:
      return { mode: gamemode.mode, target: gamemode.target }
  }
}

/** 解析 JSON 数组参数（非法值回退默认）。 */
function listParam<T>(raw: string, fallback: T[]): T[] {
  if (!raw) return fallback
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as T[]) : fallback
  } catch {
    return fallback
  }
}

/** 用 URL 查询参数覆盖当前指令类型的表单（缺失字段保持默认值）。 */
function applyTabState() {
  const q = route.query
  const s = (key: string) => queryString(q[key])
  const flag = (key: string) => queryBool(q[key])
  switch (activeTab.value) {
    case 'give':
      give.target = s('target') || give.target
      give.item = s('item') || give.item
      give.count = s('count') || give.count
      give.customName = s('custom_name')
      give.unbreakable = flag('unbreakable')
      give.damage = s('damage')
      give.enchantments = listParam(s('ench'), give.enchantments)
      break
    case 'summon':
      summon.entity = s('entity') || summon.entity
      summon.pos.x = s('x') || summon.pos.x
      summon.pos.y = s('y') || summon.pos.y
      summon.pos.z = s('z') || summon.pos.z
      summon.customName = s('custom_name')
      summon.noAI = flag('no_ai')
      summon.noGravity = flag('no_gravity')
      summon.silent = flag('silent')
      summon.persistenceRequired = flag('persistent')
      summon.glowing = flag('glowing')
      break
    case 'effect':
      effect.target = s('target') || effect.target
      effect.effect = s('effect') || effect.effect
      effect.duration = s('duration') || effect.duration
      effect.amplifier = s('amplifier') || effect.amplifier
      effect.hideParticles = flag('hide_particles')
      break
    case 'setblock':
      setblock.pos.x = s('x') || setblock.pos.x
      setblock.pos.y = s('y') || setblock.pos.y
      setblock.pos.z = s('z') || setblock.pos.z
      setblock.block = s('block') || setblock.block
      setblock.states = listParam(s('states'), setblock.states)
      if ((['replace', 'destroy', 'keep'] as string[]).includes(s('mode'))) {
        setblock.mode = s('mode') as 'replace' | 'destroy' | 'keep'
      }
      break
    case 'tp':
      tp.target = s('target') || tp.target
      if (s('mode') === 'entity' || s('mode') === 'coords') {
        tp.mode = s('mode') as 'coords' | 'entity'
      }
      tp.pos.x = s('x') || tp.pos.x
      tp.pos.y = s('y') || tp.pos.y
      tp.pos.z = s('z') || tp.pos.z
      tp.destination = s('destination') || tp.destination
      tp.yaw = s('yaw')
      tp.pitch = s('pitch')
      break
    default:
      if ((GAMEMODES as readonly string[]).includes(s('mode'))) {
        gamemode.mode = s('mode') as Gamemode
      }
      gamemode.target = s('target')
  }
}

/** 把当前状态写回地址栏（只写与默认值不同的字段，URL 保持简洁可读）。 */
function syncUrl() {
  router.replace({
    path: '/command-generator',
    query: {
      ...changedOnly({ tab: activeTab.value, v: version.value }, { tab: 'give', v: '1.21' }),
      ...changedOnly(tabState()),
    },
  })
}

applyTabState()
watch([activeTab, version, give, summon, effect, setblock, tp, gamemode], syncUrl, { deep: true })

// ===== 生成 =====
const result = computed<CommandResult>(() => {
  switch (activeTab.value) {
    case 'give':
      return buildGive(give, syntax.value.syntax)
    case 'summon':
      return buildSummon(summon)
    case 'effect':
      return buildEffect(effect, version.value)
    case 'setblock':
      return buildSetblock(setblock)
    case 'tp':
      return buildTp({
        target: tp.target,
        mode: tp.mode,
        pos: tp.pos,
        destination: tp.destination,
        rotation: { yaw: tp.yaw, pitch: tp.pitch },
      })
    case 'gamemode':
      return buildGamemode({ mode: gamemode.mode, target: gamemode.target })
    default:
      return { ok: false, issues: [] }
  }
})

const commandText = computed(() => (result.value.ok ? withSlash(result.value.command) : ''))
const notes = computed(() => (result.value.ok ? result.value.notes : []))
const issues = computed(() => (result.value.ok ? [] : result.value.issues))

function issueFor(field: string): string | undefined {
  return issues.value.find((i) => i.field === field)?.reason
}

const inputClass =
  'h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'
const selectClass =
  'h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none'

// ===== 附魔行 =====
function addEnchant() {
  give.enchantments.push({ id: ENCHANTMENTS[0].id, level: '1' })
}
function removeEnchant(i: number) {
  give.enchantments.splice(i, 1)
}

// ===== 方块状态行 =====
function addState() {
  setblock.states.push({ key: '', value: '' })
}
function removeState(i: number) {
  setblock.states.splice(i, 1)
}

// ===== 清空 / 示例 =====
function clearForm() {
  if (activeTab.value === 'give') {
    give.target = '@s'
    give.item = ''
    give.count = '1'
    give.enchantments = []
    give.customName = ''
    give.unbreakable = false
    give.damage = ''
  } else if (activeTab.value === 'summon') {
    summon.entity = ''
    summon.pos = { x: '~', y: '~', z: '~' }
    summon.customName = ''
    summon.noAI = false
    summon.noGravity = false
    summon.silent = false
    summon.persistenceRequired = false
    summon.glowing = false
  } else if (activeTab.value === 'effect') {
    effect.target = '@s'
    effect.effect = 'speed'
    effect.duration = '30'
    effect.amplifier = '0'
    effect.hideParticles = false
  } else if (activeTab.value === 'setblock') {
    setblock.pos = { x: '0', y: '64', z: '0' }
    setblock.block = ''
    setblock.states = []
    setblock.mode = 'replace'
  } else if (activeTab.value === 'tp') {
    tp.target = '@s'
    tp.mode = 'coords'
    tp.pos = { x: '0', y: '64', z: '0' }
    tp.destination = ''
    tp.yaw = ''
    tp.pitch = ''
  } else {
    gamemode.mode = 'creative'
    gamemode.target = ''
  }
}

function fillExample() {
  if (activeTab.value === 'give') {
    give.target = '@s'
    give.item = 'diamond_sword'
    give.count = '1'
    give.enchantments = [
      { id: 'sharpness', level: '5' },
      { id: 'mending', level: '1' },
    ]
    give.customName = 'Excalibur'
    give.unbreakable = true
    give.damage = ''
  } else if (activeTab.value === 'summon') {
    summon.entity = 'zombie'
    summon.pos = { x: '~', y: '~1', z: '~' }
    summon.customName = '守卫'
    summon.noAI = true
  } else if (activeTab.value === 'effect') {
    effect.target = '@a'
    effect.effect = 'speed'
    effect.duration = '60'
    effect.amplifier = '1'
    effect.hideParticles = true
  } else if (activeTab.value === 'setblock') {
    setblock.pos = { x: '10', y: '64', z: '10' }
    setblock.block = 'oak_planks'
    setblock.states = [{ key: 'waterlogged', value: 'true' }]
    setblock.mode = 'replace'
  } else if (activeTab.value === 'tp') {
    tp.target = '@p'
    tp.mode = 'coords'
    tp.pos = { x: '100', y: '64', z: '-200' }
    tp.yaw = '90'
    tp.pitch = '0'
  } else {
    gamemode.mode = 'creative'
    gamemode.target = 'Steve'
  }
}

function goTools() {
  router.push('/tools')
}
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: '指令生成器' }]" />

    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">可视化指令生成器</h1>
        <p class="text-text-secondary">
          表单化配置常见指令，自动按版本区分旧式 NBT 与新版数据组件语法。
          本页<strong class="text-text-primary">只生成文本</strong>，不执行命令、不连接你的服务器。
        </p>
      </div>
      <ShareButton />
    </div>

    <div class="grid gap-6 lg:grid-cols-5">
      <!-- 配置区 -->
      <section class="lg:col-span-3" aria-labelledby="cmd-form-title">
        <div class="card p-5">
          <div class="mb-5 flex flex-wrap items-end justify-between gap-3">
            <div>
              <h2 id="cmd-form-title" class="font-semibold text-text-primary">指令类型</h2>
              <p class="text-xs text-text-muted">{{ tabs.find((t) => t.key === activeTab)?.hint }}</p>
            </div>
            <div class="w-full max-w-[14rem]">
              <FormField label="目标 MC 版本" hint="如 1.21 / 1.20.4 / 1.16.5">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="version"
                    :list="`${id}-list`"
                    :class="inputClass"
                    placeholder="1.21"
                  />
                  <datalist :id="`${id}-list`">
                    <option v-for="v in releaseVersions" :key="v" :value="v" />
                  </datalist>
                </template>
              </FormField>
            </div>
          </div>

          <!-- 语法提示 -->
          <div
            class="mb-5 rounded-lg border px-4 py-2.5 text-xs"
            :class="
              syntax.assumed
                ? 'border-amber-500/40 bg-amber-500/10 text-amber-300'
                : 'border-grass-500/30 bg-grass-500/10 text-grass-300'
            "
          >
            物品语法：
            <strong>{{ syntax.syntax === 'components' ? '新版数据组件 item[组件]' : '旧式物品 NBT {…}' }}</strong>
            ·
            <span v-if="activeTab === 'effect'">
              /effect 形态：<strong>{{ effectForm.form === 'give' ? 'effect give …' : '旧式 effect …' }}</strong>
            </span>
            <span v-else-if="activeTab === 'summon'">实体数据：<strong>NBT 标签</strong></span>
            <span v-else>该指令无物品/实体数据语法差异</span>
            <span v-if="syntax.note" class="mt-1 block">注：{{ syntax.note }}</span>
          </div>

          <!-- 版本不支持 / 不确定提示（统一组件，TODO §2） -->
          <div v-if="versionNotices.length" class="mb-5 space-y-2">
            <VersionNotice
              v-for="n in versionNotices"
              :key="n.feature.key"
              :level="n.level"
              :message="n.message"
            />
          </div>

          <!-- Tabs -->
          <div class="mb-5 flex flex-wrap gap-2" role="tablist" aria-label="指令类型">
            <button
              v-for="t in tabs"
              :key="t.key"
              type="button"
              role="tab"
              :aria-selected="activeTab === t.key"
              class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition"
              :class="
                activeTab === t.key
                  ? 'border-grass-500 bg-grass-600 text-white'
                  : 'border-border-dark bg-ink-800 text-text-secondary hover:bg-ink-700 hover:text-text-primary'
              "
              @click="activeTab = t.key"
            >
              <span aria-hidden="true">{{ t.icon }}</span>
              <span class="font-mono">{{ t.label }}</span>
            </button>
          </div>

          <!-- /give -->
          <form v-if="activeTab === 'give'" class="space-y-4" novalidate @submit.prevent>
            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="目标" required :error="issueFor('target')" hint="选择器或玩家名，如 @s / @a / Steve">
                <template #default="{ id }"><input :id="id" v-model="give.target" :class="inputClass" /></template>
              </FormField>
              <FormField label="物品 ID" required :error="issueFor('item')" hint="如 diamond_sword（自动补 minecraft:）">
                <template #default="{ id }"><input :id="id" v-model="give.item" :class="inputClass" /></template>
              </FormField>
            </div>

            <FormField label="数量" required :error="issueFor('count')">
              <template #default="{ id }">
                <input :id="id" v-model="give.count" type="number" min="1" max="6400" :class="inputClass" />
              </template>
            </FormField>

            <div>
              <div class="mb-2 flex items-center justify-between">
                <span class="text-sm font-medium text-text-primary">附魔</span>
                <button type="button" class="text-xs text-grass-400 hover:underline" @click="addEnchant">
                  + 添加附魔
                </button>
              </div>
              <p v-if="!give.enchantments.length" class="text-xs text-text-muted">未添加附魔</p>
              <div v-for="(en, i) in give.enchantments" :key="i" class="mb-2 flex items-end gap-2">
                <div class="flex-1">
                  <select v-model="en.id" :class="selectClass" aria-label="附魔">
                    <option v-for="e in ENCHANTMENTS" :key="e.id" :value="e.id">
                      {{ e.name }}（{{ e.id }}）
                    </option>
                  </select>
                </div>
                <div class="w-24">
                  <input
                    v-model="en.level"
                    type="number"
                    min="1"
                    :class="inputClass"
                    aria-label="附魔等级"
                    placeholder="等级"
                  />
                </div>
                <button
                  type="button"
                  class="h-10 shrink-0 rounded-md border border-border-dark px-3 text-sm text-text-muted hover:text-danger-mc"
                  aria-label="删除该附魔"
                  @click="removeEnchant(i)"
                >
                  ✕
                </button>
              </div>
              <p v-if="issues.some((x) => x.field.startsWith('enchant.'))" class="text-xs text-danger-mc" role="alert">
                {{ issues.find((x) => x.field.startsWith('enchant.'))?.reason }}
              </p>
            </div>

            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="自定义名称" :error="issueFor('customName')" hint="可留空">
                <template #default="{ id }"><input :id="id" v-model="give.customName" :class="inputClass" /></template>
              </FormField>
              <FormField label="损伤值" :error="issueFor('damage')" hint="可留空；受损工具的 Damage 值">
                <template #default="{ id }">
                  <input :id="id" v-model="give.damage" type="number" min="0" :class="inputClass" />
                </template>
              </FormField>
            </div>

            <label class="flex items-center gap-2 text-sm text-text-secondary">
              <input v-model="give.unbreakable" type="checkbox" class="h-4 w-4" />
              不可破坏（Unbreakable）
            </label>
          </form>

          <!-- /summon -->
          <form v-else-if="activeTab === 'summon'" class="space-y-4" novalidate @submit.prevent>
            <FormField label="实体 ID" required :error="issueFor('entity')" hint="如 zombie / minecraft:zombie">
              <template #default="{ id }"><input :id="id" v-model="summon.entity" :class="inputClass" /></template>
            </FormField>

            <div>
              <span class="mb-1.5 block text-sm font-medium text-text-primary">生成坐标</span>
              <div class="grid gap-3 sm:grid-cols-3">
                <input v-model="summon.pos.x" :class="inputClass" aria-label="X" placeholder="X" />
                <input v-model="summon.pos.y" :class="inputClass" aria-label="Y" placeholder="Y" />
                <input v-model="summon.pos.z" :class="inputClass" aria-label="Z" placeholder="Z" />
              </div>
              <p v-if="issueFor('pos')" class="mt-1 text-xs text-danger-mc" role="alert">{{ issueFor('pos') }}</p>
              <p class="mt-1 text-xs text-text-muted">支持 ~ 相对坐标与 ^ 局部坐标；默认 ~ ~ ~。</p>
            </div>

            <FormField label="自定义名称" :error="issueFor('customName')">
              <template #default="{ id }"><input :id="id" v-model="summon.customName" :class="inputClass" /></template>
            </FormField>

            <div class="grid gap-2 sm:grid-cols-2">
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="summon.noAI" type="checkbox" class="h-4 w-4" /> 无 AI（NoAI）
              </label>
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="summon.noGravity" type="checkbox" class="h-4 w-4" /> 不受重力（NoGravity）
              </label>
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="summon.silent" type="checkbox" class="h-4 w-4" /> 静音（Silent）
              </label>
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="summon.persistenceRequired" type="checkbox" class="h-4 w-4" /> 不消失（PersistenceRequired）
              </label>
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="summon.glowing" type="checkbox" class="h-4 w-4" /> 发光（Glowing）
              </label>
            </div>
          </form>

          <!-- /effect -->
          <form v-else-if="activeTab === 'effect'" class="space-y-4" novalidate @submit.prevent>
            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="目标" required :error="issueFor('target')">
                <template #default="{ id }"><input :id="id" v-model="effect.target" :class="inputClass" /></template>
              </FormField>
              <FormField label="状态效果" required :error="issueFor('effect')" hint="可手填其他 ID">
                <template #default="{ id }">
                  <input :id="id" v-model="effect.effect" :list="`${id}-list`" :class="inputClass" />
                  <datalist :id="`${id}-list`">
                    <option v-for="e in EFFECTS" :key="e.id" :value="e.id">{{ e.name }}</option>
                  </datalist>
                </template>
              </FormField>
            </div>

            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="持续时间（秒）" required :error="issueFor('duration')">
                <template #default="{ id }">
                  <input :id="id" v-model="effect.duration" type="number" min="1" :class="inputClass" />
                </template>
              </FormField>
              <FormField label="等级（amplifier）" required :error="issueFor('amplifier')" hint="0 = 一级">
                <template #default="{ id }">
                  <input :id="id" v-model="effect.amplifier" type="number" min="0" max="255" :class="inputClass" />
                </template>
              </FormField>
            </div>

            <label class="flex items-center gap-2 text-sm text-text-secondary">
              <input v-model="effect.hideParticles" type="checkbox" class="h-4 w-4" />
              隐藏粒子（hideParticles = true）
            </label>
          </form>

          <!-- /setblock -->
          <form v-else-if="activeTab === 'setblock'" class="space-y-4" novalidate @submit.prevent>
            <div>
              <span class="mb-1.5 block text-sm font-medium text-text-primary">坐标</span>
              <div class="grid gap-3 sm:grid-cols-3">
                <input v-model="setblock.pos.x" :class="inputClass" aria-label="X" placeholder="X" />
                <input v-model="setblock.pos.y" :class="inputClass" aria-label="Y" placeholder="Y" />
                <input v-model="setblock.pos.z" :class="inputClass" aria-label="Z" placeholder="Z" />
              </div>
              <p v-if="issueFor('pos')" class="mt-1 text-xs text-danger-mc" role="alert">{{ issueFor('pos') }}</p>
            </div>

            <div class="grid gap-4 sm:grid-cols-2">
              <FormField label="方块 ID" required :error="issueFor('block')" hint="如 oak_planks">
                <template #default="{ id }"><input :id="id" v-model="setblock.block" :class="inputClass" /></template>
              </FormField>
              <FormField label="处理模式">
                <template #default="{ id }">
                  <select :id="id" v-model="setblock.mode" :class="selectClass">
                    <option value="replace">replace（替换，默认）</option>
                    <option value="destroy">destroy（破坏并掉落）</option>
                    <option value="keep">keep（仅当目标为空气时放置）</option>
                  </select>
                </template>
              </FormField>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between">
                <span class="text-sm font-medium text-text-primary">方块状态</span>
                <button type="button" class="text-xs text-grass-400 hover:underline" @click="addState">
                  + 添加状态
                </button>
              </div>
              <p v-if="!setblock.states.length" class="text-xs text-text-muted">未添加方块状态</p>
              <div v-for="(st, i) in setblock.states" :key="i" class="mb-2 flex items-center gap-2">
                <input v-model="st.key" :class="inputClass" placeholder="key（如 waterlogged）" aria-label="状态名" />
                <span class="text-text-muted">=</span>
                <input v-model="st.value" :class="inputClass" placeholder="value（如 true）" aria-label="状态值" />
                <button
                  type="button"
                  class="h-10 shrink-0 rounded-md border border-border-dark px-3 text-sm text-text-muted hover:text-danger-mc"
                  aria-label="删除该状态"
                  @click="removeState(i)"
                >
                  ✕
                </button>
              </div>
              <p v-if="issues.some((x) => x.field.startsWith('state.'))" class="text-xs text-danger-mc" role="alert">
                {{ issues.find((x) => x.field.startsWith('state.'))?.reason }}
              </p>
            </div>
          </form>

          <!-- /tp -->
          <form v-else-if="activeTab === 'tp'" class="space-y-4" novalidate @submit.prevent>
            <div class="flex flex-wrap gap-4">
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="tp.mode" type="radio" value="coords" class="h-4 w-4" /> 传送到坐标
              </label>
              <label class="flex items-center gap-2 text-sm text-text-secondary">
                <input v-model="tp.mode" type="radio" value="entity" class="h-4 w-4" /> 传送到实体
              </label>
            </div>

            <FormField label="目标" required :error="issueFor('target')" hint="被传送者">
              <template #default="{ id }"><input :id="id" v-model="tp.target" :class="inputClass" /></template>
            </FormField>

            <template v-if="tp.mode === 'coords'">
              <div>
                <span class="mb-1.5 block text-sm font-medium text-text-primary">目标坐标</span>
                <div class="grid gap-3 sm:grid-cols-3">
                  <input v-model="tp.pos.x" :class="inputClass" aria-label="X" placeholder="X" />
                  <input v-model="tp.pos.y" :class="inputClass" aria-label="Y" placeholder="Y" />
                  <input v-model="tp.pos.z" :class="inputClass" aria-label="Z" placeholder="Z" />
                </div>
                <p v-if="issueFor('pos')" class="mt-1 text-xs text-danger-mc" role="alert">{{ issueFor('pos') }}</p>
              </div>
              <div class="grid gap-4 sm:grid-cols-2">
                <FormField label="水平角 yaw" :error="issueFor('yaw')" hint="可留空">
                  <template #default="{ id }"><input :id="id" v-model="tp.yaw" :class="inputClass" placeholder="90" /></template>
                </FormField>
                <FormField label="俯仰角 pitch" :error="issueFor('pitch')" hint="可留空">
                  <template #default="{ id }"><input :id="id" v-model="tp.pitch" :class="inputClass" placeholder="0" /></template>
                </FormField>
              </div>
            </template>
            <FormField v-else label="目标实体" required :error="issueFor('destination')" hint="如 @e[type=zombie,limit=1]">
              <template #default="{ id }"><input :id="id" v-model="tp.destination" :class="inputClass" /></template>
            </FormField>
          </form>

          <!-- /gamemode -->
          <form v-else class="space-y-4" novalidate @submit.prevent>
            <FormField label="游戏模式" required :error="issueFor('mode')">
              <template #default="{ id }">
                <select :id="id" v-model="gamemode.mode" :class="selectClass">
                  <option v-for="m in GAMEMODES" :key="m" :value="m">{{ GAMEMODE_LABELS[m] }}（{{ m }}）</option>
                </select>
              </template>
            </FormField>
            <FormField label="目标" :error="issueFor('target')" hint="留空表示命令执行者自身">
              <template #default="{ id }"><input :id="id" v-model="gamemode.target" :class="inputClass" /></template>
            </FormField>
          </form>

          <div class="mt-5 flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
              @click="fillExample"
            >
              填入示例
            </button>
            <button
              type="button"
              class="text-sm text-text-muted transition hover:text-text-primary"
              @click="clearForm"
            >
              清空表单
            </button>
          </div>
        </div>
      </section>

      <!-- 输出区 -->
      <section class="lg:col-span-2" aria-labelledby="cmd-output-title">
        <div class="card p-5">
          <div class="mb-4 flex items-center justify-between">
            <h2 id="cmd-output-title" class="font-semibold text-text-primary">生成的指令</h2>
            <CopyButton tool="command-generator" v-if="commandText" :text="commandText" label="复制指令" />
          </div>

          <div v-if="result.ok" class="space-y-4">
            <div class="code-block break-all p-4 font-mono text-sm text-grass-300">
              {{ commandText }}
            </div>

            <ul v-if="notes.length" class="space-y-1 text-xs leading-relaxed text-text-secondary">
              <li v-for="(n, i) in notes" :key="i">· {{ n }}</li>
            </ul>
          </div>

          <div
            v-else
            class="rounded-lg border border-danger-mc/40 bg-danger-mc/10 p-4 text-sm text-danger-mc"
            role="alert"
          >
            <p class="mb-2 font-medium">请先修正以下问题：</p>
            <ul class="list-disc space-y-1 pl-5 text-xs">
              <li v-for="(x, i) in issues" :key="i">{{ x.reason }}</li>
            </ul>
          </div>

          <div class="mt-4 rounded-lg border border-border-dark bg-ink-900 p-4 text-xs">
            <h3 class="mb-2 font-medium text-text-primary">使用说明</h3>
            <ul class="list-disc space-y-1 pl-5 leading-relaxed text-text-secondary">
              <li>生成结果仅为文本，需在游戏内或服务器控制台自行执行（通常需 OP / 命令方块）。</li>
              <li>物品：1.20.5 起使用数据组件 <code>item[组件]</code>，更早版本使用 NBT <code>{…}</code>。</li>
              <li>实体（/summon）数据标签始终为 NBT；<code>~</code> 相对、<code>^</code> 局部坐标。</li>
              <li>/effect 自 1.19 起使用 <code>give</code> 子命令，更早版本无该子命令。</li>
              <li>版本无法识别（快照 / 预览版）时按最新语法生成，请自行核对。</li>
            </ul>
          </div>

          <button
            type="button"
            class="mt-4 text-sm text-grass-400 hover:underline"
            @click="goTools"
          >
            浏览更多工具 →
          </button>
        </div>
      </section>
    </div>
  </PageContainer>
</template>
