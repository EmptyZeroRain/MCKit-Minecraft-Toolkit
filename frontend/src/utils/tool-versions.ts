// ===== 版本特性注册表（TODO §2：版本不支持提示 + 统一版本交互）=====
//
// 设计目标：
// 1. 把「某功能自哪个版本起可用」集中登记，避免各工具页各写一套判断；
// 2. 版本无法裁决时（未填 / 快照 / 预览版）**不把理论结果说成确定结果**，
//    统一提示「按最新规则展示，可能不适用于旧版本，请自行核对」。
//
// 版本事实均为 Java 版正式版中广为人知的稳定结论；不确定的一律不登记。
import {
  compareSemVer,
  isSnapshotOrPreRelease,
  parseReleaseVersion,
  type SemVer,
} from './command-generator'

/** 版本敏感功能项 */
export interface VersionFeature {
  /** 稳定标识（供工具页引用） */
  key: string
  /** 中文名 */
  label: string
  /** 生效版本（含），格式 `X.Y` 或 `X.Y.Z` */
  since: string
  /** 移除版本（含移除），缺省表示至今仍有效 */
  until?: string
  /** 面向用户的说明（不夸大、不猜测） */
  note: string
}

/** 已登记的版本特性（按引入版本升序）。 */
export const VERSION_FEATURES: VersionFeature[] = [
  {
    key: 'elytra',
    label: '鞘翅',
    since: '1.9',
    note: '鞘翅随末地更新加入，更早版本无飞行装备。',
  },
  {
    key: 'shulker_box',
    label: '潜影盒',
    since: '1.11',
    note: '潜影盒随探索更新加入，更早版本无便携储物。',
  },
  {
    key: 'ocean',
    label: '海洋内容（海晶石 / 三叉戟 / 海龟）',
    since: '1.13',
    note: '海洋更新重写了水域生成与相关物品；1.13 前为旧海洋。',
  },
  {
    key: 'villages_pillage',
    label: '村庄与掠夺（村民交易体系 / 袭击）',
    since: '1.14',
    note: '村民职业、工作站与交易等级体系自 1.14 起定型。',
  },
  {
    key: 'bees',
    label: '蜜蜂与蜂巢',
    since: '1.15',
    note: '蜜蜂随嗡嗡蜂群更新加入。',
  },
  {
    key: 'nether_update',
    label: '下界更新（下界合金 / 猪灵 / 堡垒遗迹）',
    since: '1.16',
    note: '下界更新加入下界合金、猪灵与相关结构。',
  },
  {
    key: 'deepslate',
    label: '深板岩 / 铜 / 紫水晶 / 发光鱿鱼',
    since: '1.17',
    note: '洞穴与山崖第一部分加入深板岩层与铜、紫水晶。',
  },
  {
    key: 'candles',
    label: '蜡烛',
    since: '1.17',
    note: '蜡烛自 1.17 起可合成与交易。',
  },
  {
    key: 'dripstone',
    label: '滴水石与钟乳石',
    since: '1.17',
    note: '滴水石随溶洞生成加入。',
  },
  {
    key: 'effect_give',
    label: '/effect give|clear 子命令',
    since: '1.19',
    note: '1.19 起 /effect 改为 give|clear 子命令；更早版本使用旧语法。',
  },
  {
    key: 'mangrove',
    label: '红树林（红树）',
    since: '1.19',
    note: '红树随荒野更新加入。',
  },
  {
    key: 'cherry',
    label: '樱花木',
    since: '1.20',
    note: '樱花木随 1.20 更新加入。',
  },
  {
    key: 'data_components',
    label: '物品数据组件 item[组件]',
    since: '1.20.5',
    note: '1.20.5 起物品使用数据组件语法，取代旧式物品 NBT {…}。',
  },
  {
    key: 'armadillo',
    label: '犰狳与狼铠',
    since: '1.20.5',
    note: '犰狳、狼铠与相关盔甲纹饰随 1.20.5 加入。',
  },
  {
    key: 'trial_chambers',
    label: '试炼密室 / 旋风人 / 重锤',
    since: '1.21',
    note: '试炼密室、旋风人、沼骸与重锤随 1.21 加入。',
  },
  {
    key: 'crafter',
    label: '合成器',
    since: '1.21',
    note: '合成器随 1.21 加入。',
  },
  {
    key: 'pale_oak',
    label: '苍白橡木与嘎枝',
    since: '1.21.4',
    note: '苍白之园内容随 1.21.4 加入。',
  },
]

