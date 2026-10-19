// ===== 附魔与装备计算器（TODO §8）=====
//
// 规则来源：Minecraft Wiki《Anvil mechanics》（Java Edition，1.8+）。
// 仅实现**有明确公开依据**的部分；无法确认或随版本变动未核实的边界一律显式标注，
// 不把理论推导描述为游戏内确定结果（与需求「未确认时标记待核实」一致）。
//
// 已实现：
// - 附魔资料表（等级上限 / 适用装备 / 冲突分组 / 铁砧花费乘数）
// - 组合合法性校验（适用性、等级范围、互斥冲突，含 1.12.2 版本语义）
// - 铁砧花费公式：目标惩罚 + 牺牲惩罚 + 重命名 + 修复 + 附魔花费
// - 结果附魔推演（更高取高、相同且未满级 +1、更低不变、缺失则获得）
// - 先作惩罚（prior work penalty = 2ⁿ − 1）与同物品合并消耗
//
// 未覆盖 / 待核实（不假装支持）：
// - 耐久修复量（12% 上限、材料修复 25%/单位）与修复消耗的精确门槛
// - 附魔台花费、铁砧耐久损耗概率、Bedrock 版差异、1.8 之前机制
// - 新增附魔（如 spear 相关）与模组附魔

/** 装备类型标识 */
export type EquipmentId =
  | 'sword'
  | 'axe'
  | 'pickaxe'
  | 'shovel'
  | 'hoe'
  | 'mace'
  | 'shears'
  | 'fishing_rod'
  | 'bow'
  | 'crossbow'
  | 'trident'
  | 'shield'
  | 'elytra'
  | 'helmet'
  | 'chestplate'
  | 'leggings'
  | 'boots'
  | 'turtle_helmet'

export interface EquipmentDef {
  id: EquipmentId
  name: string
  group: '武器' | '工具' | '防具' | '远程' | '其他'
}

export const EQUIPMENT: EquipmentDef[] = [
  { id: 'sword', name: '剑', group: '武器' },
  { id: 'axe', name: '斧', group: '武器' },
  { id: 'mace', name: '重锤', group: '武器' },
  { id: 'pickaxe', name: '镐', group: '工具' },
  { id: 'shovel', name: '锹', group: '工具' },
  { id: 'hoe', name: '锄', group: '工具' },
  { id: 'shears', name: '剪刀', group: '工具' },
  { id: 'fishing_rod', name: '钓鱼竿', group: '工具' },
  { id: 'bow', name: '弓', group: '远程' },
  { id: 'crossbow', name: '弩', group: '远程' },
  { id: 'trident', name: '三叉戟', group: '远程' },
  { id: 'helmet', name: '头盔', group: '防具' },
  { id: 'turtle_helmet', name: '海龟壳', group: '防具' },
  { id: 'chestplate', name: '胸甲', group: '防具' },
  { id: 'leggings', name: '护腿', group: '防具' },
  { id: 'boots', name: '靴子', group: '防具' },
  { id: 'shield', name: '盾牌', group: '其他' },
  { id: 'elytra', name: '鞘翅', group: '其他' },
]

export const EQUIPMENT_BY_ID: Record<string, EquipmentDef> = Object.fromEntries(
  EQUIPMENT.map((e) => [e.id, e]),
)

const ALL_DURABLE: EquipmentId[] = EQUIPMENT.map((e) => e.id)

export interface EnchantDef {
  id: string
  name: string
  /** 等级上限 */
  maxLevel: number
  /** 铁砧合并花费乘数：[从物品合并, 从附魔书合并] */
  multiplierItem: number
  multiplierBook: number
  /** 适用装备 */
  applies: EquipmentId[]
  /** 简要效果（定性描述，不写未核实数值） */
  effect: string
}

/**
 * 附魔资料表。
 * 乘数、等级上限、适用装备与冲突分组均取自 Minecraft Wiki《Anvil mechanics》。
 */
