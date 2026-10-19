// ===== 可视化指令生成器（TODO §7）=====
//
// 设计目标：把「拼指令字符串」变成「填表单」，并在生成前做校验，
// 同时按 Minecraft Java 版版本区分**旧式 NBT** 与**新版数据组件**语法。
//
// 核心原则（与需求一致）：
// 1. 生成逻辑为**纯函数**，不读写全局状态，便于表驱动测试（TODO §7 测试要求）。
// 2. 只生成文本，**不执行、不连接任何服务器**。
// 3. 版本差异基于可验证规则：
//    - 物品数据组件（`item[k=v]`）自 **1.20.5** 起取代物品 NBT（`{...}`）；
//    - `/effect` 自 **1.19** 起改为 `/effect give|clear` 子命令形式；
//    - 实体仍使用 NBT（`/summon` 的数据标签）。
// 4. 快照 / 预览版无法精确裁决语法，按**最新语法**生成并显式标注「按最新语法」，
//    避免把不确定的版本描述成确定结论。

/** 指令语法模式 */
export type CommandSyntax = 'nbt' | 'components'

/** 语义化版本（仅数字段） */
export interface SemVer {
  major: number
  minor: number
  patch: number
}

/** 字段级校验问题 */
export interface FieldIssue {
  /** 字段名（与表单字段对应，供视图就近显示） */
  field: string
  reason: string
}

/** 生成结果 */
export type CommandResult =
  | { ok: true; command: string; notes: string[] }
  | { ok: false; issues: FieldIssue[] }

/**
 * 统一把表单值当字符串读取。
 * 原因：`<input type="number">` 上的 `v-model` 会让 Vue 把值自动转成 number，
 * 而校验逻辑按字符串处理（`trim()`），因此这里做一次归一，避免出现
 * `value.trim is not a function` 之类的运行期错误。
 */
function asText(value: unknown): string {
  return value === null || value === undefined ? '' : String(value)
}

/** 数据组件语法生效版本：1.20.5 */
export const COMPONENT_SINCE: SemVer = { major: 1, minor: 20, patch: 5 }
/** `/effect give` 子命令生效版本：1.19 */
export const EFFECT_GIVE_SINCE: SemVer = { major: 1, minor: 19, patch: 0 }

/** 解析正式版版本号（`1.21` / `1.20.5`）。快照 / 预览版 / 空值返回 null。 */
export function parseReleaseVersion(raw: string): SemVer | null {
  const m = /^(\d+)\.(\d+)(?:\.(\d+))?$/.exec(asText(raw).trim())
  if (!m) return null
  return { major: Number(m[1]), minor: Number(m[2]), patch: Number(m[3] ?? 0) }
}

/** 比较两个语义化版本：a<b 返回负数，a>b 返回正数，相等返回 0。 */
export function compareSemVer(a: SemVer, b: SemVer): number {
  if (a.major !== b.major) return a.major - b.major
  if (a.minor !== b.minor) return a.minor - b.minor
  return a.patch - b.patch
}

/** 是否为快照（如 `24w14a`）或预览版（如 `1.21-pre1` / `1.21-rc1`）。 */
export function isSnapshotOrPreRelease(raw: string): boolean {
  const v = asText(raw).trim()
  if (v === '') return false
  return /\d+w\d+[a-z]/i.test(v) || /-(pre|rc)\d+/i.test(v)
}

export interface SyntaxDecision {
  syntax: CommandSyntax
  /** 版本无法精确裁决（快照 / 预览版 / 非法输入）时为 true */
  assumed: boolean
  note?: string
}

/**
 * 依据目标版本裁决物品指令语法。
 * - 合法正式版 >= 1.20.5 → components；否则 → nbt。
 * - 快照 / 预览版 / 无法识别 → components，并标记 assumed（按最新语法）。
 */
export function syntaxForVersion(version: string): SyntaxDecision {
  const parsed = parseReleaseVersion(version)
  if (parsed) {
    return {
      syntax: compareSemVer(parsed, COMPONENT_SINCE) >= 0 ? 'components' : 'nbt',
      assumed: false,
    }
  }
  return {
    syntax: 'components',
    assumed: true,
    note: '未识别为正式版版本号，按最新数据组件语法生成；请自行核对。',
  }
}

