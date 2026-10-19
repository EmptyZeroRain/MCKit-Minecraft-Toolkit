// ===== 启动参数说明字典（移植自 CreeperCode argDict.js）=====
// 说明：-Xmx/-Xmn 后的数值各版本不同，按前缀匹配；
// 其余参数按 flag 名（value 的第一个空白分隔 token）精确匹配。

const ARG_DICT_EXACT: Record<string, string> = {
  '--username': '后接用户名',
  '--version': '后接游戏版本',
  '--gameDir': '后接游戏路径',
  '--assetsDir': '后接资源文件路径',
  '--assetIndex': '后接资源索引版本',
  '--uuid': '后接用户 UUID',
  '--accessToken': '后接登录令牌',
  '--userType': '后接用户类型',
  '--versionType': '后接版本类型，会显示在游戏主界面右下角',
  '--width': '后接窗口宽度',
  '--height': '后接窗口高度',
  '--server': '后接服务器地址，游戏进入时将直接连入服务器',
  '--port': '后接服务器端口号',
  '-XX:+UseG1GC': '开启 G1 垃圾回收器',
  '-XX:-UseAdaptiveSizePolicy': '自动选择年轻代区大小和相应的 Survivor 区比例',
  '-XX:-OmitStackTraceInFastThrow': '省略异常栈信息从而快速抛出',
  '-Djava.library.path': '指定本地库（natives）搜索路径',
  '-cp': '指定类路径（classpath）',
  '-classpath': '指定类路径（classpath）',
}

const ARG_DICT_PREFIX: Array<[string, string]> = [
  ['-Xmx', '设置最大堆大小（-Xmx1024m 即 1024MB）'],
  ['-Xmn', '设置新生代堆大小（-Xmn128m 即 128MB）'],
  ['-Xms', '设置初始堆大小（-Xms1024m 即 1024MB）'],
  ['-Dorg.lwjgl', 'LWJGL 相关的系统属性'],
]

/** 返回启动参数解释，未收录时返回空串 */
export function argDesc(value: string): string {
  const flag = String(value).split(/\s+/)[0]
  if (ARG_DICT_EXACT[flag]) return ARG_DICT_EXACT[flag]
  for (const [prefix, desc] of ARG_DICT_PREFIX) {
    if (flag.startsWith(prefix)) return desc
  }
  return ''
}

/** 库原生变体分类名 -> 中文说明 */
export const CLASSIFIER_LABELS: Record<string, string> = {
  'natives-macos': 'macOS 原生库',
  'natives-windows': 'Windows 原生库',
  'natives-linux': 'Linux 原生库',
  'natives-osx': 'macOS 原生库',
}

export function classifierLabel(classifier: string): string {
  return CLASSIFIER_LABELS[classifier] ?? classifier
}