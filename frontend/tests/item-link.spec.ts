import { describe, expect, it } from 'vitest'
import { resolveIngredients, resolveRelatedItem, resolveResultItem } from '@/utils/item-link'
import type { Recipe } from '@/types/recipe'

const refs = {
  'minecraft:iron_ingot': {
    item_id: 'minecraft:iron_ingot',
    name_zh: '铁锭',
    name_en: 'Iron Ingot',
    item_category: 'material' as const,
    url: '/items/minecraft:iron_ingot',
  },
}

describe('resolveRelatedItem', () => {
  it('已收录物品给出中文名与详情页链接', () => {
    const got = resolveRelatedItem(refs, 'minecraft:iron_ingot')
    expect(got.label).toBe('铁锭')
    expect(got.url).toBe('/items/minecraft:iron_ingot')
  })

  it('未收录物品回退为纯文本 ID 且不生成链接（避免死链）', () => {
    const got = resolveRelatedItem(refs, 'minecraft:coal')
    expect(got.label).toBe('minecraft:coal')
    expect(got.url).toBeNull()
    expect(got.ref).toBeNull()
  })

  it('缺少 resolved_items 时不报错', () => {
    const got = resolveRelatedItem(undefined, 'minecraft:coal')
    expect(got.url).toBeNull()
  })

  it('空 item_id 不产生关联', () => {
    const got = resolveRelatedItem(refs, '')
    expect(got.ref).toBeNull()
    expect(got.url).toBeNull()
  })
})

describe('resolveResultItem / resolveIngredients', () => {
  const recipe: Recipe = {
    id: 'r1',
    recipe_key: 'minecraft:iron_ingot_from_smelting',
    recipe_type: 'smelting',
    game_version: '1.21',
    result_item_id: 'minecraft:iron_ingot',
    result_count: 1,
    pattern: [],
    key_mapping: {},
    ingredients: [
      { item_id: 'minecraft:raw_iron', count: 1 },
      { item_id: 'minecraft:iron_ingot', count: 2 },
    ],
    requires_crafting_table: false,
    source_url: null,
    data_version: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    resolved_items: refs,
  }

  it('结果物品解析为可跳转链接', () => {
    expect(resolveResultItem(recipe).url).toBe('/items/minecraft:iron_ingot')
  })

  it('材料保留数量与顺序，未收录材料只能纯文本展示', () => {
    const got = resolveIngredients(recipe)
    expect(got).toHaveLength(2)
    expect(got[0]).toMatchObject({ itemId: 'minecraft:raw_iron', url: null, count: 1 })
    expect(got[1]).toMatchObject({ itemId: 'minecraft:iron_ingot', url: '/items/minecraft:iron_ingot', count: 2 })
  })
})