/** `/effect` 语法形态裁决。 */
export function effectFormForVersion(version: string): { form: 'give' | 'legacy'; assumed: boolean } {
  const parsed = parseReleaseVersion(version)
  if (parsed) {
    return { form: compareSemVer(parsed, EFFECT_GIVE_SINCE) >= 0 ? 'give' : 'legacy', assumed: false }
  }
  return { form: 'give', assumed: true }
}

// ===== 通用校验原语 =====

const NAMESPACE_RE = /^[a-z0-9_.-]+:[a-z0-9_./-]+$/
const TARGET_RE = /^(?:@[parese](?:\[[^\]]*\])?|[A-Za-z0-9_]{1,16})$/
const NUMBER = String.raw`-?\d+(?:\.\d+)?`
const POSITION_RE = new RegExp(String.raw`^(?:${NUMBER}|~(?:${NUMBER})?|\^(?:${NUMBER})?)$`)

/** 规范化命名空间 ID：缺省补 `minecraft:`，非法返回 null。 */
export function normalizeNamespacedId(raw: string, fallbackNamespace = 'minecraft'): string | null {
  const text = asText(raw).trim().toLowerCase().replace(/\s+/g, '_')
  if (text === '') return null
  const full = text.includes(':') ? text : `${fallbackNamespace}:${text}`
  return NAMESPACE_RE.test(full) ? full : null
}

/** 校验目标选择器或玩家名。 */
export function isValidTarget(raw: string): boolean {
  return TARGET_RE.test(asText(raw).trim())
}

/** 校验单个坐标分量（绝对 / 相对 `~` / 局部 `^`）。 */
export function isValidPositionComponent(raw: string): boolean {
  return POSITION_RE.test(asText(raw).trim())
}

export interface Position {
  /** 数字输入框的 v-model 会得到 number，故允许二者 */
  x: string | number
  y: string | number
  z: string | number
}

/** 校验三元坐标；空值与非法值分别给出问题。 */
export function validatePosition(pos: Position, field = 'pos'): FieldIssue[] {
  const issues: FieldIssue[] = []
  const labels: Array<[keyof Position, string]> = [
    ['x', 'X'],
    ['y', 'Y'],
    ['z', 'Z'],
  ]
  for (const [key, label] of labels) {
    const raw = asText(pos[key]).trim()
    if (raw === '') {
      issues.push({ field, reason: `${label} 坐标不能为空` })
    } else if (!isValidPositionComponent(raw)) {
      issues.push({ field, reason: `${label} 坐标格式非法（绝对 / ~ 相对 / ^ 局部）` })
    }
  }
  return issues
}

/** 坐标拼接为 `x y z`（调用方需先校验）。 */
export function formatPosition(pos: Position): string {
  return `${asText(pos.x).trim()} ${asText(pos.y).trim()} ${asText(pos.z).trim()}`
}

/** NBT / 组件文本的花括号配平检查（仅做浅层结构检查，不做完整解析）。 */
export function isBalancedBraces(text: string): boolean {
  let depth = 0
  let quote: string | null = null
  for (const ch of text) {
    if (quote) {
      if (ch === quote) quote = null
      continue
    }
    if (ch === '"' || ch === "'") {
      quote = ch
      continue
    }
    if (ch === '{' || ch === '[') depth++
    else if (ch === '}' || ch === ']') {
      depth--
      if (depth < 0) return false
    }
  }
  return depth === 0 && quote === null
}

/** 把字符串渲染为单引号包裹的 JSON 文本组件值：`"Name"` → `'"Name"'`。 */
export function jsonTextComponent(text: string): string {
  return `'${JSON.stringify(text)}'`
}

// ===== 附魔 / 状态 / 药水等可选值清单 =====
//
// 仅收录生成指令所需的**标识与上限**，不描述具体数值效果（避免把未核实数值写成确定事实）。

export interface EnchantOption {
  /** 命名空间 ID（不含 minecraft: 前缀，与游戏内短名一致） */
  id: string
  name: string
  maxLevel: number
}

