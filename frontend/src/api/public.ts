// ===== 公开 API（需求 8.2，全 POST + JSON body）=====
// 说明：按项目契约，所有接口统一使用 POST，查询参数放在请求体。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { Tool, ToolListQuery } from '@/types/tool'
import type { Resource, ResourceDetail, ResourceListQuery } from '@/types/resource'
import type { Item, ItemDetail, ItemQuery } from '@/types/item'
import type { Potion, PotionDetail, PotionQuery, BrewingRules } from '@/types/potion'
import type {
  VillagerProfession,
  VillagerProfessionDetail,
  VillagerTradeLevel,
  VillagerQuery,
} from '@/types/villager'
import type { Mob, MobDetail, MobQuery } from '@/types/mob'
import type {
  Mod,
  ModDetail,
  ModQuery,
  Modpack,
  ModpackDetail,
  ModpackQuery,
} from '@/types/mods'
import type {
  RedstoneComponent,
  RedstoneComponentQuery,
  RedstoneDesign,
  RedstoneDesignDetail,
  RedstoneDesignQuery,
} from '@/types/redstone'
import type { Recipe, RecipeQuery } from '@/types/recipe'
import type { Command, CommandQuery } from '@/types/command'
import type { WikiEntry } from '@/types/wiki'
import type { Category, CategoryQuery } from '@/types/category'
import type { FeedbackSubmitPayload } from '@/types/feedback'
import type { SearchQuery, SearchResultItem } from '@/types/search'

/** 健康检查 */
export async function health(): Promise<{ status: string }> {
  const { data } = await http.post<ApiResponse<{ status: string }>>('/health', {})
  return data.data
}

/** 全站搜索 */
export async function search(params: SearchQuery) {
  const { data } = await http.post<ApiResponse<SearchResultItem[]>>('/search', params)
  return toPaged({ data })
}

/** 工具列表 */
export async function listTools(params: ToolListQuery = {}) {
  const { data } = await http.post<ApiResponse<Tool[]>>('/tools', params)
  return toPaged({ data })
}

/** 工具详情（slug） */
export async function getTool(slug: string): Promise<Tool> {
  const { data } = await http.post<ApiResponse<Tool>>(`/tools/${encodeURIComponent(slug)}`, {})
  return data.data
}

/** 资源列表 */
export async function listResources(params: ResourceListQuery = {}) {
  const { data } = await http.post<ApiResponse<Resource[]>>('/resources', params)
  return toPaged({ data })
}

/** 资源详情（slug） */
export async function getResource(slug: string): Promise<ResourceDetail> {
  const { data } = await http.post<ApiResponse<ResourceDetail>>(`/resources/${encodeURIComponent(slug)}`, {})
  return data.data
}

/** 物品查询 */
export async function listItems(params: ItemQuery = {}) {
  const { data } = await http.post<ApiResponse<Item[]>>('/items', params)
  return toPaged({ data })
}

/** 物品详情（item_id 含冒号，必须正确编码；返回物品 + 关联配方 + 获取指令） */
export async function getItem(itemId: string): Promise<ItemDetail> {
  const { data } = await http.post<ApiResponse<ItemDetail>>(`/items/${encodeURIComponent(itemId)}`, {})
  return data.data
}

/** 配方查询 */
export async function listRecipes(params: RecipeQuery = {}) {
  const { data } = await http.post<ApiResponse<Recipe[]>>('/recipes', params)
  return toPaged({ data })
}

/** 药水查询（TODO §9A） */
export async function listPotions(params: PotionQuery = {}) {
  const { data } = await http.post<ApiResponse<Potion[]>>('/potions', params)
  return toPaged({ data })
}

/** 通用酿造规则（红石延长 / 萤石增强 / 火药喷溅 / 龙息滞留，TODO §9A） */
export async function getBrewingRules(): Promise<BrewingRules> {
  const { data } = await http.post<ApiResponse<BrewingRules>>('/potions/rules', {})
  return data.data
}

/** 药水详情（slug；返回药水 + 酿造步骤 + 关联物品，TODO §9A） */
export async function getPotion(slug: string): Promise<PotionDetail> {
  const { data } = await http.post<ApiResponse<PotionDetail>>(`/potions/${encodeURIComponent(slug)}`, {})
  return data.data
}

/** 村民职业查询（TODO §9B） */
export async function listVillagers(params: VillagerQuery = {}) {
  const { data } = await http.post<ApiResponse<VillagerProfession[]>>('/villagers', params)
  return toPaged({ data })
}

