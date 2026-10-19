// ===== MC 源码库（mcsrc）类型 =====

/** 侧别：客户端 / 服务端 jar */
export const MCSRC_SIDES = ['client', 'server'] as const
export type McsrcSide = (typeof MCSRC_SIDES)[number]

export const MCSRC_SIDE_LABELS: Record<McsrcSide, string> = {
  client: '客户端',
  server: '服务端',
}

/** 单个版本（含侧别）的准备状态 */
export interface McsrcStatus {
  version: string
  side: McsrcSide
  /** 是否已缓存 jar */
  cached: boolean
  /** jar 字节数 */
  size: number
  sha1: string
  fetched_at?: string
  /** 是否已建立类索引 */
  indexed: boolean
  class_count: number
  /** 索引是否因超出上限被截断 */
  truncated: boolean
  index_built_at?: string
  /** 是否正在准备（下载 jar / 建索引），用于渲染进度条 */
  preparing?: boolean
  /** 准备阶段：downloading（可算百分比）/ indexing（不确定进度） */
  phase?: 'downloading' | 'indexing' | ''
  /** 已下载字节数 */
  downloaded?: number
  /** 预期 jar 字节数（0 表示未知） */
  total?: number
}

/** 类索引条目 */
export interface McsrcClass {
  /** 全限定名，如 net.minecraft.server.MinecraftServer */
  name: string
  /** 简单名，如 MinecraftServer */
  simple: string
  /** 包名，如 net.minecraft.server */
  pkg: string
  /** 是否为内部类（简单名含 $） */
  inner: boolean
  /** class 文件字节数 */
  size: number
}

/** 类检索结果（分页） */
export interface McsrcClassPage {
  items: McsrcClass[]
  total: number
  page: number
  page_size: number
}

/** 类种类 */
export type McsrcClassKind = 'class' | 'interface' | 'enum' | 'annotation'

export const MCSRC_KIND_LABELS: Record<McsrcClassKind, string> = {
  class: '类',
  interface: '接口',
  enum: '枚举',
  annotation: '注解',
}

/** 类成员（字段 / 方法），描述符已由后端渲染为 Java 类型 */
export interface McsrcMember {
  name: string
  /** Java 修饰符（public / static / final …） */
  modifiers: string[]
  /** 字段类型或方法返回类型 */
  type: string
  /** 方法参数类型；字段无此字段 */
  params?: string[]
  /** 原始 JVM 描述符，便于对照 */
  descriptor: string
  /** 是否为编译器生成的成员（bridge / synthetic） */
  synthetic: boolean
}

/** 类结构视图（纯字节码解析，无需反编译） */
export interface McsrcClassStructure {
  name: string
  simple: string
  pkg: string
  kind: McsrcClassKind
  modifiers: string[]
  /** class 文件主版本号（52 = Java 8） */
  major: number
  minor: number
  super: string
  /** 父类是否也在本 jar 内（false 表示 JDK / 第三方库类） */
  in_jar_super: boolean
  interfaces: string[]
  /** 直接接口中位于本 jar 内的部分（可继续下钻） */
  in_jar_interfaces: string[]
  fields: McsrcMember[]
  methods: McsrcMember[]
  /** 是否为内部类 */
  inner: boolean
}

/** 子类条目 */
export interface McsrcDescendant {
  name: string
  simple: string
  inner: boolean
}

/** 继承关系：向上祖先链 + 向下直接子类 */
export interface McsrcHierarchy {
  name: string
  super: string
  interfaces: string[]
  /** 祖先链：直接父类起逐级向上，含跳出本 jar 的终点 */
  ancestors: string[]
  /** 祖先链是否全程在本 jar 内 */
  ancestors_in_jar: boolean
  /** 直接子类（含实现本接口的类） */
  descendants: McsrcDescendant[]
  /** 直接子类总数（可能大于 descendants.length） */
  descendant_count: number
  /** 递归子类总数（含间接） */
  total_descendants: number
  /** 直接子类列表是否被截断 */
  truncated: boolean
}

/** class 文件主版本号 → Java 版本名（45 = 1.1，52 = 8） */
export function javaVersionLabel(major: number): string {
  if (!major || major < 45) return '未知'
  const n = major - 44
  return n <= 4 ? `Java 1.${n}` : `Java ${n}`
}

/** 渲染方法的可读签名：返回类型 + 名称(参数) */
export function memberSignature(m: McsrcMember): string {
  const params = (m.params ?? []).join(', ')
  return `${m.type || '?'} ${m.name}(${params})`
}

/** 反编译请求（主线程 → Worker） */
export interface McsrcDecompileRequest {
  /** 点号全限定类名，如 net.minecraft.server.MinecraftServer */
  className: string
  /** jar 缓存键（<version>-<side>） */
  jarName: string
  /** jar 字节（浏览器内用 @katana-project/zip 解析） */
  jar: Blob
}

/** 反编译源码中的 token 类型（对应 Vineflower TokenCollector 的回调） */
export type McsrcTokenType = 'class' | 'field' | 'method' | 'parameter' | 'local'

/**
 * 反编译源码中的 token：记录字符区间与语义，供着色与跳转使用。
 * 区间基于 `McsrcDecompileResult.source` 的字符偏移（0 起，左闭右开）。
 */
export interface McsrcToken {
  type: McsrcTokenType
  /** 起始字符偏移 */
  start: number
  /** 字符长度 */
  length: number
  /** 是否为声明处（声明处不作为跳转起点） */
  declaration: boolean
  /**
   * 点号全限定类名：
   *   - class token → 被引用的类；
   *   - field / method token → 引用点看到的所属类（可能不是真正的声明类，需沿继承链向上找）。
   */
  className: string
  /** field / method token 的成员名 */
  name?: string
  /** field / method token 的 JVM 描述符（如 Ljava/lang/String; 或 (I)V） */
  descriptor?: string
  /** 目标类（去掉内部类后缀）是否存在于该 jar；false 表示不可跳转 */
  inJar: boolean
}

/** 跳转后在目标类源码中需要定位的成员声明 */
export interface McsrcRevealTarget {
  type: 'field' | 'method'
  name: string
  descriptor?: string
}

/** 源码跳转请求（SourceViewer → 页面） */
export interface McsrcJumpTarget {
  /** 目标类（点号全限定名） */
  className: string
  /** 需要定位的成员；为空表示仅打开类源码 */
  reveal: McsrcRevealTarget | null
}

/** 反编译结果（浏览器内 Vineflower 产出） */
export interface McsrcDecompileResult {
  /** 点号全限定类名 */
  className: string
  /** Java 源码 */
  source: string
  language: 'java'
  /** 源码 token（着色 / 跳转）；引擎未产出时为空数组 */
  tokens: McsrcToken[]
}