/** 常用附魔（覆盖工具 / 武器 / 防具 / 通用）。 */
export const ENCHANTMENTS: EnchantOption[] = [
  { id: 'sharpness', name: '锋利', maxLevel: 5 },
  { id: 'smite', name: '亡灵杀手', maxLevel: 5 },
  { id: 'bane_of_arthropods', name: '节肢杀手', maxLevel: 5 },
  { id: 'knockback', name: '击退', maxLevel: 2 },
  { id: 'fire_aspect', name: '火焰附加', maxLevel: 2 },
  { id: 'looting', name: '抢夺', maxLevel: 3 },
  { id: 'sweeping_edge', name: '横扫之刃', maxLevel: 3 },
  { id: 'efficiency', name: '效率', maxLevel: 5 },
  { id: 'silk_touch', name: '精准采集', maxLevel: 1 },
  { id: 'fortune', name: '时运', maxLevel: 3 },
  { id: 'unbreaking', name: '耐久', maxLevel: 3 },
  { id: 'mending', name: '经验修补', maxLevel: 1 },
  { id: 'protection', name: '保护', maxLevel: 4 },
  { id: 'fire_protection', name: '火焰保护', maxLevel: 4 },
  { id: 'blast_protection', name: '爆炸保护', maxLevel: 4 },
  { id: 'projectile_protection', name: '弹射物保护', maxLevel: 4 },
  { id: 'feather_falling', name: '摔落保护', maxLevel: 4 },
  { id: 'respiration', name: '水下呼吸', maxLevel: 3 },
  { id: 'aqua_affinity', name: '水下速掘', maxLevel: 1 },
  { id: 'thorns', name: '荆棘', maxLevel: 3 },
  { id: 'depth_strider', name: '深海探索者', maxLevel: 3 },
  { id: 'frost_walker', name: '冰霜行者', maxLevel: 2 },
  { id: 'soul_speed', name: '灵魂疾行', maxLevel: 3 },
  { id: 'swift_sneak', name: '迅捷潜行', maxLevel: 3 },
  { id: 'power', name: '力量', maxLevel: 5 },
  { id: 'punch', name: '冲击', maxLevel: 2 },
  { id: 'flame', name: '火矢', maxLevel: 1 },
  { id: 'infinity', name: '无限', maxLevel: 1 },
  { id: 'multishot', name: '多重射击', maxLevel: 1 },
  { id: 'quick_charge', name: '快速装填', maxLevel: 3 },
  { id: 'piercing', name: '穿透', maxLevel: 4 },
  { id: 'loyalty', name: '忠诚', maxLevel: 3 },
  { id: 'impaling', name: '穿刺', maxLevel: 5 },
  { id: 'riptide', name: '激流', maxLevel: 3 },
  { id: 'channeling', name: '引雷', maxLevel: 1 },
  { id: 'curse_of_vanishing', name: '消失诅咒', maxLevel: 1 },
  { id: 'curse_of_binding', name: '绑定诅咒', maxLevel: 1 },
]

export const ENCHANT_BY_ID: Record<string, EnchantOption> = Object.fromEntries(
  ENCHANTMENTS.map((e) => [e.id, e]),
)

export interface EffectOption {
  id: string
  name: string
}

/** 常用状态效果（药水 / effect 指令共用）。 */
export const EFFECTS: EffectOption[] = [
  { id: 'speed', name: '迅捷' },
  { id: 'slowness', name: '缓慢' },
  { id: 'haste', name: '急迫' },
  { id: 'mining_fatigue', name: '挖掘疲劳' },
  { id: 'strength', name: '力量' },
  { id: 'instant_health', name: '瞬间治疗' },
  { id: 'instant_damage', name: '瞬间伤害' },
  { id: 'jump_boost', name: '跳跃提升' },
  { id: 'nausea', name: '反胃' },
  { id: 'regeneration', name: '生命恢复' },
  { id: 'resistance', name: '抗性提升' },
  { id: 'fire_resistance', name: '防火' },
  { id: 'water_breathing', name: '水下呼吸' },
  { id: 'invisibility', name: '隐身' },
  { id: 'blindness', name: '失明' },
  { id: 'night_vision', name: '夜视' },
  { id: 'hunger', name: '饥饿' },
  { id: 'weakness', name: '虚弱' },
  { id: 'poison', name: '中毒' },
  { id: 'wither', name: '凋零' },
  { id: 'health_boost', name: '生命提升' },
  { id: 'absorption', name: '伤害吸收' },
  { id: 'saturation', name: '饱和' },
  { id: 'glowing', name: '发光' },
  { id: 'levitation', name: '飘浮' },
  { id: 'slow_falling', name: '缓降' },
  { id: 'conduit_power', name: '潮涌能量' },
  { id: 'dolphins_grace', name: '海豚的恩惠' },
  { id: 'bad_omen', name: '不祥之兆' },
  { id: 'hero_of_the_village', name: '村庄英雄' },
  { id: 'darkness', name: '黑暗' },
]

