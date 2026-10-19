// 可视化指令生成器单元测试（TODO §7：为不同指令类型编写生成结果测试）
import { describe, expect, it } from 'vitest'
import {
  COMPONENT_SINCE,
  EFFECT_GIVE_SINCE,
  buildEffect,
  buildGamemode,
  buildGive,
  buildSetblock,
  buildSummon,
  buildTp,
  compareSemVer,
  effectFormForVersion,
  formatPosition,
  isBalancedBraces,
  isValidPositionComponent,
  isValidTarget,
  jsonTextComponent,
  normalizeNamespacedId,
  parseReleaseVersion,
  syntaxForVersion,
  validatePosition,
  withSlash,
} from '@/utils/command-generator'

describe('版本裁决', () => {
  it('解析正式版版本号，快照 / 预览版返回 null', () => {
    expect(parseReleaseVersion('1.21')).toEqual({ major: 1, minor: 21, patch: 0 })
    expect(parseReleaseVersion('1.20.5')).toEqual({ major: 1, minor: 20, patch: 5 })
    expect(parseReleaseVersion('24w14a')).toBeNull()
    expect(parseReleaseVersion('1.21-pre1')).toBeNull()
    expect(parseReleaseVersion('')).toBeNull()
  })

  it('语义化版本比较', () => {
    expect(compareSemVer({ major: 1, minor: 20, patch: 5 }, COMPONENT_SINCE)).toBe(0)
    expect(compareSemVer({ major: 1, minor: 20, patch: 4 }, COMPONENT_SINCE)).toBeLessThan(0)
    expect(compareSemVer({ major: 1, minor: 21, patch: 0 }, COMPONENT_SINCE)).toBeGreaterThan(0)
  })

  it('1.20.5 起使用数据组件，更早使用 NBT', () => {
    expect(syntaxForVersion('1.21.4').syntax).toBe('components')
    expect(syntaxForVersion('1.20.5').syntax).toBe('components')
    expect(syntaxForVersion('1.20.4').syntax).toBe('nbt')
    expect(syntaxForVersion('1.16.5').syntax).toBe('nbt')
  })

  it('快照 / 无法识别的版本按最新语法生成并标记 assumed', () => {
    const snap = syntaxForVersion('24w14a')
    expect(snap.syntax).toBe('components')
    expect(snap.assumed).toBe(true)
    expect(snap.note).toBeTruthy()
    expect(syntaxForVersion('1.20.5').assumed).toBe(false)
  })

  it('1.19 起 /effect 使用 give 子命令', () => {
    expect(effectFormForVersion('1.19').form).toBe('give')
    expect(effectFormForVersion('1.18.2').form).toBe('legacy')
    expect(effectFormForVersion('1.19').assumed).toBe(false)
    expect(effectFormForVersion('1.21').form).toBe('give')
    expect(EFFECT_GIVE_SINCE.minor).toBe(19)
  })
})

describe('通用校验', () => {
  it('规范化命名空间 ID', () => {
    expect(normalizeNamespacedId('diamond_sword')).toBe('minecraft:diamond_sword')
    expect(normalizeNamespacedId('mod:custom_item')).toBe('mod:custom_item')
    expect(normalizeNamespacedId('Diamond_Sword')).toBe('minecraft:diamond_sword')
    expect(normalizeNamespacedId('')).toBeNull()
    expect(normalizeNamespacedId('bad!id')).toBeNull()
  })

  it('目标选择器 / 玩家名校验', () => {
    expect(isValidTarget('@p')).toBe(true)
    expect(isValidTarget('@a[team=red]')).toBe(true)
    expect(isValidTarget('Steve_01')).toBe(true)
    expect(isValidTarget('')).toBe(false)
    expect(isValidTarget('bad name')).toBe(false)
    expect(isValidTarget('@x')).toBe(false)
  })

  it('坐标分量支持绝对 / 相对 / 局部', () => {
    expect(isValidPositionComponent('100')).toBe(true)
    expect(isValidPositionComponent('-12.5')).toBe(true)
    expect(isValidPositionComponent('~')).toBe(true)
    expect(isValidPositionComponent('~5')).toBe(true)
    expect(isValidPositionComponent('~-3.5')).toBe(true)
    expect(isValidPositionComponent('^2')).toBe(true)
    expect(isValidPositionComponent('abc')).toBe(false)
    expect(isValidPositionComponent('')).toBe(false)
  })

  it('三元坐标校验区分空值与非法值', () => {
    expect(validatePosition({ x: '1', y: '~', z: '^2' })).toHaveLength(0)
    const empty = validatePosition({ x: '', y: '1', z: '1' })
    expect(empty[0].reason).toContain('不能为空')
    const bad = validatePosition({ x: 'x', y: '1', z: '1' })
    expect(bad[0].reason).toContain('非法')
  })

  it('花括号配平检查', () => {
    expect(isBalancedBraces('{a:1,b:[1,2]}')).toBe(true)
    expect(isBalancedBraces('{Name:\'"X"\'}')).toBe(true)
    expect(isBalancedBraces('{a:1')).toBe(false)
    expect(isBalancedBraces('}')).toBe(false)
  })

  it('JSON 文本组件渲染', () => {
    expect(jsonTextComponent('Excalibur')).toBe(`'"Excalibur"'`)
    expect(formatPosition({ x: '1', y: '~', z: '^2' })).toBe('1 ~ ^2')
    expect(withSlash('give @s stone 1')).toBe('/give @s stone 1')
  })
})

