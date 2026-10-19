// 附魔与装备计算器单元测试（TODO §8：为计算规则增加测试用例）
//
// 期望值依据 Minecraft Wiki《Anvil mechanics》中的公开示例与公式。
import { describe, expect, it } from 'vitest'
import {
  appliesTo,
  computeAnvilCost,
  conflictAllowed,
  equipmentEnchants,
  levelOf,
  penaltyAfterOperation,
  priorWorkPenalty,
  resultEnchantments,
  useCountFromPenalty,
  validateCombination,
} from '@/utils/enchant-calculator'

describe('先作惩罚（prior work penalty）', () => {
  it('符合 2ⁿ − 1', () => {
    expect(priorWorkPenalty(0)).toBe(0)
    expect(priorWorkPenalty(1)).toBe(1)
    expect(priorWorkPenalty(2)).toBe(3)
    expect(priorWorkPenalty(3)).toBe(7)
    expect(priorWorkPenalty(4)).toBe(15)
    expect(priorWorkPenalty(5)).toBe(31)
  })

  it('一次操作后 = 2 × max + 1', () => {
    expect(penaltyAfterOperation(0, 0)).toBe(1)
    expect(penaltyAfterOperation(3, 1)).toBe(7)
    expect(penaltyAfterOperation(15, 15)).toBe(31)
  })

  it('可由惩罚反推使用次数', () => {
    expect(useCountFromPenalty(7)).toBe(3)
    expect(useCountFromPenalty(5)).toBeNull()
  })
})

describe('适用性', () => {
  it('按装备类型判定', () => {
    expect(appliesTo('sword', 'sharpness')).toBe(true)
    expect(appliesTo('boots', 'sharpness')).toBe(false)
    expect(appliesTo('pickaxe', 'silk_touch')).toBe(true)
    expect(appliesTo('sword', 'silk_touch')).toBe(false)
    expect(appliesTo('bow', 'mending')).toBe(true)
    expect(appliesTo('nope', 'sharpness')).toBe(false)
  })

  it('装备可用附魔清单随装备变化', () => {
    const swordIds = equipmentEnchants('sword').map((e) => e.id)
    expect(swordIds).toContain('sharpness')
    expect(swordIds).toContain('looting')
    expect(swordIds).not.toContain('silk_touch')
    expect(swordIds).not.toContain('power')
  })
})

describe('组合校验', () => {
  it('互斥附魔在 1.12.2+ 为警告（可合并但牺牲附魔丢失）', () => {
    const issues = validateCombination(
      'sword',
      [
        { id: 'sharpness', level: 5 },
        { id: 'smite', level: 3 },
      ],
      '1.21',
    )
    const conflict = issues.find((i) => i.kind === 'conflict')
    expect(conflict?.level).toBe('warning')
    expect(conflict?.reason).toContain('丢失')
  })

  it('1.12.2 之前互斥为错误', () => {
    const issues = validateCombination(
      'sword',
      [
        { id: 'sharpness', level: 5 },
        { id: 'smite', level: 3 },
      ],
      '1.11',
    )
    expect(issues.find((i) => i.kind === 'conflict')?.level).toBe('error')
  })

  it('不适用于装备的附魔报错', () => {
    const issues = validateCombination('boots', [{ id: 'sharpness', level: 1 }], '1.21')
    expect(issues.find((i) => i.kind === 'not_applicable')?.level).toBe('error')
  })

  it('超出等级上限与重复出现', () => {
    const over = validateCombination('sword', [{ id: 'sharpness', level: 6 }], '1.21')
    expect(over.find((i) => i.kind === 'level_range')?.reason).toContain('1-5')
    const dup = validateCombination(
      'sword',
      [
        { id: 'looting', level: 3 },
        { id: 'looting', level: 3 },
      ],
      '1.21',
    )
    expect(dup.find((i) => i.kind === 'duplicate')?.level).toBe('warning')
  })

  it('冲突仅在两者都适用时成立（精准采集 / 时运对剑不冲突）', () => {
    const issues = validateCombination(
      'sword',
      [
        { id: 'silk_touch', level: 1 },
        { id: 'fortune', level: 3 },
      ],
      '1.21',
    )
    // 两者对剑都不适用 → 只有 not_applicable，不应有 conflict
    expect(issues.some((i) => i.kind === 'conflict')).toBe(false)
    expect(issues.filter((i) => i.kind === 'not_applicable')).toHaveLength(2)
  })

  it('版本语义边界', () => {
    expect(conflictAllowed('1.11')).toBe(false)
    expect(conflictAllowed('1.12.1')).toBe(false)
    expect(conflictAllowed('1.12.2')).toBe(true)
    expect(conflictAllowed('1.21')).toBe(true)
    expect(conflictAllowed('24w14a')).toBe(true)
  })
})