const FEATURE_INDEX = new Map(VERSION_FEATURES.map((f) => [f.key, f]))

/** 按 key 取功能项；不存在返回 undefined。 */
export function getVersionFeature(key: string): VersionFeature | undefined {
  return FEATURE_INDEX.get(key)
}

/** 支持级别：支持 / 不支持 / 无法裁决（未填、快照、预览版）。 */
export type SupportLevel = 'supported' | 'unsupported' | 'unknown'

/** 单个功能项在指定版本下的支持结论。 */
export interface FeatureSupport {
  feature: VersionFeature
  level: SupportLevel
  /** 面向用户的统一提示文案；`supported` 时为空串。 */
  message: string
}

/** 无法裁决版本时的统一文案（不把理论结果说成确定结果）。 */
export const UNKNOWN_VERSION_MESSAGE =
  '未指定或无法识别的版本（如快照 / 预览版）：以下内容按最新规则展示，可能不适用于旧版本，请自行核对。'

function compareTo(raw: string, target: string): number | null {
  const a = parseReleaseVersion(raw)
  const b = parseReleaseVersion(target)
  if (!a || !b) return null
  return compareSemVer(a, b)
}

/**
 * 判断某功能在指定版本下的支持情况。
 *
 * - 版本为空 / 快照 / 预览版 / 非法 → `unknown`，并给出统一提示；
 * - 版本早于 `since` → `unsupported`，明确提示「不支持」并说明理论值边界；
 * - 版本达到或晚于 `until` → `unsupported`（已移除）。
 */
export function checkFeatureSupport(key: string, version: string): FeatureSupport {
  const feature = getVersionFeature(key)
  if (!feature) {
    return {
      feature: { key, label: key, since: '', note: '' },
      level: 'unknown',
      message: UNKNOWN_VERSION_MESSAGE,
    }
  }

  const raw = (version ?? '').trim()
  if (raw === '' || isSnapshotOrPreRelease(raw) || !parseReleaseVersion(raw)) {
    return { feature, level: 'unknown', message: UNKNOWN_VERSION_MESSAGE }
  }

  const sinceCmp = compareTo(raw, feature.since)
  if (sinceCmp !== null && sinceCmp < 0) {
    return {
      feature,
      level: 'unsupported',
      message: `「${feature.label}」自 ${feature.since} 起可用，当前选择的 ${raw} 不支持该功能；若仍生成 / 计算，结果仅为理论值，不代表该版本的实际行为。`,
    }
  }

  if (feature.until) {
    const untilCmp = compareTo(raw, feature.until)
    if (untilCmp !== null && untilCmp >= 0) {
      return {
        feature,
        level: 'unsupported',
        message: `「${feature.label}」在 ${feature.until} 起已不再适用，当前选择的 ${raw} 无此功能。`,
      }
    }
  }

  return { feature, level: 'supported', message: '' }
}

/** 批量判断：返回所有非 supported 的结论（用于工具页一次性展示）。 */
export function checkFeaturesSupport(keys: string[], version: string): FeatureSupport[] {
  return keys
    .map((k) => checkFeatureSupport(k, version))
    .filter((s) => s.level !== 'supported')
}

/** 判断给定版本是否可用于确定性结论（未填 / 快照 / 预览版为 false）。 */
export function isDecidableVersion(version: string): boolean {
  const raw = (version ?? '').trim()
  if (raw === '' || isSnapshotOrPreRelease(raw)) return false
  return parseReleaseVersion(raw) !== null
}

/** 供测试与展示：解析后的语义化版本（无法解析返回 null）。 */
export function semverOf(version: string): SemVer | null {
  return parseReleaseVersion(version)
}
