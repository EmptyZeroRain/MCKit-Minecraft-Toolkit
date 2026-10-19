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