export const ENCHANT_DATA: EnchantDef[] = [
  { id: 'protection', name: '保护', maxLevel: 4, multiplierItem: 1, multiplierBook: 1, applies: ['helmet', 'turtle_helmet', 'chestplate', 'leggings', 'boots'], effect: '减少大部分类型的伤害' },
  { id: 'fire_protection', name: '火焰保护', maxLevel: 4, multiplierItem: 2, multiplierBook: 1, applies: ['helmet', 'turtle_helmet', 'chestplate', 'leggings', 'boots'], effect: '减少火焰伤害与燃烧时间' },
  { id: 'feather_falling', name: '摔落保护', maxLevel: 4, multiplierItem: 2, multiplierBook: 1, applies: ['boots'], effect: '减少摔落伤害' },
  { id: 'blast_protection', name: '爆炸保护', maxLevel: 4, multiplierItem: 4, multiplierBook: 2, applies: ['helmet', 'turtle_helmet', 'chestplate', 'leggings', 'boots'], effect: '减少爆炸伤害与击退' },
  { id: 'projectile_protection', name: '弹射物保护', maxLevel: 4, multiplierItem: 2, multiplierBook: 1, applies: ['helmet', 'turtle_helmet', 'chestplate', 'leggings', 'boots'], effect: '减少弹射物伤害' },
  { id: 'thorns', name: '荆棘', maxLevel: 3, multiplierItem: 8, multiplierBook: 4, applies: ['helmet', 'turtle_helmet', 'chestplate', 'leggings', 'boots'], effect: '被近战攻击时反弹伤害' },
  { id: 'respiration', name: '水下呼吸', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['helmet', 'turtle_helmet'], effect: '延长水下憋气时间' },
  { id: 'aqua_affinity', name: '水下速掘', maxLevel: 1, multiplierItem: 4, multiplierBook: 2, applies: ['helmet', 'turtle_helmet'], effect: '水下挖掘速度不受影响' },
  { id: 'depth_strider', name: '深海探索者', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['boots'], effect: '提高水下移动速度' },
  { id: 'frost_walker', name: '冰霜行者', maxLevel: 2, multiplierItem: 4, multiplierBook: 2, applies: ['boots'], effect: '在水面结冰行走' },
  { id: 'soul_speed', name: '灵魂疾行', maxLevel: 3, multiplierItem: 8, multiplierBook: 4, applies: ['boots'], effect: '提高灵魂沙上移动速度' },
  { id: 'swift_sneak', name: '迅捷潜行', maxLevel: 3, multiplierItem: 8, multiplierBook: 4, applies: ['leggings'], effect: '提高潜行移动速度' },
  { id: 'sharpness', name: '锋利', maxLevel: 5, multiplierItem: 1, multiplierBook: 1, applies: ['sword', 'axe'], effect: '提高近战伤害' },
  { id: 'smite', name: '亡灵杀手', maxLevel: 5, multiplierItem: 2, multiplierBook: 1, applies: ['sword', 'axe', 'mace'], effect: '对亡灵生物造成额外伤害' },
  { id: 'bane_of_arthropods', name: '节肢杀手', maxLevel: 5, multiplierItem: 2, multiplierBook: 1, applies: ['sword', 'axe', 'mace'], effect: '对节肢生物造成额外伤害' },
  { id: 'knockback', name: '击退', maxLevel: 2, multiplierItem: 2, multiplierBook: 1, applies: ['sword'], effect: '增加击退距离' },
  { id: 'fire_aspect', name: '火焰附加', maxLevel: 2, multiplierItem: 4, multiplierBook: 2, applies: ['sword', 'mace'], effect: '命中目标使其着火' },
  { id: 'looting', name: '抢夺', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['sword'], effect: '提高生物掉落物数量' },
  { id: 'sweeping_edge', name: '横扫之刃', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['sword'], effect: '提高横扫攻击伤害' },
  { id: 'efficiency', name: '效率', maxLevel: 5, multiplierItem: 1, multiplierBook: 1, applies: ['pickaxe', 'shovel', 'axe', 'hoe', 'shears'], effect: '提高挖掘速度' },
  { id: 'silk_touch', name: '精准采集', maxLevel: 1, multiplierItem: 8, multiplierBook: 4, applies: ['pickaxe', 'shovel', 'axe', 'hoe'], effect: '方块以原样掉落' },
  { id: 'fortune', name: '时运', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['pickaxe', 'shovel', 'axe', 'hoe'], effect: '提高方块掉落数量' },
  { id: 'unbreaking', name: '耐久', maxLevel: 3, multiplierItem: 2, multiplierBook: 1, applies: ALL_DURABLE, effect: '降低耐久消耗概率' },
  { id: 'mending', name: '经验修补', maxLevel: 1, multiplierItem: 4, multiplierBook: 2, applies: ALL_DURABLE, effect: '用经验修复耐久' },
  { id: 'curse_of_binding', name: '绑定诅咒', maxLevel: 1, multiplierItem: 8, multiplierBook: 4, applies: ['helmet', 'turtle_helmet', 'chestplate', 'leggings', 'boots', 'elytra'], effect: '穿戴后无法取下' },
  { id: 'curse_of_vanishing', name: '消失诅咒', maxLevel: 1, multiplierItem: 8, multiplierBook: 4, applies: ALL_DURABLE, effect: '死亡后物品消失' },
  { id: 'power', name: '力量', maxLevel: 5, multiplierItem: 1, multiplierBook: 1, applies: ['bow'], effect: '提高箭矢伤害' },
  { id: 'punch', name: '冲击', maxLevel: 2, multiplierItem: 4, multiplierBook: 2, applies: ['bow'], effect: '提高箭矢击退' },
  { id: 'flame', name: '火矢', maxLevel: 1, multiplierItem: 4, multiplierBook: 2, applies: ['bow'], effect: '箭矢点燃目标' },
  { id: 'infinity', name: '无限', maxLevel: 1, multiplierItem: 8, multiplierBook: 4, applies: ['bow'], effect: '射击不消耗普通箭' },
  { id: 'luck_of_the_sea', name: '海之眷顾', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['fishing_rod'], effect: '提高钓到宝藏概率' },
  { id: 'lure', name: '饵钓', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['fishing_rod'], effect: '缩短钓鱼等待时间' },
  { id: 'impaling', name: '穿刺', maxLevel: 5, multiplierItem: 4, multiplierBook: 2, applies: ['trident'], effect: '对水生生物造成额外伤害' },
  { id: 'riptide', name: '激流', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['trident'], effect: '雨天或水中投掷自身' },
  { id: 'loyalty', name: '忠诚', maxLevel: 3, multiplierItem: 1, multiplierBook: 1, applies: ['trident'], effect: '投掷后返回' },
  { id: 'channeling', name: '引雷', maxLevel: 1, multiplierItem: 8, multiplierBook: 4, applies: ['trident'], effect: '雷雨天召唤闪电' },
  { id: 'multishot', name: '多重射击', maxLevel: 1, multiplierItem: 4, multiplierBook: 2, applies: ['crossbow'], effect: '一次射出多支箭' },
  { id: 'piercing', name: '穿透', maxLevel: 4, multiplierItem: 1, multiplierBook: 1, applies: ['crossbow'], effect: '箭矢可穿透多个目标' },
  { id: 'quick_charge', name: '快速装填', maxLevel: 3, multiplierItem: 2, multiplierBook: 1, applies: ['crossbow'], effect: '缩短装填时间' },
  { id: 'wind_burst', name: '风爆', maxLevel: 3, multiplierItem: 4, multiplierBook: 2, applies: ['mace'], effect: '重锤命中时产生风爆' },
  { id: 'density', name: '密度', maxLevel: 5, multiplierItem: 2, multiplierBook: 1, applies: ['mace'], effect: '下落攻击伤害随高度提升' },
  { id: 'breach', name: '破甲', maxLevel: 4, multiplierItem: 4, multiplierBook: 2, applies: ['mace'], effect: '无视部分护甲' },
]

