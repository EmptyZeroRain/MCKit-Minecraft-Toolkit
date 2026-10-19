<script setup lang="ts">
// 附魔与装备计算器（TODO §8）
//
// 组合合法性校验 + 铁砧合并花费（Java 公式）+ 结果附魔推演 + 附魔资料表。
// 规则来源：Minecraft Wiki《Anvil mechanics》；未核实/未覆盖部分显式标注。
import { computed, onMounted, ref, watch } from 'vue'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import FormField from '@/components/common/FormField.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import { useRecentToolsStore } from '@/stores/recent-tools'
import { loadReleaseVersions } from '@/utils/mc-versions'
import VersionNotice from '@/components/common/VersionNotice.vue'
import { UNKNOWN_VERSION_MESSAGE, isDecidableVersion } from '@/utils/tool-versions'
import {
  CONFLICT_ALLOWED_SINCE,
  ENCHANT_DATA_BY_ID,
  EQUIPMENT,
  type EnchantLevel,
  type EquipmentId,
  computeAnvilCost,
  equipmentEnchants,
  priorWorkPenalty,
  resultEnchantments,
  validateCombination,
} from '@/utils/enchant-calculator'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '附魔与装备计算器',
  description:
    'MC 附魔计算器：校验附魔组合是否合法（互斥附魔 / 适用装备 / 等级上限），按官方公式估算铁砧合并的经验花费与先作惩罚，并推演结果附魔。',
  canonicalPath: '/enchant-calculator',
})

useRecentToolsStore().record('enchant-calculator', '附魔计算器', '计算类')

const version = ref('1.21')
const releaseVersions = ref<string[]>([])
onMounted(async () => {
  releaseVersions.value = await loadReleaseVersions()
})

const equipment = ref<EquipmentId>('sword')
const availableEnchants = computed(() => equipmentEnchants(equipment.value))

const targetEnchants = ref<EnchantLevel[]>([])
const targetUseCount = ref(0)
const sacrificeKind = ref<'item' | 'book'>('book')
const sacrificeUseCount = ref(0)
const sacrificeEnchants = ref<EnchantLevel[]>([])
const rename = ref(false)
const targetDamaged = ref(false)

// 切换装备时，剔除不再适用的附魔，避免留下非法组合
watch(equipment, () => {
  const ids = new Set(availableEnchants.value.map((e) => e.id))
  targetEnchants.value = targetEnchants.value.filter((e) => ids.has(e.id))
  sacrificeEnchants.value = sacrificeEnchants.value.filter((e) => ids.has(e.id))
})

const issues = computed(() => validateCombination(equipment.value, targetEnchants.value, version.value))
const errors = computed(() => issues.value.filter((i) => i.level === 'error'))
const warnings = computed(() => issues.value.filter((i) => i.level === 'warning'))

const cost = computed(() =>
  computeAnvilCost(
    equipment.value,
    { kind: 'item', useCount: targetUseCount.value, enchantments: targetEnchants.value },
    { kind: sacrificeKind.value, useCount: sacrificeUseCount.value, enchantments: sacrificeEnchants.value },
    { rename: rename.value, targetDamaged: targetDamaged.value, version: version.value },
  ),
)

const combined = computed(() =>
  resultEnchantments(equipment.value, targetEnchants.value, sacrificeEnchants.value, version.value),
)

const tooExpensive = computed(() => cost.value.total >= 40)

const penaltyRows = computed(() => Array.from({ length: 9 }, (_, n) => ({ n, penalty: priorWorkPenalty(n) })))

const inputClass =
  'h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'
const monoClass =
  'h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none'

function addEnchant(list: EnchantLevel[]) {
  const first = availableEnchants.value[0]
  if (!first) return
  list.push({ id: first.id, level: 1 })
}
function maxLevelOf(id: string): number {
  return ENCHANT_DATA_BY_ID[id]?.maxLevel ?? 1
}