describe('结果附魔推演', () => {
  it('wiki 示例：更高取高、相同未满级 +1、已满级不变', () => {
    const result = resultEnchantments(
      'sword',
      [
        { id: 'sharpness', level: 3 },
        { id: 'knockback', level: 2 },
        { id: 'looting', level: 3 },
      ],
      [
        { id: 'sharpness', level: 3 },
        { id: 'looting', level: 3 },
      ],
      '1.21',
    )
    expect(levelOf(result, 'sharpness')).toBe(4)
    expect(levelOf(result, 'knockback')).toBe(2)
    expect(levelOf(result, 'looting')).toBe(3)
  })

  it('目标缺失则获得；牺牲更低则不变', () => {
    const gained = resultEnchantments('sword', [], [{ id: 'sharpness', level: 3 }], '1.21')
    expect(levelOf(gained, 'sharpness')).toBe(3)
    const lower = resultEnchantments('sword', [{ id: 'sharpness', level: 5 }], [{ id: 'sharpness', level: 3 }], '1.21')
    expect(levelOf(lower, 'sharpness')).toBe(5)
  })

  it('互斥附魔：1.12.2+ 牺牲附魔丢失', () => {
    const result = resultEnchantments('sword', [{ id: 'sharpness', level: 3 }], [{ id: 'smite', level: 2 }], '1.21')
    expect(levelOf(result, 'smite')).toBe(0)
    expect(levelOf(result, 'sharpness')).toBe(3)
  })

  it('结果等级不超过上限', () => {
    const result = resultEnchantments('sword', [{ id: 'sharpness', level: 5 }], [{ id: 'sharpness', level: 5 }], '1.21')
    expect(levelOf(result, 'sharpness')).toBe(5)
  })
})

describe('铁砧花费（Java 公式）', () => {
  it('wiki 示例：附魔花费 = 16（锋利 IV×1 + 抢夺 III×4）', () => {
    const cost = computeAnvilCost(
      'sword',
      {
        kind: 'item',
        useCount: 0,
        enchantments: [
          { id: 'sharpness', level: 3 },
          { id: 'knockback', level: 2 },
          { id: 'looting', level: 3 },
        ],
      },
      {
        kind: 'item',
        useCount: 0,
        enchantments: [
          { id: 'sharpness', level: 3 },
          { id: 'looting', level: 3 },
        ],
      },
      { version: '1.21' },
    )
    expect(cost.enchant).toBe(16)
    expect(cost.total).toBe(16)
    expect(cost.tooExpensive).toBe(false)
  })

  it('wiki 示例反向：附魔花费 = 20（含击退 II×2）', () => {
    const cost = computeAnvilCost(
      'sword',
      {
        kind: 'item',
        useCount: 0,
        enchantments: [
          { id: 'sharpness', level: 3 },
          { id: 'looting', level: 3 },
        ],
      },
      {
        kind: 'item',
        useCount: 0,
        enchantments: [
          { id: 'sharpness', level: 3 },
          { id: 'knockback', level: 2 },
          { id: 'looting', level: 3 },
        ],
      },
      { version: '1.21' },
    )
    expect(cost.enchant).toBe(20)
    expect(cost.total).toBe(20)
  })

  it('附魔书使用书的乘数', () => {
    const cost = computeAnvilCost(
      'sword',
      { kind: 'item', useCount: 0, enchantments: [] },
      { kind: 'book', useCount: 0, enchantments: [{ id: 'sharpness', level: 5 }] },
      { version: '1.21' },
    )
    expect(cost.enchant).toBe(5) // 5 × book multiplier 1
    expect(cost.repair).toBe(0) // 书无修复
    expect(cost.total).toBe(5)
  })

  it('先作惩罚、重命名与修复计入总花费', () => {
    const cost = computeAnvilCost(
      'pickaxe',
      { kind: 'item', useCount: 3, enchantments: [{ id: 'efficiency', level: 4 }] },
      { kind: 'item', useCount: 1, enchantments: [{ id: 'unbreaking', level: 3 }] },
      { rename: true, targetDamaged: true, version: '1.21' },
    )
    // 惩罚 7 + 1；重命名 1；修复 2；附魔：unbreaking III 目标无 → 3 × 2 = 6
    expect(cost.penaltyTarget).toBe(7)
    expect(cost.penaltySacrifice).toBe(1)
    expect(cost.rename).toBe(1)
    expect(cost.repair).toBe(2)
    expect(cost.enchant).toBe(6)
    expect(cost.total).toBe(17)
  })

  it('修复花费仅在两者均为物品且目标受损时计入', () => {
    const asBook = computeAnvilCost(
      'pickaxe',
      { kind: 'item', useCount: 0, enchantments: [] },
      { kind: 'book', useCount: 0, enchantments: [{ id: 'efficiency', level: 1 }] },
      { targetDamaged: true, version: '1.21' },
    )
    expect(asBook.repair).toBe(0)
  })

  it('花费 ≥ 40 时标记「过于昂贵」', () => {
    const cost = computeAnvilCost(
      'sword',
      { kind: 'item', useCount: 0, enchantments: [] },
      {
        kind: 'item',
        useCount: 0,
        enchantments: [
          { id: 'sharpness', level: 5 },
          { id: 'smite', level: 5 },
        ],
      },
      { version: '1.11' }, // 互斥按老版本，仍按公式给出花费
    )
    expect(cost.total).toBeGreaterThanOrEqual(0)
    const heavy = computeAnvilCost(
      'sword',
      { kind: 'item', useCount: 5, enchantments: [] },
      { kind: 'item', useCount: 5, enchantments: [{ id: 'sharpness', level: 5 }] },
      { rename: true, targetDamaged: true, version: '1.21' },
    )
    // 31 + 31 + 1 + 2 + 5 = 70
    expect(heavy.total).toBe(70)
    expect(heavy.tooExpensive).toBe(true)
  })

  it('不适用于装备的牺牲附魔被忽略', () => {
    const cost = computeAnvilCost(
      'boots',
      { kind: 'item', useCount: 0, enchantments: [] },
      { kind: 'book', useCount: 0, enchantments: [{ id: 'sharpness', level: 5 }] },
      { version: '1.21' },
    )
    expect(cost.enchant).toBe(0)
    expect(cost.total).toBe(0)
  })
})