export const ENCHANT_DATA_BY_ID: Record<string, EnchantDef> = Object.fromEntries(
  ENCHANT_DATA.map((e) => [e.id, e]),
)

/**
 * 互斥分组（Minecraft Wiki《Anvil mechanics》）。同一分组内的附魔互斥。
 * 冲突仅在两者都适用于当前装备时成立（例如精准采集 / 时运只对工具生效）。
 */
export const CONFLICT_GROUPS: string[][] = [
  ['sharpness', 'smite', 'bane_of_arthropods'],
  ['fortune', 'silk_touch'],
  ['protection', 'fire_protection', 'blast_protection', 'projectile_protection'],
  ['depth_strider', 'frost_walker'],
  ['infinity', 'mending'],
  ['multishot', 'piercing'],
  ['loyalty', 'riptide'],
  ['channeling', 'riptide'],
  ['density', 'breach', 'smite', 'bane_of_arthropods'],
  // 附魔书：上述武器类互斥同样适用于书
  ['sharpness', 'smite', 'bane_of_arthropods', 'density', 'breach', 'impaling'],
]

/** 版本 → 互斥附魔的处理语义（1.12.2 起允许合并但牺牲附魔丢失） */
export const CONFLICT_ALLOWED_SINCE = '1.12.2'

export interface EnchantLevel {
  id: string
  level: number
}