export const GAMEMODES = ['survival', 'creative', 'adventure', 'spectator'] as const
export type Gamemode = (typeof GAMEMODES)[number]

export const GAMEMODE_LABELS: Record<Gamemode, string> = {
  survival: '生存',
  creative: '创造',
  adventure: '冒险',
  spectator: '旁观',
}

export const SETBLOCK_MODES = ['replace', 'destroy', 'keep'] as const
export type SetblockMode = (typeof SETBLOCK_MODES)[number]

// ===== 指令构建器 =====

/** `/give` 输入 */
export interface GiveOptions {
  target: string
  item: string
  count: string | number
  enchantments: Array<{ id: string; level: string | number }>
  customName?: string
  unbreakable?: boolean
  damage?: string | number
}

function buildEnchantNbt(enchantments: Array<{ id: string; level: number }>): string {
  const parts = enchantments.map((e) => `{id:"${e.id}",lvl:${e.level}}`)
  return `Enchantments:[${parts.join(',')}]`
}

function buildEnchantComponent(enchantments: Array<{ id: string; level: number }>): string {
  const levels = enchantments.map((e) => `"minecraft:${e.id}":${e.level}`).join(',')
  return `minecraft:enchantments={levels:{${levels}}}`
}

/** 生成 `/give`。 */
export function buildGive(opts: GiveOptions, syntax: CommandSyntax): CommandResult {
  const issues: FieldIssue[] = []
  const notes: string[] = []

  const target = (opts.target ?? '').trim()
  if (target === '') issues.push({ field: 'target', reason: '目标不能为空' })
  else if (!isValidTarget(target)) issues.push({ field: 'target', reason: '目标须为选择器（@p/@a/@s/@r/@e）或合法玩家名' })

  const item = normalizeNamespacedId(opts.item ?? '')
  if (!item) issues.push({ field: 'item', reason: '物品 ID 非法（示例：diamond_sword / minecraft:diamond_sword）' })

  const countNum = Number(asText(opts.count).trim())
  if (!Number.isInteger(countNum) || countNum < 1 || countNum > 6400) {
    issues.push({ field: 'count', reason: '数量须为 1-6400 的整数' })
  }

  const enchantments: Array<{ id: string; level: number }> = []
  for (const [i, e] of (opts.enchantments ?? []).entries()) {
    const id = asText(e.id).trim()
    if (id === '' && asText(e.level).trim() === '') continue
    if (!ENCHANT_BY_ID[id]) {
      issues.push({ field: `enchant.${i}`, reason: '未知附魔，请从列表选择' })
      continue
    }
    const level = Number(asText(e.level).trim())
    const max = ENCHANT_BY_ID[id].maxLevel
    if (!Number.isInteger(level) || level < 1 || level > max) {
      issues.push({ field: `enchant.${i}`, reason: `${ENCHANT_BY_ID[id].name} 等级须为 1-${max} 的整数` })
      continue
    }
    enchantments.push({ id, level })
  }

  let damageNum: number | null = null
  if (asText(opts.damage).trim() !== '') {
    damageNum = Number(asText(opts.damage).trim())
    if (!Number.isInteger(damageNum) || damageNum < 0) {
      issues.push({ field: 'damage', reason: '损伤值须为非负整数' })
    }
  }

  if (issues.length) return { ok: false, issues }

  let modifier = ''
  if (syntax === 'nbt') {
    const nbt: string[] = []
    if (enchantments.length) nbt.push(buildEnchantNbt(enchantments))
    if (opts.unbreakable) nbt.push('Unbreakable:1b')
    if (opts.customName && asText(opts.customName).trim() !== '') {
      nbt.push(`display:{Name:${jsonTextComponent(asText(opts.customName).trim())}}`)
    }
    if (damageNum !== null) nbt.push(`Damage:${damageNum}`)
    modifier = nbt.length ? ` {${nbt.join(',')}}` : ''
    if (enchantments.length || opts.customName || opts.unbreakable || damageNum !== null) {
      notes.push('旧式物品 NBT 语法（适用于 1.20.4 及更早版本）。')
    }
  } else {
    const comps: string[] = []
    if (enchantments.length) comps.push(buildEnchantComponent(enchantments))
    if (opts.unbreakable) comps.push('minecraft:unbreakable={}')
    if (opts.customName && asText(opts.customName).trim() !== '') {
      comps.push(`minecraft:custom_name=${jsonTextComponent(asText(opts.customName).trim())}`)
    }
    if (damageNum !== null) comps.push(`minecraft:damage=${damageNum}`)
    modifier = comps.length ? `[${comps.join(',')}]` : ''
    if (comps.length) notes.push('新版物品数据组件语法（1.20.5+）。')
  }

  return { ok: true, command: `give ${target} ${item}${modifier} ${countNum}`, notes }
}

