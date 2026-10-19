// ===== 字节码（ASM Textifier 风格文本）的 Monaco 语言支持 =====
//
// 服务端用纯 Go 把 class 字节码渲染成 ASM Textifier 风格的文本（见
// backend/internal/mcsrc/disasm.go），这里为它注册 Monarch 词法高亮与配套主题。
// 词法规则与上游 mcsrc（src/utils/JavaBytecode.ts）保持一致，便于对照上游界面。
//
// 只在首次打开字节码视图时注册一次；Monaco 体积较大，本模块只被懒加载的
// SourceViewer 引入，不会进入首屏包。
import { monaco } from './monaco'
import { MCSRC_BYTECODE_LANGUAGE } from '@/types/mcsrc'

/** 字节码 Monaco 语言 id（与页面入口共享同一常量，避免字面量漂移） */
export const BYTECODE_LANGUAGE = MCSRC_BYTECODE_LANGUAGE

/** 亮色主题 id（base: vs，继承基础主题，只覆盖字节码相关 token） */
export const BYTECODE_THEME_LIGHT = 'mckit-bytecode-light'
/** 暗色主题 id（base: vs-dark，继承基础主题） */
export const BYTECODE_THEME_DARK = 'mckit-bytecode-dark'

let registered = false

/** 指令操作码（JVMS §6.5），与后端 opcodeTable 同源。 */
const OPCODES = [
  'NOP', 'ACONST_NULL', 'ICONST_M1', 'ICONST_0', 'ICONST_1', 'ICONST_2', 'ICONST_3', 'ICONST_4',
  'ICONST_5', 'LCONST_0', 'LCONST_1', 'FCONST_0', 'FCONST_1', 'FCONST_2', 'DCONST_0', 'DCONST_1',
  'BIPUSH', 'SIPUSH', 'LDC', 'LDC_W', 'LDC2_W', 'ILOAD', 'LLOAD', 'FLOAD', 'DLOAD', 'ALOAD',
  'ILOAD_0', 'ILOAD_1', 'ILOAD_2', 'ILOAD_3', 'LLOAD_0', 'LLOAD_1', 'LLOAD_2', 'LLOAD_3',
  'FLOAD_0', 'FLOAD_1', 'FLOAD_2', 'FLOAD_3', 'DLOAD_0', 'DLOAD_1', 'DLOAD_2', 'DLOAD_3',
  'ALOAD_0', 'ALOAD_1', 'ALOAD_2', 'ALOAD_3', 'IALOAD', 'LALOAD', 'FALOAD', 'DALOAD', 'AALOAD',
  'BALOAD', 'CALOAD', 'SALOAD', 'ISTORE', 'LSTORE', 'FSTORE', 'DSTORE', 'ASTORE', 'ISTORE_0',
  'ISTORE_1', 'ISTORE_2', 'ISTORE_3', 'LSTORE_0', 'LSTORE_1', 'LSTORE_2', 'LSTORE_3', 'FSTORE_0',
  'FSTORE_1', 'FSTORE_2', 'FSTORE_3', 'DSTORE_0', 'DSTORE_1', 'DSTORE_2', 'DSTORE_3', 'ASTORE_0',
  'ASTORE_1', 'ASTORE_2', 'ASTORE_3', 'IASTORE', 'LASTORE', 'FASTORE', 'DASTORE', 'AASTORE',
  'BASTORE', 'CASTORE', 'SASTORE', 'POP', 'POP2', 'DUP', 'DUP_X1', 'DUP_X2', 'DUP2', 'DUP2_X1',
  'DUP2_X2', 'SWAP', 'IADD', 'LADD', 'FADD', 'DADD', 'ISUB', 'LSUB', 'FSUB', 'DSUB', 'IMUL',
  'LMUL', 'FMUL', 'DMUL', 'IDIV', 'LDIV', 'FDIV', 'DDIV', 'IREM', 'LREM', 'FREM', 'DREM', 'INEG',
  'LNEG', 'FNEG', 'DNEG', 'ISHL', 'LSHL', 'ISHR', 'LSHR', 'IUSHR', 'LUSHR', 'IAND', 'LAND', 'IOR',
  'LOR', 'IXOR', 'LXOR', 'IINC', 'I2L', 'I2F', 'I2D', 'L2I', 'L2F', 'L2D', 'F2I', 'F2L', 'F2D',
  'D2I', 'D2L', 'D2F', 'I2B', 'I2C', 'I2S', 'LCMP', 'FCMPL', 'FCMPG', 'DCMPL', 'DCMPG', 'IFEQ',
  'IFNE', 'IFLT', 'IFGE', 'IFGT', 'IFLE', 'IF_ICMPEQ', 'IF_ICMPNE', 'IF_ICMPLT', 'IF_ICMPGE',
  'IF_ICMPGT', 'IF_ICMPLE', 'IF_ACMPEQ', 'IF_ACMPNE', 'GOTO', 'JSR', 'RET', 'TABLESWITCH',
  'LOOKUPSWITCH', 'IRETURN', 'LRETURN', 'FRETURN', 'DRETURN', 'ARETURN', 'RETURN', 'GETSTATIC',
  'PUTSTATIC', 'GETFIELD', 'PUTFIELD', 'INVOKEVIRTUAL', 'INVOKESPECIAL', 'INVOKESTATIC',
  'INVOKEINTERFACE', 'INVOKEDYNAMIC', 'NEW', 'NEWARRAY', 'ANEWARRAY', 'ARRAYLENGTH', 'ATHROW',
  'CHECKCAST', 'INSTANCEOF', 'MONITORENTER', 'MONITOREXIT', 'WIDE', 'MULTIANEWARRAY', 'IFNULL',
  'IFNONNULL',
].join('|')