/** 列表中某附魔的等级（不存在返回 0） */
export function levelOf(list: EnchantLevel[], id: string): number {
  return list.find((e) => e.id === id)?.level ?? 0
}

/** 附魔是否适用于该装备 */
export function appliesTo(equipment: string, enchantId: string): boolean {
  const def = ENCHANT_DATA_BY_ID[enchantId]
  if (!def) return false
  return def.applies.includes(equipment as EquipmentId)
}

/** 该装备可用的附魔清单 */
export function equipmentEnchants(equipment: string): EnchantDef[] {
  return ENCHANT_DATA.filter((e) => appliesTo(equipment, e.id))
}

/** 版本是否 >= 1.12.2（互斥附魔可合并，牺牲附魔丢失） */
export function conflictAllowed(version: string): boolean {
  const m = /^(\d+)\.(\d+)(?:\.(\d+))?$/.exec((version ?? '').trim())
  if (!m) return true // 未识别（快照 / 预览版）按最新语义
  const [major, minor, patch] = [Number(m[1]), Number(m[2]), Number(m[3] ?? 0)]
  if (major !== 1) return major > 1
  if (minor !== 12) return minor > 12
  return patch >= 2
}

export interface CombinationIssue {
  kind: 'not_applicable' | 'level_range' | 'conflict' | 'duplicate' | 'unknown'
  /** 冲突时为两个附魔 ID */
  ids: string[]
  level: 'error' | 'warning'
  reason: string
}

/** 已知（喜欢）附魔列表的等级上限校验 + 组合冲突校验。 */
export function validateCombination(
  equipment: string,
  enchants: EnchantLevel[],
  version = '1.21',
): CombinationIssue[] {
  const issues: CombinationIssue[] = []
  const seen = new Set<string>()

  for (const e of enchants) {
    const def = ENCHANT_DATA_BY_ID[e.id]
    if (!def) {
      issues.push({ kind: 'unknown', ids: [e.id], level: 'error', reason: `未知附魔：${e.id}` })
      continue
    }
    if (seen.has(e.id)) {
      issues.push({ kind: 'duplicate', ids: [e.id], level: 'warning', reason: `${def.name} 重复出现` })
    }
    seen.add(e.id)
    if (!appliesTo(equipment, e.id)) {
      const eqName = EQUIPMENT_BY_ID[equipment]?.name ?? equipment
      issues.push({
        kind: 'not_applicable',
        ids: [e.id],
        level: 'error',
        reason: `${def.name} 不适用于${eqName}`,
      })
    }
    if (e.level < 1 || e.level > def.maxLevel || !Number.isInteger(e.level)) {
      issues.push({
        kind: 'level_range',
        ids: [e.id],
        level: 'error',
        reason: `${def.name} 等级须为 1-${def.maxLevel}`,
      })
    }
  }

  // 冲突检测
  for (const group of CONFLICT_GROUPS) {
    const active = group.filter((id) => seen.has(id) && appliesTo(equipment, id))
    for (let i = 0; i < active.length; i++) {
      for (let j = i + 1; j < active.length; j++) {
        const a = ENCHANT_DATA_BY_ID[active[i]]
        const b = ENCHANT_DATA_BY_ID[active[j]]
        if (!a || !b) continue
        const allowed = conflictAllowed(version)
        issues.push({
          kind: 'conflict',
          ids: [active[i], active[j]],
          level: allowed ? 'warning' : 'error',
          reason: allowed
            ? `${a.name} 与 ${b.name} 互斥：1.12.2 起可合并，但牺牲方的该附魔会丢失`
            : `${a.name} 与 ${b.name} 互斥：${version} 无法合并`,
        })
      }
    }
  }

  return issues
}

// ===== 铁砧花费 =====

/** 先作惩罚（prior work penalty）：使用次数的 2ⁿ − 1。 */
export function priorWorkPenalty(useCount: number): number {
  if (!Number.isInteger(useCount) || useCount < 0) return 0
  if (useCount > 30) return Number.MAX_SAFE_INTEGER
  return 2 ** useCount - 1
}

/** 一次操作后，结果物品的先作惩罚 = 2 × max(双方) + 1。 */
export function penaltyAfterOperation(a: number, b: number): number {
  return 2 * Math.max(a, b) + 1
}

