// 物品详情页测试（统一物品资料库，TODO §1）
// 覆盖两层：展示辅助纯函数 + 详情视图（关联配方 / 获取指令 / 类别属性 / 空态）。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import type { Router } from 'vue-router'
import {
  buildItemSearchLinks,
  itemCategoryLabel,
  relatedRecipeCount,
  supportsDurability,
} from '@/utils/item-category'
import type { ItemDetail } from '@/types/item'
import type { Recipe } from '@/types/recipe'

function makeRecipe(overrides: Partial<Recipe> = {}): Recipe {
  return {
    id: 'r1',
    recipe_key: 'minecraft:stick',
    recipe_type: 'shaped',
    game_version: '1.21',
    result_item_id: 'minecraft:stick',
    result_count: 4,
    pattern: ['#', '#'],
    key_mapping: { '#': 'minecraft:oak_planks' },
    ingredients: [{ item_id: 'minecraft:oak_planks', count: 2 }],
    requires_crafting_table: true,
    source_url: null,
    data_version: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const DETAIL: ItemDetail = {
  id: 'i1',
  item_id: 'minecraft:stick',
  name_zh: '木棍',
  name_en: 'Stick',
  aliases: ['棍子'],
  description: '基础工具原料。',
  item_category: 'material',
  stack_size: 64,
  durability: null,
  obtain_method: '由 2 个木板合成。',
  usage: '合成工具与武器。',
  image_url: null,
  game_edition: 'java',
  supported_versions: ['1.21'],
  source_url: 'https://minecraft.wiki/w/Stick',
  data_version: 'demo-2026-01',
  last_verified_at: '2026-01-15T00:00:00Z',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  related_recipes: {
    produced: [makeRecipe()],
    used_as: [makeRecipe({ id: 'r2', result_item_id: 'minecraft:diamond_pickaxe', recipe_type: 'shaped' })],
  },
  obtain_command: '/give @s minecraft:stick 1',
}

describe('物品展示辅助函数', () => {
  it('类别标签映射，未知类别回退为「其他」', () => {
    expect(itemCategoryLabel('tool')).toBe('工具')
    expect(itemCategoryLabel('block')).toBe('方块')
    expect(itemCategoryLabel('unknown')).toBe('其他')
    expect(itemCategoryLabel(null)).toBe('其他')
  })

  it('仅工具 / 武器 / 防具适用耐久度', () => {
    expect(supportsDurability('tool')).toBe(true)
    expect(supportsDurability('weapon')).toBe(true)
    expect(supportsDurability('armor')).toBe(true)
    expect(supportsDurability('block')).toBe(false)
    expect(supportsDurability(null)).toBe(false)
  })

  it('跨模块检索入口按名称编码，不伪造关联', () => {
    const links = buildItemSearchLinks({ name_zh: '木棍', item_id: 'minecraft:stick' })
    expect(links.wiki).toBe('/search?q=%E6%9C%A8%E6%A3%8D&type=wiki')
    expect(links.resource).toBe('/search?q=%E6%9C%A8%E6%A3%8D&type=resource')
    // 指令不在全站搜索覆盖范围内，必须指向指令页自身检索
    expect(links.command).toBe('/commands?q=%E6%9C%A8%E6%A3%8D')
  })

  it('关联配方计数为产出 + 作为材料之和', () => {
    expect(relatedRecipeCount(DETAIL)).toBe(2)
    expect(relatedRecipeCount(null)).toBe(0)
  })
})

// ===== 视图交互 =====
vi.mock('@/api/public', () => ({
  getItem: vi.fn(),
}))

import { getItem } from '@/api/public'
import ItemDetailView from '@/views/ItemDetailView.vue'

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/items', name: 'items', component: { template: '<div />' } },
      { path: '/items/:itemId', name: 'item-detail', component: { template: '<div />' } },
      { path: '/recipes', name: 'recipes', component: { template: '<div />' } },
      { path: '/commands', name: 'commands', component: { template: '<div />' } },
      { path: '/search', name: 'search', component: { template: '<div />' } },
    ],
  })
}

async function mountDetail(itemId = 'minecraft:stick', detail: ItemDetail | null = DETAIL) {
  setActivePinia(createPinia())
  const router = makeRouter()
  router.push(`/items/${itemId}`)
  await router.isReady()

  if (detail) {
    ;(getItem as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
  } else {
    const err = new Error('未找到')
    ;(getItem as ReturnType<typeof vi.fn>).mockRejectedValue(err)
  }

  const wrapper = mount(ItemDetailView, {
    global: { plugins: [router] },
  })
  await flushPromises()
  return wrapper
}

describe('物品详情页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('按 item_id 请求详情并渲染名称与 ID', async () => {
    const wrapper = await mountDetail()
    expect(getItem).toHaveBeenCalledWith('minecraft:stick')
    expect(wrapper.text()).toContain('木棍')
    expect(wrapper.text()).toContain('minecraft:stick')
    expect(wrapper.text()).toContain('材料')
  })

  it('渲染获取方式、用途与获取指令', async () => {
    const wrapper = await mountDetail()
    expect(wrapper.text()).toContain('由 2 个木板合成。')
    expect(wrapper.text()).toContain('合成工具与武器。')
    expect(wrapper.text()).toContain('/give @s minecraft:stick 1')
  })

  it('展示数据驱动的关联配方：产出与作为材料', async () => {
    const wrapper = await mountDetail()
    expect(wrapper.text()).toContain('产出该物品的配方')
    expect(wrapper.text()).toContain('作为材料的配方')
    expect(wrapper.text()).toContain('minecraft:diamond_pickaxe'.split(':')[1])
  })

  it('材料类物品不展示耐久度属性', async () => {
    const wrapper = await mountDetail()
    expect(wrapper.text()).not.toContain('耐久度')
  })

  it('工具类物品展示耐久度', async () => {
    const wrapper = await mountDetail('minecraft:diamond_pickaxe', {
      ...DETAIL,
      item_id: 'minecraft:diamond_pickaxe',
      name_zh: '钻石镐',
      item_category: 'tool',
      durability: 1561,
    })
    expect(wrapper.text()).toContain('耐久度')
    expect(wrapper.text()).toContain('1561')
  })

  it('无关联配方时展示明确提示而非伪造', async () => {
    const wrapper = await mountDetail('minecraft:stone', {
      ...DETAIL,
      item_id: 'minecraft:stone',
      name_zh: '石头',
      item_category: 'block',
      related_recipes: { produced: [], used_as: [] },
    })
    expect(wrapper.text()).toContain('暂未收录与该物品相关的配方')
  })
})