/** 伪指令 / 属性名（Textifier 输出中的大写指令）。 */
const DIRECTIVES =
  'MAXSTACK|MAXLOCALS|LINENUMBER|LOCALVARIABLE|FRAME|TRYCATCHBLOCK|FIELD|METHOD|OUTERCLASS|' +
  'DEPRECATED|SIGNATURE|SOURCEFILE|SOURCEDEBUGINFOEXTENSION|NESTHOST|NESTMEMBERS|' +
  'PERMITTEDSUBCLASSES|RECORDCOMPONENT|ANNOTATION|PARAMETER|ATTRIBUTE|INNERCLASS|' +
  'BOOTSTRAPMETHOD|NESTMEMBER|PERMITTEDSUBCLASS|ENCLOSINGMETHOD|THROWS|SIGNATURE'

/** 访问标志与修饰符。 */
const MODIFIERS =
  'public|private|protected|static|final|synchronized|volatile|transient|native|interface|' +
  'abstract|strictfp|synthetic|annotation|enum|mandated'

/** 类头关键字。 */
const KEYWORDS = 'class|extends|implements|version|access|flags|signature|declaration|parameter|handle|kind|arguments|itf|super'

/** 主题色（十六进制，不带 #）。 */
const COLORS_DARK = {
  comment: '6A9955',
  string: 'CE9178',
  opcode: '569CD6',
  directive: 'C586C0',
  label: '4EC9B0',
  number: 'B5CEA8',
  descriptor: '4EC9B0',
  primitive: '569CD6',
  annotation: 'DCDCAA',
  keyword: '569CD6',
  delimiter: '808080',
}

const COLORS_LIGHT = {
  comment: '008000',
  string: 'A31515',
  opcode: '0000FF',
  directive: 'AF00DB',
  label: '267F99',
  number: '098658',
  descriptor: '267F99',
  primitive: '0000FF',
  annotation: '795E26',
  keyword: '0000FF',
  delimiter: '808080',
}

/** 由配色生成 Monaco 主题规则。 */
function themeRules(c: typeof COLORS_DARK) {
  return [
    { token: 'comment', foreground: c.comment },
    { token: 'string', foreground: c.string },
    { token: 'string.invalid', foreground: c.string },
    { token: 'string.escape', foreground: c.number },
    { token: 'annotation', foreground: c.annotation },
    { token: 'label', foreground: c.label },
    { token: 'keyword.directive', foreground: c.directive },
    { token: 'keyword.opcode', foreground: c.opcode },
    { token: 'keyword', foreground: c.keyword },
    { token: 'type.primitive', foreground: c.primitive },
    { token: 'type.descriptor', foreground: c.descriptor },
    { token: 'number', foreground: c.number },
    { token: 'number.hex', foreground: c.number },
    { token: 'delimiter', foreground: c.delimiter },
    { token: 'delimiter.bracket', foreground: c.delimiter },
    { token: 'delimiter.angle', foreground: c.delimiter },
  ]
}

/** 注册字节码语言与主题（幂等）。 */
export function registerBytecodeLanguage(): void {
  if (registered) return
  registered = true

  monaco.languages.register({ id: BYTECODE_LANGUAGE })
  monaco.languages.setMonarchTokensProvider(BYTECODE_LANGUAGE, {
    tokenizer: {
      root: [
        // 注释
        [/\/\/.*$/, 'comment'],
        // 字符串
        [/"([^"\\]|\\.)*$/, 'string.invalid'],
        [/"/, 'string', '@string'],
        // 注解
        [/@[a-zA-Z_$][\w$]*/, 'annotation'],
        // 标签（L0 / L12）
        [/\bL\d+\b/, 'label'],
        // 伪指令 / 属性
        [new RegExp(`\\b(?:${DIRECTIVES})\\b`), 'keyword.directive'],
        // 操作码
        [new RegExp(`\\b(?:${OPCODES})\\b`), 'keyword.opcode'],
        // 访问标志与修饰符
        [new RegExp(`\\b(?:${MODIFIERS})\\b`), 'keyword'],
        // 类头关键字
        [new RegExp(`\\b(?:${KEYWORDS})\\b`), 'keyword'],
        // 基本类型
        [/\b(?:void|boolean|byte|char|short|int|long|float|double)\b/, 'type.primitive'],
        // 数字
        [/\b0[xX][0-9a-fA-F]+\b/, 'number.hex'],
        [/\b\d+\b/, 'number'],
        // 类型描述符
        [/\[L[a-zA-Z0-9_$/]+;/, 'type.descriptor'],
        [/L[a-zA-Z0-9_$/]+;/, 'type.descriptor'],
        [/\[[ZBCSIJFD]/, 'type.descriptor'],
        // 标识符（含 / 的包名）
        [/[a-zA-Z_$][\w$/]*/, 'identifier'],
        // 分隔符
        [/[{}()[\]]/, 'delimiter.bracket'],
        [/[<>]/, 'delimiter.angle'],
        [/[:,.]/, 'delimiter'],
      ],
      string: [
        [/[^\\"]+/, 'string'],
        [/\\./, 'string.escape'],
        [/"/, 'string', '@pop'],
      ],
    },
  })

  monaco.editor.defineTheme(BYTECODE_THEME_DARK, {
    base: 'vs-dark',
    inherit: true,
    rules: themeRules(COLORS_DARK),
    colors: {},
  })
  monaco.editor.defineTheme(BYTECODE_THEME_LIGHT, {
    base: 'vs',
    inherit: true,
    rules: themeRules(COLORS_LIGHT),
    colors: {},
  })
}

/** 字节码视图对应的 Monaco 主题 id。 */
export function bytecodeTheme(isDark: boolean): string {
  return isDark ? BYTECODE_THEME_DARK : BYTECODE_THEME_LIGHT
}
