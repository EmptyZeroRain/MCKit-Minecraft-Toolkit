package captcha

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand/v2"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// 版面参数：留白、字间距与干扰元素数量。
const (
	padX      = 14
	padY      = 10
	charGap   = 4
	noiseLine = 5
	// noiseDots 噪点密度（占像素比例）。
	noiseDots = 0.02
)

// render 将验证码文本绘制为 PNG 图片。
//
// 抗自动识别手段：随机浅色背景 + 随机深色字色 + 每字纵向抖动 + 随机噪点与干扰线。
// 字体为内置 Go Mono（gomono），无需外部字体文件。
func (m *Manager) render(code string) ([]byte, error) {
	face := m.face
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	height := ascent + metrics.Descent.Ceil() + padY*2

	width := padX * 2
	advances := make([]int, 0, len(code))
	for _, ch := range code {
		adv := font.MeasureString(face, string(ch)).Ceil()
		advances = append(advances, adv)
		width += adv + charGap
	}
	width -= charGap

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 背景：随机浅色，保证与深色文字形成足够对比。
	bg := color.RGBA{
		R: uint8(225 + rand.IntN(30)),
		G: uint8(225 + rand.IntN(30)),
		B: uint8(225 + rand.IntN(30)),
		A: 255,
	}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	// 干扰线先画，随后被文字覆盖，避免线条横穿字形中心影响辨识。
	for i := 0; i < noiseLine; i++ {
		drawLine(img,
			rand.IntN(width), rand.IntN(height),
			rand.IntN(width), rand.IntN(height),
			randInk(120, 200),
		)
	}

	// 文字：逐字随机颜色与纵向抖动。
	x := padX
	for i, ch := range code {
		dy := rand.IntN(7) - 3
		d := &font.Drawer{
			Dst:  img,
			Src:  &image.Uniform{C: randInk(20, 90)},
			Face: face,
			Dot:  fixed.P(x, padY+ascent+dy),
		}
		d.DrawString(string(ch))
		x += advances[i] + charGap
	}

	// 噪点叠加在文字之上，进一步削弱 OCR。
	dots := int(float64(width*height) * noiseDots)
	for i := 0; i < dots; i++ {
		img.SetRGBA(rand.IntN(width), rand.IntN(height), randInk(60, 190))
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// randInk 生成指定灰度区间的随机中性色（R=G=B）。
func randInk(lo, hi int) color.RGBA {
	v := uint8(lo + rand.IntN(hi-lo+1))
	return color.RGBA{R: v, G: v, B: v, A: 255}
}

// drawLine 使用 Bresenham 算法绘制直线；调用方保证端点落在图像范围内。
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		img.SetRGBA(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