function fillExample() {
  equipment.value = 'sword'
  targetEnchants.value = [
    { id: 'sharpness', level: 3 },
    { id: 'knockback', level: 2 },
    { id: 'looting', level: 3 },
  ]
  targetUseCount.value = 0
  sacrificeKind.value = 'item'
  sacrificeUseCount.value = 0
  sacrificeEnchants.value = [
    { id: 'sharpness', level: 3 },
    { id: 'looting', level: 3 },
  ]
  rename.value = false
  targetDamaged.value = false
}

function clearAll() {
  targetEnchants.value = []
  sacrificeEnchants.value = []
  targetUseCount.value = 0
  sacrificeUseCount.value = 0
  rename.value = false
  targetDamaged.value = false
}

const sourceNote =
  '规则依据 Minecraft Wiki《Anvil mechanics》（Java Edition）。未覆盖：耐久修复量、附魔台花费、Bedrock 版差异、1.8 之前机制与模组附魔。'

// ===== 方案对比（TODO §8：对比不同装备或附魔组合）=====
interface Snapshot {
  id: number
  equipmentName: string
  total: number
  tooExpensive: boolean
  enchants: string
}

const snapshots = ref<Snapshot[]>([])
let snapSeq = 0

const currentSummary = computed(() => {
  const eq = EQUIPMENT.find((e) => e.id === equipment.value)?.name ?? equipment.value
  const list = targetEnchants.value.map((e) => `${ENCHANT_DATA_BY_ID[e.id]?.name ?? e.id} ${e.level}`).join('、')
  return { equipmentName: eq, enchants: list || '无附魔' }
})

function addSnapshot() {
  snapshots.value.push({
    id: ++snapSeq,
    equipmentName: currentSummary.value.equipmentName,
    enchants: currentSummary.value.enchants,
    total: cost.value.total,
    tooExpensive: cost.value.total >= 40,
  })
  if (snapshots.value.length > 8) snapshots.value.shift()
}