describe('/give 生成', () => {
  const base = { target: '@s', item: 'diamond_sword', count: '1', enchantments: [] }

  it('无修饰符的最简形式', () => {
    const r = buildGive(base, 'nbt')
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('give @s minecraft:diamond_sword 1')
  })

  it('旧式 NBT：附魔 / 不可破坏 / 自定义名称 / 损伤', () => {
    const r = buildGive(
      {
        ...base,
        enchantments: [{ id: 'sharpness', level: '5' }],
        unbreakable: true,
        customName: 'Excalibur',
        damage: '10',
      },
      'nbt',
    )
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.command).toBe(
        `give @s minecraft:diamond_sword {Enchantments:[{id:"sharpness",lvl:5}],Unbreakable:1b,display:{Name:'"Excalibur"'},Damage:10} 1`,
      )
      expect(r.notes.join()).toContain('NBT')
    }
  })

  it('新版数据组件', () => {
    const r = buildGive(
      {
        ...base,
        enchantments: [
          { id: 'sharpness', level: '5' },
          { id: 'mending', level: '1' },
        ],
        unbreakable: true,
        customName: 'Excalibur',
        damage: '10',
      },
      'components',
    )
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.command).toBe(
        `give @s minecraft:diamond_sword[minecraft:enchantments={levels:{"minecraft:sharpness":5,"minecraft:mending":1}},minecraft:unbreakable={},minecraft:custom_name='"Excalibur"',minecraft:damage=10] 1`,
      )
    }
  })

  it('非法输入逐项报错', () => {
    const r = buildGive(
      {
        target: 'bad name',
        item: 'bad!id',
        count: '0',
        enchantments: [{ id: 'nope', level: '1' }],
      },
      'nbt',
    )
    expect(r.ok).toBe(false)
    if (!r.ok) {
      const fields = r.issues.map((i) => i.field)
      expect(fields).toContain('target')
      expect(fields).toContain('item')
      expect(fields).toContain('count')
      expect(fields).toContain('enchant.0')
    }
  })

  it('附魔等级超出上限报错', () => {
    const r = buildGive({ ...base, enchantments: [{ id: 'mending', level: '3' }] }, 'nbt')
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.issues[0].reason).toContain('1-1')
  })

  it('空的附魔行被忽略而不报错', () => {
    const r = buildGive({ ...base, enchantments: [{ id: '', level: '' }] }, 'nbt')
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('give @s minecraft:diamond_sword 1')
  })
})

describe('/summon 生成', () => {
  it('实体数据为 NBT，支持相对坐标', () => {
    const r = buildSummon({
      entity: 'zombie',
      pos: { x: '~', y: '~1', z: '~' },
      customName: 'Test',
      noAI: true,
    })
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.command).toBe(`summon minecraft:zombie ~ ~1 ~ {CustomName:'"Test"',NoAI:1b}`)
    }
  })

  it('非法的实体 ID 与坐标报错', () => {
    const r = buildSummon({ entity: 'bad!id', pos: { x: 'a', y: '1', z: '1' } })
    expect(r.ok).toBe(false)
    if (!r.ok) {
      expect(r.issues.map((i) => i.field)).toContain('entity')
      expect(r.issues.map((i) => i.field)).toContain('pos')
    }
  })
})