/** `/summon` 输入 */
export interface SummonOptions {
  entity: string
  pos: Position
  customName?: string
  noAI?: boolean
  noGravity?: boolean
  silent?: boolean
  persistenceRequired?: boolean
  glowing?: boolean
}

/** 生成 `/summon`（实体数据标签始终为 NBT）。 */
export function buildSummon(opts: SummonOptions): CommandResult {
  const issues: FieldIssue[] = []

  const entity = normalizeNamespacedId(opts.entity ?? '')
  if (!entity) issues.push({ field: 'entity', reason: '实体 ID 非法（示例：zombie / minecraft:zombie）' })

  issues.push(...validatePosition(opts.pos))

  if (issues.length) return { ok: false, issues }

  const nbt: string[] = []
  if (opts.customName && asText(opts.customName).trim() !== '') {
    nbt.push(`CustomName:${jsonTextComponent(asText(opts.customName).trim())}`)
  }
  if (opts.noAI) nbt.push('NoAI:1b')
  if (opts.noGravity) nbt.push('NoGravity:1b')
  if (opts.silent) nbt.push('Silent:1b')
  if (opts.persistenceRequired) nbt.push('PersistenceRequired:1b')
  if (opts.glowing) nbt.push('Glowing:1b')

  const nbtPart = nbt.length ? ` {${nbt.join(',')}}` : ''
  const notes = ['实体数据始终使用 NBT 标签；`~` 表示相对坐标。']
  return { ok: true, command: `summon ${entity} ${formatPosition(opts.pos)}${nbtPart}`, notes }
}

/** `/effect` 输入 */
export interface EffectOptions {
  target: string
  effect: string
  duration: string | number
  amplifier: string | number
  hideParticles?: boolean
}

/** 生成 `/effect`，按版本选择 `give` 子命令或旧语法。 */
export function buildEffect(opts: EffectOptions, version: string): CommandResult {
  const issues: FieldIssue[] = []

  const target = (opts.target ?? '').trim()
  if (target === '') issues.push({ field: 'target', reason: '目标不能为空' })
  else if (!isValidTarget(target)) issues.push({ field: 'target', reason: '目标须为选择器或合法玩家名' })

  const effect = normalizeNamespacedId(opts.effect ?? '')
  if (!effect) issues.push({ field: 'effect', reason: '状态效果 ID 非法（示例：speed / minecraft:speed）' })

  const duration = Number(asText(opts.duration).trim())
  if (!Number.isInteger(duration) || duration < 1) {
    issues.push({ field: 'duration', reason: '持续时间须为 ≥1 的整数（秒）' })
  }

  const amplifier = Number(asText(opts.amplifier).trim())
  if (!Number.isInteger(amplifier) || amplifier < 0 || amplifier > 255) {
    issues.push({ field: 'amplifier', reason: '等级须为 0-255 的整数（0 = 一级）' })
  }

  if (issues.length) return { ok: false, issues }

  const { form, assumed } = effectFormForVersion(version)
  const notes: string[] = []
  const head = form === 'give' ? `effect give ${target} ${effect}` : `effect ${target} ${effect}`
  const tail = opts.hideParticles ? ' true' : ''
  if (form === 'legacy') notes.push('1.19 之前的 `/effect` 语法（无 give/clear 子命令）。')
  else notes.push('`/effect give` 语法（1.19+）。')
  if (assumed) notes.push('版本无法精确裁决，按新语法生成。')

  return { ok: true, command: `${head} ${duration} ${amplifier}${tail}`, notes }
}

/** `/setblock` 输入 */
export interface SetblockOptions {
  pos: Position
  block: string
  states?: Array<{ key: string; value: string }>
  mode?: SetblockMode
}

