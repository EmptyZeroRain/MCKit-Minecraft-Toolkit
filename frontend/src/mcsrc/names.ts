// ===== mcsrc 类名格式转换 =====
// mckit 前后端统一使用**点号**全限定名（net.minecraft.server.MinecraftServer）；
// 而 JVM 内部名（jar 条目名 / 常量池）使用**斜杠**（net/minecraft/server/MinecraftServer）。
// 反编译引擎（Vineflower）只认内部名，故在调用边界做转换。

/** 点号全限定名 → JVM 内部名（net.minecraft.Foo → net/minecraft/Foo） */
export function toInternalClassName(className: string): string {
  return className.replaceAll('.', '/')
}

/** JVM 内部名 → 点号全限定名（net/minecraft/Foo → net.minecraft.Foo） */
export function toDottedClassName(className: string): string {
  return className.replaceAll('/', '.')
}

/** 点号全限定名 → jar 条目路径（net.minecraft.Foo → net/minecraft/Foo.class） */
export function toClassFilePath(className: string): string {
  return `${toInternalClassName(className)}.class`
}

/**
 * 去掉内部类后缀（net/minecraft/Foo$Bar → net/minecraft/Foo）。
 * 内部类在 jar 中单独成条目，但「该类是否在 jar 内」按最外层类判断（与上游一致）。
 */
export function outerClassName(className: string): string {
  const i = className.indexOf('$')
  return i < 0 ? className : className.slice(0, i)
}

/**
 * 生成类名探测序列（内部名形式），用于把源码里的类名对齐到 jar 条目 / 后端索引的约定。
 *
 * 反编译源码里内部类写作 `Outer.Inner`（Java 语法），而 jar 条目与后端索引用 `Outer$Inner`；
 * 但 `.` 同时也是包分隔符，无法仅凭字符串判断，因此逐个从右往左把 `.` 换成 `$` 生成候选，
 * 由调用方按顺序探测 jar 条目集合。顺序：原名 → 最外层类 → 逐级 `$` 替换。
 */
export function classLookupCandidates(className: string): string[] {
  const internal = toInternalClassName(className)
  const candidates = [internal]
  const outer = outerClassName(internal)
  if (outer !== internal) candidates.push(outer)
  // 在**点号形式**上做替换（内部名里已无点号），逐个从右往左把包分隔符换成 `$`。
  const dotted = className.replaceAll('/', '.')
  for (let i = dotted.length - 1; i >= 0; i--) {
    if (dotted[i] === '.') {
      candidates.push(toInternalClassName(`${dotted.slice(0, i)}$${dotted.slice(i + 1)}`))
    }
  }
  return candidates
}