describe('/effect 生成', () => {
  it('1.19+ 使用 give 子命令', () => {
    const r = buildEffect(
      { target: '@a', effect: 'speed', duration: '30', amplifier: '1', hideParticles: true },
      '1.21',
    )
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('effect give @a minecraft:speed 30 1 true')
  })

  it('1.19 之前使用旧语法', () => {
    const r = buildEffect({ target: '@p', effect: 'speed', duration: '30', amplifier: '0' }, '1.18.2')
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('effect @p minecraft:speed 30 0')
  })

  it('持续时间为 0 时校验失败', () => {
    const r = buildEffect({ target: '@p', effect: 'speed', duration: '0', amplifier: '0' }, '1.21')
    expect(r.ok).toBe(false)
  })
})

describe('/setblock 生成', () => {
  it('含方块状态与处理模式', () => {
    const r = buildSetblock({
      pos: { x: '10', y: '64', z: '10' },
      block: 'oak_planks',
      states: [{ key: 'waterlogged', value: 'true' }],
      mode: 'destroy',
    })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('setblock 10 64 10 minecraft:oak_planks[waterlogged=true] destroy')
  })

  it('replace 模式省略（默认）', () => {
    const r = buildSetblock({ pos: { x: '1', y: '2', z: '3' }, block: 'stone', mode: 'replace' })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('setblock 1 2 3 minecraft:stone')
  })

  it('非法的方块状态名报错', () => {
    const r = buildSetblock({
      pos: { x: '1', y: '2', z: '3' },
      block: 'stone',
      states: [{ key: 'Bad Key', value: 'true' }],
    })
    expect(r.ok).toBe(false)
  })
})

describe('/tp 生成', () => {
  it('坐标模式可附带朝向', () => {
    const r = buildTp({
      target: '@p',
      mode: 'coords',
      pos: { x: '100', y: '64', z: '-200' },
      rotation: { yaw: '90', pitch: '0' },
    })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('tp @p 100 64 -200 90 0')
  })

  it('实体模式校验目标实体', () => {
    const r = buildTp({
      target: '@s',
      mode: 'entity',
      pos: { x: '0', y: '0', z: '0' },
      destination: '@e[type=zombie,limit=1]',
    })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('tp @s @e[type=zombie,limit=1]')
  })

  it('朝向非数值时报错', () => {
    const r = buildTp({
      target: '@p',
      mode: 'coords',
      pos: { x: '1', y: '2', z: '3' },
      rotation: { yaw: 'east', pitch: '0' },
    })
    expect(r.ok).toBe(false)
  })
})

describe('/gamemode 生成', () => {
  it('带目标与不带目标', () => {
    expect(buildGamemode({ mode: 'creative', target: 'Steve' })).toEqual({
      ok: true,
      command: 'gamemode creative Steve',
      notes: [],
    })
    const noTarget = buildGamemode({ mode: 'survival' })
    expect(noTarget.ok).toBe(true)
    if (noTarget.ok) expect(noTarget.command).toBe('gamemode survival')
  })
})

// ===== 回归（TODO §14）：`<input type="number">` 的 v-model 会得到 number =====
// 修复前 `(opts.count ?? '').trim()` 在数字上抛 `trim is not a function`，
// 也就是用户在页面上输入数量就会让生成器崩溃。此处锁定「数字输入同样可用」。
describe('数字输入兼容（number 形式的表单值）', () => {
  it('buildGive 接受 number 类型的 count / damage / 附魔等级', () => {
    const r = buildGive(
      {
        target: '@s',
        item: 'diamond_sword',
        count: 1,
        enchantments: [{ id: 'sharpness', level: 5 }],
        damage: 10,
      },
      'nbt',
    )
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.command).toContain('diamond_sword')
      expect(r.command).toContain('Damage:10')
      expect(r.command).toContain('sharpness')
    }
  })

  it('buildGive 仍拒绝越界的 number 数量', () => {
    const r = buildGive({ target: '@s', item: 'stone', count: 0, enchantments: [] }, 'nbt')
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.issues[0].field).toBe('count')
  })

  it('buildEffect 接受 number 类型的 duration / amplifier', () => {
    const r = buildEffect({ target: '@s', effect: 'speed', duration: 30, amplifier: 1 }, 'nbt')
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.command).toBe('effect give @s minecraft:speed 30 1')
  })

  it('validatePosition / formatPosition 接受 number 分量', () => {
    expect(validatePosition({ x: 1, y: 2, z: -3 }, 'pos')).toEqual([])
    expect(formatPosition({ x: 1, y: 2, z: -3 })).toBe('1 2 -3')
  })
})
