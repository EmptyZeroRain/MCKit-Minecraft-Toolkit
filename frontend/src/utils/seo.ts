// ===== SEO：页面 meta 管理与 canonical（需求 9.3）=====

interface PageMetaOptions {
  title: string
  description?: string
  /** 相对路径，如 /tools/coordinate */
  canonicalPath?: string
  robot?: 'index,follow' | 'noindex,nofollow'
}

let globalSiteUrl = import.meta.env.VITE_SITE_URL || ''

/** 由外部（如部署脚本）覆盖站点 URL */
export function setSiteUrl(url: string) {
  globalSiteUrl = url
}

export function siteUrl(): string {
  return globalSiteUrl.replace(/\/$/, '')
}

function setMeta(name: string, content: string) {
  let el = document.head.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
  if (!el) {
    el = document.createElement('meta')
    el.setAttribute('name', name)
    document.head.appendChild(el)
  }
  el.setAttribute('content', content)
}

function setCanonical(href: string) {
  let el = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
  if (!el) {
    el = document.createElement('link')
    el.setAttribute('rel', 'canonical')
    document.head.appendChild(el)
  }
  el.setAttribute('href', href)
}

/**
 * 设置页面标题、描述与 canonical。
 * 管理页使用 noindex；详情页（工具/资源/百科）传独立 canonicalPath。
 * 未传描述时保持站点默认描述。
 */
export function applyPageMeta(opts: PageMetaOptions): void {
  const base = 'MC 工具站'
  document.title = opts.title === base ? base : `${opts.title} · ${base}`

  if (opts.description) {
    setMeta('description', opts.description)
  }
  setMeta('robots', opts.robot ?? 'index,follow')

  if (opts.canonicalPath) {
    setCanonical(siteUrl() + opts.canonicalPath)
  }
}

/** 重置管理页 SEO：noindex，避免管理页进入搜索结果 */
export function applyNoIndexMeta(): void {
  setMeta('robots', 'noindex,nofollow')
  document.title = `${document.title.split(' · ')[0]} · MC 工具站`
}

/** 生成单条搜索结果 JSON-LD（不生成虚假数据，仅真实条目） */
export function buildItemJsonLd(data: {
  name: string
  url: string
  description?: string
}): string {
  return JSON.stringify({
    '@context': 'https://schema.org',
    '@type': 'WebPage',
    name: data.name,
    url: data.url,
    description: data.description,
  })
}