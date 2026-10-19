// 头像处理：把用户选择的图片文件压缩为小尺寸 data URI。
//
// 头像只允许上传图片，不接受任意 URL。为降低传输与存储开销（后端硬上限 512KB），
// 这里统一：短边居中裁剪为正方形 → 缩放到 256px → 输出 JPEG（GIF 保留原格式以免丢帧）。
// 浏览器不支持 canvas 时回退为原图 data URI，最终仍由后端校验兜底。

/** 头像输出边长（像素） */
const MAX_AVATAR_EDGE = 256
/** JPEG 输出质量 */
const JPEG_QUALITY = 0.85
/** 允许的图片类型（与后端 auth 包保持一致） */
export const ALLOWED_AVATAR_TYPES = 'image/png,image/jpeg,image/webp,image/gif'

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(new Error('读取图片失败'))
    reader.readAsDataURL(file)
  })
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('图片解析失败，请更换图片'))
    img.src = src
  })
}

/**
 * 将图片文件转换为方形头像 data URI。
 * 非图片文件或解析失败时抛出错误，由调用方提示用户。
 */
export async function fileToAvatarDataUrl(file: File): Promise<string> {
  if (!file.type.startsWith('image/')) {
    throw new Error('请选择图片文件（PNG/JPEG/WebP/GIF）')
  }
  const original = await readAsDataURL(file)
  // GIF 交给后端保留原格式（canvas 会丢失动画）。
  if (file.type === 'image/gif') return original

  const img = await loadImage(original)
  const side = Math.min(img.naturalWidth, img.naturalHeight)
  if (!side) return original

  const size = Math.min(side, MAX_AVATAR_EDGE)
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')
  if (!ctx) return original

  // 透明背景填白，避免转 JPEG 后出现黑底。
  ctx.fillStyle = '#ffffff'
  ctx.fillRect(0, 0, size, size)
  const sx = (img.naturalWidth - side) / 2
  const sy = (img.naturalHeight - side) / 2
  ctx.drawImage(img, sx, sy, side, side, 0, 0, size, size)
  return canvas.toDataURL('image/jpeg', JPEG_QUALITY)
}
