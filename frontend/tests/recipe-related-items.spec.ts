// 配方 → 物品关联渲染测试（TODO §3）
// 关键约束：只有后端确认已收录的物品才生成详情页链接；未收录物品必须是纯文本。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/api/public', () => ({ listRecipes: vi.fn() }))

import { listRecipes } from '@/api/public'
import RecipesView from '@/views/RecipesView.vue'
import type { Recipe } from '@/types/recipe'

const RECIPE: Recipe = {
  id: 'r1',
  recipe_key: 'minecraft:iron_ingot_from_blasting',
  recipe_type: 'smelting',
  game_version: '1.21',
  result_item_id: 'minecraft:iron_ingot',
  result_count: 1,
  pattern: [],
  key_mapping: {},
  ingredients: [
    { item_id: 'minecraft:raw_iron', count: 1 },
    { item_id: 'minecraft:coal', count: 1 },
  ],
  requires_crafting_table: false,
  source_url: null,
  data_version: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  resolved_items: {
    'minecraft:iron_ingot': {
      item_id: 'minecraft:iron_ingot',
      name_zh: '铁锭',
      name_en: 'Iron Ingot',
      item_category: 'material',
      url: '/items/minecraft:iron_ingot',
    },
    'minecraft:raw_iron': {
      item_id: 'minecraft:raw_iron',
      name_zh: '粗铁',
      name_en: 'Raw Iron',
      item_category: 'material',
      url: '/items/minecraft:raw_iron',
    },
    // minecraft:coal 未收录：不在映射中
  },
}

async function mountView(recipes: Recipe[]) {
  setActivePinia(createPinia())
  ;(listRecipes as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: recipes,
    total: recipes.length,
    page: 1,
    pageSize: 15,
    totalPages: 1,
    suggestions: [],
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/recipes', name: 'recipes', component: { template: '<div />' } },
      { path: '/items/:itemId', name: 'item-detail', component: { template: '<div />' } },
    ],
  })
  await router.push('/recipes')
  await router.isReady()
  const wrapper = mount(RecipesView, {
    global: { plugins: [router], stubs: { Pagination: true } },
  })
  await flushPromises()
  return wrapper
}

describe('配方卡片的关联物品', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('已收录的结果物品与材料渲染为详情页链接', async () => {
    const wrapper = await mountView([RECIPE])
    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href'))
    expect(hrefs).toContain('/items/minecraft:iron_ingot')
    expect(hrefs).toContain('/items/minecraft:raw_iron')
    expect(wrapper.text()).toContain('铁锭')
    expect(wrapper.text()).toContain('粗铁')
  })

  it('未收录的 ItemRef 不在映射中，材料回退为纯文本且不产生链接', async () => {
    const wrapper = await mountView([RECIPE])
    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href'))
    expect(hrefs).not.toContain('/items/minecraft:coal')
    // 未收录材料仍以原始 ID 展示（不隐藏、不谎报名称）
    expect(wrapper.text()).toContain('minecraft:coal')
  })

  it('缺少 resolved_items 时不报错且不生成物品链接', async () => {
    const wrapper = await mountView([{ ...RECIPE, resolved_items: undefined }])
    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href'))
    expect(hrefs.filter((h) => h && h.startsWith('/items/'))).toHaveLength(0)
    expect(wrapper.text()).toContain('minecraft:iron_ingot')
  })
})