/** 生成 `/setblock`。 */
export function buildSetblock(opts: SetblockOptions): CommandResult {
  const issues: FieldIssue[] = []

  issues.push(...validatePosition(opts.pos))

  const block = normalizeNamespacedId(opts.block ?? '')
  if (!block) issues.push({ field: 'block', reason: '方块 ID 非法（示例：oak_planks / minecraft:oak_planks）' })

  const states: string[] = []
  for (const [i, s] of (opts.states ?? []).entries()) {
    const key = asText(s.key).trim()
    const value = asText(s.value).trim()
    if (key === '' && value === '') continue
    if (!/^[a-z0-9_]+$/.test(key)) {
      issues.push({ field: `state.${i}`, reason: '状态名仅允许小写字母、数字与下划线' })
      continue
    }
    if (!/^[a-z0-9_]+$/.test(value)) {
      issues.push({ field: `state.${i}`, reason: '状态值仅允许小写字母、数字与下划线' })
      continue
    }
    states.push(`${key}=${value}`)
  }

  if (issues.length) return { ok: false, issues }

  const statePart = states.length ? `[${states.join(',')}]` : ''
  const mode = opts.mode && opts.mode !== 'replace' ? ` ${opts.mode}` : ''
  const notes: string[] = []
  if (opts.mode && opts.mode !== 'replace') notes.push(`方块处理模式：${opts.mode}。`)
  return { ok: true, command: `setblock ${formatPosition(opts.pos)} ${block}${statePart}${mode}`, notes }
}

/** `/tp` 输入 */
export interface TpOptions {
  target: string
  mode: 'coords' | 'entity'
  pos: Position
  destination?: string
  rotation?: { yaw: string; pitch: string }
}

/** 生成 `/tp`（坐标或实体目标）。 */
export function buildTp(opts: TpOptions): CommandResult {
  const issues: FieldIssue[] = []
  const notes: string[] = []

  const target = (opts.target ?? '').trim()
  if (target === '') issues.push({ field: 'target', reason: '目标不能为空' })
  else if (!isValidTarget(target)) issues.push({ field: 'target', reason: '目标须为选择器或合法玩家名' })

  if (opts.mode === 'entity') {
    const dest = (opts.destination ?? '').trim()
    if (dest === '') issues.push({ field: 'destination', reason: '目标实体不能为空' })
    else if (!isValidTarget(dest)) issues.push({ field: 'destination', reason: '目标须为选择器或合法玩家名' })
    if (issues.length) return { ok: false, issues }
    return { ok: true, command: `tp ${target} ${dest}`, notes }
  }

  issues.push(...validatePosition(opts.pos))

  const rot = opts.rotation
  let rotPart = ''
  if (rot && ((rot.yaw ?? '').trim() !== '' || (rot.pitch ?? '').trim() !== '')) {
    const yaw = (rot.yaw ?? '').trim()
    const pitch = (rot.pitch ?? '').trim()
    const num = /^-?\d+(?:\.\d+)?$/
    if (!num.test(yaw)) issues.push({ field: 'yaw', reason: '水平角须为数值' })
    if (!num.test(pitch)) issues.push({ field: 'pitch', reason: '俯仰角须为数值' })
    rotPart = ` ${yaw} ${pitch}`
  }

  if (issues.length) return { ok: false, issues }
  return { ok: true, command: `tp ${target} ${formatPosition(opts.pos)}${rotPart}`, notes }
}

/** `/gamemode` 输入 */
export interface GamemodeOptions {
  mode: Gamemode
  target?: string
}

/** 生成 `/gamemode`。 */
export function buildGamemode(opts: GamemodeOptions): CommandResult {
  const issues: FieldIssue[] = []
  if (!GAMEMODES.includes(opts.mode)) {
    issues.push({ field: 'mode', reason: '游戏模式非法' })
  }
  const target = (opts.target ?? '').trim()
  if (target !== '' && !isValidTarget(target)) {
    issues.push({ field: 'target', reason: '目标须为选择器或合法玩家名' })
  }
  if (issues.length) return { ok: false, issues }
  return { ok: true, command: `gamemode ${opts.mode}${target ? ` ${target}` : ''}`, notes: [] }
}

/** 把生成结果转换为最终命令文本（前缀 `/`）。 */
export function withSlash(command: string): string {
  return `/${command}`
}