</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: '附魔计算器' }]" />

    <div class="mb-6">
      <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">附魔与装备计算器</h1>
      <p class="text-text-secondary">
        校验附魔组合是否合法，并按官方公式估算<strong class="text-text-primary">铁砧合并的经验花费</strong>与先作惩罚。
        涉及版本差异时按所选版本裁决；无法确认的边界会明确标注。
      </p>
    </div>

    <div class="grid gap-6 lg:grid-cols-5">
      <!-- 左：输入 -->
      <div class="space-y-6 lg:col-span-3">
        <section class="card p-5" aria-labelledby="combo-title">
          <div class="mb-4 flex flex-wrap items-end justify-between gap-3">
            <h2 id="combo-title" class="font-semibold text-text-primary">装备与附魔组合</h2>
            <div class="w-full max-w-[13rem]">
              <FormField label="游戏版本" hint="用于互斥附魔的版本语义">
                <template #default="{ id }">
                  <input :id="id" v-model="version" :list="`${id}-list`" :class="inputClass" placeholder="1.21" />
                  <datalist :id="`${id}-list`">
                    <option v-for="v in releaseVersions" :key="v" :value="v" />
                  </datalist>
                </template>
              </FormField>
            </div>
          </div>

          <!-- 版本不确定提示（统一组件，TODO §2）：快照 / 预览版 / 未填时不把理论值说成确定结果 -->
          <VersionNotice
            v-if="!isDecidableVersion(version)"
            class="mb-4"
            level="unknown"
            :message="UNKNOWN_VERSION_MESSAGE"
          />

          <FormField label="装备类型">
            <template #default="{ id }">
              <select :id="id" v-model="equipment" :class="inputClass">
                <option v-for="eq in EQUIPMENT" :key="eq.id" :value="eq.id">
                  {{ eq.group }} · {{ eq.name }}
                </option>
              </select>
            </template>
          </FormField>

          <div class="mt-5">
            <div class="mb-2 flex items-center justify-between">
              <span class="text-sm font-medium text-text-primary">目标装备附魔</span>
              <button type="button" class="text-xs text-grass-400 hover:underline" @click="addEnchant(targetEnchants)">
                + 添加附魔
              </button>
            </div>
            <p v-if="!targetEnchants.length" class="text-xs text-text-muted">尚未添加附魔</p>
            <div v-for="(en, i) in targetEnchants" :key="i" class="mb-2 flex items-center gap-2">
              <select v-model="en.id" :class="inputClass" aria-label="附魔">
                <option v-for="e in availableEnchants" :key="e.id" :value="e.id">{{ e.name }}（max {{ e.maxLevel }}）</option>
              </select>
              <input
                v-model.number="en.level"
                type="number"
                min="1"
                :max="maxLevelOf(en.id)"
                :class="monoClass"
                class="w-24"
                aria-label="等级"
              />
              <button
                type="button"
                class="h-10 shrink-0 rounded-md border border-border-dark px-3 text-sm text-text-muted hover:text-danger-mc"
                aria-label="删除附魔"
                @click="targetEnchants.splice(i, 1)"
              >
                ✕
              </button>
            </div>
          </div>

          <!-- 校验结果 -->
          <div v-if="errors.length || warnings.length" class="mt-4 space-y-2">
            <div
              v-if="errors.length"
              class="rounded-lg border border-danger-mc/40 bg-danger-mc/10 p-3 text-xs text-danger-mc"
              role="alert"
            >
              <p class="mb-1 font-medium">组合不合法：</p>
              <ul class="list-disc space-y-1 pl-5">
                <li v-for="(x, i) in errors" :key="i">{{ x.reason }}</li>
              </ul>
            </div>
            <div
              v-if="warnings.length"
              class="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs text-amber-300"
            >
              <p class="mb-1 font-medium">需要注意：</p>
              <ul class="list-disc space-y-1 pl-5">
                <li v-for="(x, i) in warnings" :key="i">{{ x.reason }}</li>
              </ul>
            </div>
          </div>
          <p
            v-else-if="targetEnchants.length"
            class="mt-4 rounded-lg border border-grass-500/30 bg-grass-500/10 p-3 text-xs text-grass-300"
          >
            组合合法：附魔均适用于当前装备，等级在上限内，未发现互斥冲突。
          </p>
        </section>

        <section class="card p-5" aria-labelledby="anvil-title">
          <h2 id="anvil-title" class="mb-4 font-semibold text-text-primary">铁砧合并</h2>

          <div class="grid gap-4 sm:grid-cols-3">
            <FormField label="目标已使用铁砧次数" hint="决定先作惩罚">
              <template #default="{ id }">
                <input :id="id" v-model.number="targetUseCount" type="number" min="0" max="30" :class="monoClass" />
              </template>
            </FormField>
            <FormField label="牺牲方类型">
              <template #default="{ id }">
                <select :id="id" v-model="sacrificeKind" :class="inputClass">
                  <option value="book">附魔书</option>
                  <option value="item">同种物品</option>
                </select>
              </template>
            </FormField>
            <FormField label="牺牲方已使用次数">
              <template #default="{ id }">
                <input :id="id" v-model.number="sacrificeUseCount" type="number" min="0" max="30" :class="monoClass" />
              </template>
            </FormField>
          </div>

          <div class="mt-4">
            <div class="mb-2 flex items-center justify-between">
              <span class="text-sm font-medium text-text-primary">牺牲方附魔</span>
              <button type="button" class="text-xs text-grass-400 hover:underline" @click="addEnchant(sacrificeEnchants)">
                + 添加附魔
              </button>
            </div>
            <p v-if="!sacrificeEnchants.length" class="text-xs text-text-muted">未添加（仅修复 / 重命名时可留空）</p>
            <div v-for="(en, i) in sacrificeEnchants" :key="i" class="mb-2 flex items-center gap-2">
              <select v-model="en.id" :class="inputClass" aria-label="牺牲方附魔">
                <option v-for="e in availableEnchants" :key="e.id" :value="e.id">{{ e.name }}（max {{ e.maxLevel }}）</option>
              </select>
              <input
                v-model.number="en.level"
                type="number"
                min="1"
                :max="maxLevelOf(en.id)"
                :class="monoClass"
                class="w-24"
                aria-label="等级"
              />
              <button
                type="button"
                class="h-10 shrink-0 rounded-md border border-border-dark px-3 text-sm text-text-muted hover:text-danger-mc"
                aria-label="删除附魔"
                @click="sacrificeEnchants.splice(i, 1)"
              >
                ✕
              </button>
            </div>
          </div>

          <div class="mt-4 flex flex-wrap gap-4">
            <label class="flex items-center gap-2 text-sm text-text-secondary">
              <input v-model="rename" type="checkbox" class="h-4 w-4" /> 同时重命名（+1 级）
            </label>
            <label
              class="flex items-center gap-2 text-sm"
              :class="sacrificeKind === 'item' ? 'text-text-secondary' : 'text-text-muted'"
            >
              <input v-model="targetDamaged" type="checkbox" class="h-4 w-4" :disabled="sacrificeKind === 'book'" />
              目标未满耐久（+2 级，仅同种物品）
            </label>
          </div>

          <div class="mt-5 flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
              @click="fillExample"
            >
              填入示例
            </button>
            <button type="button" class="text-sm text-text-muted transition hover:text-text-primary" @click="clearAll">
              清空
            </button>
          </div>
        </section>
      </div>

      <!-- 右：结果 -->
      <div class="space-y-6 lg:col-span-2">
        <section class="card p-5" aria-labelledby="cost-title">
          <div class="mb-4 flex items-center justify-between">
            <h2 id="cost-title" class="font-semibold text-text-primary">铁砧花费（经验等级）</h2>
            <CopyButton tool="enchant-calculator" :text="`铁砧合并花费：${cost.total} 级`" label="复制结论" />
          </div>

          <div
            class="mb-4 rounded-lg border px-4 py-3 text-center"
            :class="tooExpensive ? 'border-danger-mc/40 bg-danger-mc/10' : 'border-grass-500/30 bg-grass-500/10'"
          >
            <div class="font-mono text-3xl font-bold" :class="tooExpensive ? 'text-danger-mc' : 'text-grass-300'">
              {{ cost.total }}
            </div>
            <div class="mt-1 text-xs" :class="tooExpensive ? 'text-danger-mc' : 'text-grass-300'">
              {{ tooExpensive ? '过于昂贵：生存/冒险模式无法进行（≥40）' : '可在生存 / 冒险模式进行' }}
            </div>
          </div>

          <table class="w-full text-left text-sm">
            <tbody>
              <tr class="border-b border-border-dark/60">
                <td class="py-1.5 text-text-secondary">目标先作惩罚</td>
                <td class="py-1.5 text-right font-mono text-text-primary">{{ cost.penaltyTarget }}</td>
              </tr>
              <tr class="border-b border-border-dark/60">
                <td class="py-1.5 text-text-secondary">牺牲先作惩罚</td>
                <td class="py-1.5 text-right font-mono text-text-primary">{{ cost.penaltySacrifice }}</td>
              </tr>
              <tr class="border-b border-border-dark/60">
                <td class="py-1.5 text-text-secondary">重命名</td>
                <td class="py-1.5 text-right font-mono text-text-primary">{{ cost.rename }}</td>
              </tr>
              <tr class="border-b border-border-dark/60">
                <td class="py-1.5 text-text-secondary">耐久修复</td>
                <td class="py-1.5 text-right font-mono text-text-primary">{{ cost.repair }}</td>
              </tr>
              <tr class="border-b border-border-dark/60">
                <td class="py-1.5 text-text-secondary">附魔花费</td>
                <td class="py-1.5 text-right font-mono text-text-primary">{{ cost.enchant }}</td>
              </tr>
              <tr>
                <td class="py-2 font-medium text-text-primary">合计</td>
                <td class="py-2 text-right font-mono font-bold text-grass-300">{{ cost.total }}</td>
              </tr>
            </tbody>
          </table>

          <div class="mt-4">
            <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">结果附魔</h3>
            <p v-if="!combined.length" class="text-xs text-text-muted">无附魔</p>
            <ul v-else class="flex flex-wrap gap-1.5">
              <li
                v-for="e in combined"
                :key="e.id"
                class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-xs text-text-primary"
              >
                {{ ENCHANT_DATA_BY_ID[e.id]?.name ?? e.id }}
                <span class="text-grass-300">{{ e.level }}</span>
              </li>
            </ul>
          </div>

          <p class="mt-4 text-xs leading-relaxed text-text-muted">
            附魔花费（Java）= Σ 结果等级 × 合并乘数；互斥冲突每项额外 +1。
            互斥附魔自 {{ CONFLICT_ALLOWED_SINCE }} 起可合并，但牺牲方的该附魔会丢失。
          </p>

          <button
            type="button"
            class="mt-4 w-full rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
            @click="addSnapshot"
          >
            + 将当前方案加入对比
          </button>
        </section>

        <section v-if="snapshots.length" class="card p-5" aria-labelledby="compare-title">
          <div class="mb-3 flex items-center justify-between">
            <h2 id="compare-title" class="font-semibold text-text-primary">方案对比</h2>
            <button type="button" class="text-xs text-text-muted hover:text-danger-mc" @click="snapshots = []">清空</button>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b border-border-dark text-xs text-text-muted">
                  <th scope="col" class="px-2 py-2">装备</th>
                  <th scope="col" class="px-2 py-2">附魔</th>
                  <th scope="col" class="px-2 py-2 text-right">合计花费</th>
                  <th scope="col" class="px-2 py-2 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="s in snapshots" :key="s.id" class="border-b border-border-dark/60">
                  <td class="px-2 py-1.5 text-text-primary">{{ s.equipmentName }}</td>
                  <td class="px-2 py-1.5 text-xs text-text-secondary">{{ s.enchants }}</td>
                  <td class="px-2 py-1.5 text-right font-mono" :class="s.tooExpensive ? 'text-danger-mc' : 'text-grass-300'">
                    {{ s.total }}
                  </td>
                  <td class="px-2 py-1.5 text-right">
                    <button
                      type="button"
                      class="text-xs text-text-muted hover:text-danger-mc"
                      aria-label="移除该方案"
                      @click="snapshots = snapshots.filter((x) => x.id !== s.id)"
                    >
                      ✕
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="card p-5" aria-labelledby="penalty-title">
          <h2 id="penalty-title" class="mb-3 font-semibold text-text-primary">先作惩罚对照</h2>
          <p class="mb-3 text-xs text-text-muted">惩罚 = 2ⁿ − 1（n = 该物品已使用铁砧次数）。</p>
          <div class="grid grid-cols-3 gap-1.5 text-xs sm:grid-cols-5">
            <div v-for="row in penaltyRows" :key="row.n" class="rounded border border-border-dark bg-ink-900 px-2 py-1.5 text-center">
              <div class="text-text-muted">n={{ row.n }}</div>
              <div class="font-mono text-text-primary">{{ row.penalty }}</div>
            </div>
          </div>
        </section>

        <p class="text-xs leading-relaxed text-text-muted">{{ sourceNote }}</p>
      </div>
    </div>

    <!-- 附魔资料表 -->
    <section class="card mt-8 p-5" aria-labelledby="enchant-table-title">
      <h2 id="enchant-table-title" class="mb-3 font-semibold text-text-primary">
        附魔资料（当前装备：{{ EQUIPMENT.find((e) => e.id === equipment)?.name }}）
      </h2>
      <div class="overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead>
            <tr class="border-b border-border-dark text-xs text-text-muted">
              <th scope="col" class="px-2 py-2">附魔</th>
              <th scope="col" class="px-2 py-2">ID</th>
              <th scope="col" class="px-2 py-2">等级上限</th>
              <th scope="col" class="px-2 py-2">物品乘数</th>
              <th scope="col" class="px-2 py-2">书乘数</th>
              <th scope="col" class="px-2 py-2">效果</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="e in availableEnchants" :key="e.id" class="border-b border-border-dark/60">
              <td class="px-2 py-1.5 text-text-primary">{{ e.name }}</td>
              <td class="px-2 py-1.5 font-mono text-xs text-text-muted">{{ e.id }}</td>
              <td class="px-2 py-1.5 text-text-secondary">{{ e.maxLevel }}</td>
              <td class="px-2 py-1.5 text-text-secondary">{{ e.multiplierItem }}</td>
              <td class="px-2 py-1.5 text-text-secondary">{{ e.multiplierBook }}</td>
              <td class="px-2 py-1.5 text-text-secondary">{{ e.effect }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </PageContainer>
</template>