/** 职业等级通用规则（新手 → 大师，TODO §9B） */
export async function getVillagerTradeLevels(): Promise<VillagerTradeLevel[]> {
  const { data } = await http.post<ApiResponse<VillagerTradeLevel[]>>('/villagers/levels', {})
  return data.data
}

/** 村民职业详情（slug；返回职业 + 等级规则 + 分组交易 + 关联物品，TODO §9B） */
export async function getVillager(slug: string): Promise<VillagerProfessionDetail> {
  const { data } = await http.post<ApiResponse<VillagerProfessionDetail>>(
    `/villagers/${encodeURIComponent(slug)}`,
    {},
  )
  return data.data
}

/** 生物列表与筛选（TODO §9C） */
export async function listMobs(params: MobQuery = {}) {
  const { data } = await http.post<ApiResponse<Mob[]>>('/mobs', params)
  return toPaged({ data })
}

/** 生物详情（slug；返回生物 + 掉落 + 关联物品，TODO §9C） */
export async function getMob(slug: string): Promise<MobDetail> {
  const { data } = await http.post<ApiResponse<MobDetail>>(
    `/mobs/${encodeURIComponent(slug)}`,
    {},
  )
  return data.data
}

/** 红石元件列表与筛选（TODO §15） */
export async function listRedstoneComponents(params: RedstoneComponentQuery = {}) {
  const { data } = await http.post<ApiResponse<RedstoneComponent[]>>('/redstone/components', params)
  return toPaged({ data })
}

/** 红石元件详情（slug；TODO §15） */
export async function getRedstoneComponent(slug: string): Promise<RedstoneComponent> {
  const { data } = await http.post<ApiResponse<RedstoneComponent>>(
    `/redstone/components/${encodeURIComponent(slug)}`,
    {},
  )
  return data.data
}

/** 机器设计列表与筛选（仅已发布，TODO §15） */
export async function listRedstoneDesigns(params: RedstoneDesignQuery = {}) {
  const { data } = await http.post<ApiResponse<RedstoneDesign[]>>('/redstone/designs', params)
  return toPaged({ data })
}

/** 机器设计详情（slug；返回设计 + 材料 + 关联物品，仅已发布，TODO §15） */
export async function getRedstoneDesign(slug: string): Promise<RedstoneDesignDetail> {
  const { data } = await http.post<ApiResponse<RedstoneDesignDetail>>(
    `/redstone/designs/${encodeURIComponent(slug)}`,
    {},
  )
  return data.data
}

/** Mod 列表与筛选（仅已发布，TODO §16） */
export async function listMods(params: ModQuery = {}) {
  const { data } = await http.post<ApiResponse<Mod[]>>('/mods', params)
  return toPaged({ data })
}

/** Mod 详情（slug；返回本体 + 发布版本 + 依赖与冲突 + 反向依赖，仅已发布，TODO §16） */
export async function getMod(slug: string): Promise<ModDetail> {
  const { data } = await http.post<ApiResponse<ModDetail>>(
    `/mods/${encodeURIComponent(slug)}`,
    {},
  )
  return data.data
}

/** 整合包列表与筛选（仅已发布，TODO §16） */
export async function listModpacks(params: ModpackQuery = {}) {
  const { data } = await http.post<ApiResponse<Modpack[]>>('/modpacks', params)
  return toPaged({ data })
}

/** 整合包详情（slug；返回本体 + 包内 Mod 清单，仅已发布，TODO §16） */
export async function getModpack(slug: string): Promise<ModpackDetail> {
  const { data } = await http.post<ApiResponse<ModpackDetail>>(
    `/modpacks/${encodeURIComponent(slug)}`,
    {},
  )
  return data.data
}

/** 指令列表与搜索 */
export async function listCommands(params: CommandQuery = {}) {
  const { data } = await http.post<ApiResponse<Command[]>>('/commands', params)
  return toPaged({ data })
}

/** 指令详情（slug）——组合在公开查询中使用 */
export async function getCommand(slug: string): Promise<Command> {
  const { data } = await http.post<ApiResponse<Command>>(`/commands/${encodeURIComponent(slug)}`, {})
  return data.data
}

/** 百科详情（slug） */
export async function getWiki(slug: string): Promise<WikiEntry> {
  const { data } = await http.post<ApiResponse<WikiEntry>>(`/wiki/${encodeURIComponent(slug)}`, {})
  return data.data
}

/** 分类列表 */
export async function listCategories(params: CategoryQuery = {}) {
  const { data } = await http.post<ApiResponse<Category[]>>('/categories', params)
  return data.data
}

/** 提交反馈 / 举报 */
export async function submitFeedback(payload: FeedbackSubmitPayload) {
  const { data } = await http.post<ApiResponse<{ id: string }>>('/feedback', payload)
  return data.data
}