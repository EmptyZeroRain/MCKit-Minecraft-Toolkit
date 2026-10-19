// ===== 内置工具清单：前端自带页面，单一事实源 =====
// 这些工具由前端直接实现（不依赖后台工具表），因此有专属落地页。
// 为避免同一功能出现两套界面，工具中心里这些 slug 统一重定向到专用页
// （见 router/index.ts 的 beforeEnter 守卫）。
// 本清单同时作为：首页快捷入口、工具中心「内置工具」区、页脚链接的唯一来源。
import type { ToolCategory } from '@/types/common'

export interface BuiltinTool {
  slug: string
  name: string
  summary: string
  category: ToolCategory
  icon: string
  /** 专属落地路径 */
  route: string
}

export const BUILTIN_TOOLS: BuiltinTool[] = [
  {
    slug: 'recipes',
    name: '合成配方',
    summary: '查询物品的合成方法与所需材料',
    category: '查询类',
    icon: '🧪',
    route: '/recipes',
  },
  {
    slug: 'coordinate',
    name: '坐标换算',
    summary: '主世界与下界坐标互转',
    category: '计算类',
    icon: '🧭',
    route: '/coordinate',
  },
  {
    slug: 'items',
    name: '物品 ID',
    summary: '查找物品的标准 namespaced ID',
    category: '查询类',
    icon: '📦',
    route: '/items',
  },
  {
    slug: 'potions',
    name: '药水与酿造',
    summary: '查询药水效果、持续时间与酿造步骤',
    category: '查询类',
    icon: '🧴',
    route: '/potions',
  },
  {
    slug: 'villagers',
    name: '村民交易',
    summary: '查询村民职业、交易等级与交易表',
    category: '查询类',
    icon: '🧑‍🌾',
    route: '/villagers',
  },
  {
    slug: 'mobs',
    name: '生物资料',
    summary: '查询生物掉落、生成条件与驯服繁殖方式',
    category: '查询类',
    icon: '🐄',
    route: '/mobs',
  },
  {
    slug: 'redstone',
    name: '红石资料',
    summary: '红石元件机制与机器设计资料',
    category: '查询类',
    icon: '🔴',
    route: '/redstone',
  },
  {
    slug: 'mods',
    name: 'Mod 兼容性',
    summary: '查询 Mod 的加载器、版本、前置依赖与整合包清单',
    category: '查询类',
    icon: '🧩',
    route: '/mods',
  },
  {
    slug: 'commands',
    name: '指令辅助',
    summary: '命令模板、参数说明与示例',
    category: '辅助类',
    icon: '⌨️',
    route: '/commands',
  },
  {
    slug: 'command-generator',
    name: '指令生成器',
    summary: '表单化生成 /give、/summon、/effect 等指令',
    category: '生成类',
    icon: '🛠️',
    route: '/command-generator',
  },
  {
    slug: 'enchant-calculator',
    name: '附魔计算器',
    summary: '校验附魔组合并估算铁砧合并经验花费',
    category: '计算类',
    icon: '⚗️',
    route: '/enchant-calculator',
  },
  {
    slug: 'versions',
    name: '版本信息',
    summary: '各版本下载地址、启动命令与 Java 要求',
    category: '查询类',
    icon: '📜',
    route: '/versions',
  },
  {
    slug: 'block-ids',
    name: 'ID 对照表',
    summary: '方块与物品的中文名 / ID 对照',
    category: '查询类',
    icon: '🧱',
    route: '/block-ids',
  },
  {
    slug: 'seed-map',
    name: '种子地图',
    summary: '种子分析：史莱姆区块与结构候选坐标',
    category: '查询类',
    icon: '🗺️',
    route: '/seed-map',
  },
  {
    slug: 'seed-tools',
    name: '种子与结构工具',
    summary: '结构坐标、要塞估算、种子生成与搜索',
    category: '计算类',
    icon: '🎲',
    route: '/seed-tools',
  },
  {
    slug: 'dev-docs',
    name: '开发文档',
    summary: 'NeoForge / Forge / Fabric / 原版数据开发知识库',
    category: '查询类',
    icon: '🛠️',
    route: '/dev',
  },
  {
    slug: 'mappings',
    name: '版本映射查询',
    summary: '查询类、方法与字段在各套映射中的名字对照',
    category: '查询类',
    icon: '🔤',
    route: '/mappings',
  },
  {
    slug: 'mcsrc',
    name: 'MC 源码浏览器',
    summary: '按版本浏览官方 jar 的类索引与检索类名',
    category: '查询类',
    icon: '📚',
    route: '/mcsrc',
  },
  {
    slug: 'region-inspect',
    name: '存档诊断',
    summary: '解析 level.dat 与 .mca，输出世界信息与只读诊断',
    category: '辅助类',
    icon: '📂',
    route: '/region-inspect',
  },
]

export const BUILTIN_TOOL_ROUTES: Record<string, string> = Object.fromEntries(
  BUILTIN_TOOLS.map((t) => [t.slug, t.route]),
)

export const BUILTIN_TOOL_SLUGS = BUILTIN_TOOLS.map((t) => t.slug)

/** 判断 slug 是否为内置工具，返回其专用落地路径（非内置返回 null） */
export function builtinToolRoute(slug: string): string | null {
  return BUILTIN_TOOL_ROUTES[slug] ?? null
}