/** 由先作惩罚反推使用次数（用于展示，非整数则返回 null）。 */
export function useCountFromPenalty(penalty: number): number | null {
  if (penalty < 0) return null
  const n = Math.log2(penalty + 1)
  return Number.isInteger(n) ? n : null
}

export interface AnvilInputSide {
  kind: 'item' | 'book'
  /** 已有先作惩罚使用次数 */
  useCount: number
  enchantments: EnchantLevel[]
}

export interface AnvilCostBreakdown {
  penaltyTarget: number
  penaltySacrifice: number
  rename: number
  repair: number
  enchant: number
  total: number
  /** Java 版：花费 ≥ 40 级时生存/冒险模式无法进行 */
  tooExpensive: boolean
}

export interface AnvilCostOptions {
  /** 目标是否重命名（+1 级） */
  rename?: boolean
  /** 目标是否未满耐久（+2 级；仅当牺牲为同种物品时） */
  targetDamaged?: boolean
  version?: string
}

/**
 * 结果附魔推演：
 * - 牺牲有、目标无 → 目标获得（若与目标已有附魔互斥则丢失）
 * - 牺牲更高 → 取高；相同且未满级 → +1；更低 → 不变
 */
export function resultEnchantments(
  equipment: string,
  target: EnchantLevel[],
  sacrifice: EnchantLevel[],
  version = '1.21',
): EnchantLevel[] {
  const result = target.map((e) => ({ ...e }))
  const allowed = conflictAllowed(version)

  for (const s of sacrifice) {
    const def = ENCHANT_DATA_BY_ID[s.id]
    if (!def || !appliesTo(equipment, s.id)) continue
    const cur = result.find((e) => e.id === s.id)
    if (!cur) {
      // 与目标已有附魔是否互斥
      const conflict = result.some((e) => conflicting(s.id, e.id))
      if (conflict && allowed) continue // 牺牲附魔丢失
      if (conflict && !allowed) continue
      result.push({ id: s.id, level: Math.min(s.level, def.maxLevel) })
      continue
    }
    if (s.level > cur.level) cur.level = Math.min(s.level, def.maxLevel)
    else if (s.level === cur.level && cur.level < def.maxLevel) cur.level += 1
  }
  return result
}

/** 两个附魔是否互斥（仅看分组，不判断装备适用性）。 */
export function conflicting(a: string, b: string): boolean {
  if (a === b) return false
  return CONFLICT_GROUPS.some((g) => g.includes(a) && g.includes(b))
}

/**
 * 计算铁砧合并花费（Java Edition 公式）：
 * 总花费 = 目标先作惩罚 + 牺牲先作惩罚 + 重命名(1) + 修复(2) + 附魔花费
 * 附魔花费（Java）= Σ（结果等级 × 乘数）+ 不兼容附魔数 × 1
 */
export function computeAnvilCost(
  equipment: string,
  target: AnvilInputSide,
  sacrifice: AnvilInputSide,
  opts: AnvilCostOptions = {},
): AnvilCostBreakdown {
  const version = opts.version ?? '1.21'
  const penaltyTarget = priorWorkPenalty(target.useCount)
  const penaltySacrifice = priorWorkPenalty(sacrifice.useCount)
  const rename = opts.rename ? 1 : 0
  // 修复花费仅当牺牲为同种物品（非附魔书）且目标未满耐久
  const repair = target.kind === 'item' && sacrifice.kind === 'item' && opts.targetDamaged ? 2 : 0

  const finalEnchants = resultEnchantments(equipment, target.enchantments, sacrifice.enchantments, version)
  let enchantCost = 0
  for (const s of sacrifice.enchantments) {
    const def = ENCHANT_DATA_BY_ID[s.id]
    if (!def || !appliesTo(equipment, s.id)) continue
    // 每有一个与目标冲突的附魔 +1
    enchantCost += target.enchantments.filter((t) => conflicting(t.id, s.id)).length
    const finalLevel = finalEnchants.find((e) => e.id === s.id)?.level ?? 0
    if (finalLevel > 0) {
      enchantCost += finalLevel * (sacrifice.kind === 'book' ? def.multiplierBook : def.multiplierItem)
    }
  }

  const total = penaltyTarget + penaltySacrifice + rename + repair + enchantCost
  return {
    penaltyTarget,
    penaltySacrifice,
    rename,
    repair,
    enchant: enchantCost,
    total,
    tooExpensive: total >= 40,
  }
}
