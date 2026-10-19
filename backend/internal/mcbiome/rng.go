// Package mcbiome 实现 Minecraft Java 版 1.18+ 主世界生物群系采样。
//
// 算法与常量忠实移植自 cubiomes（MIT License, https://github.com/Cubitect/cubiomes）：
//   - Xoroshiro128++ 随机源（MC 1.18+ 世界生成随机数）
//   - 改进型 Perlin 噪声 / DoublePerlinNoise
//   - 六维气候参数（temperature/humidity/continentalness/erosion/depth/weirdness）
//   - 气候 spline（深度偏移）
//   - 生物群系决策树（tables/btree*.h，逐字节取自上游）
//
// 采样坐标为 1:4 的“quart”坐标（区块坐标 ×4），与 MC 生物群系分辨率的语义一致。
package mcbiome

import "math"

// xoroshiro128pp 是 MC 1.18+ 使用的 Xoroshiro128++ 随机源。
type xoroshiro struct {
	lo, hi uint64
}

// xSetSeed 复刻 cubiomes xSetSeed：以 splitmix64 风格混合初始种子。
func xSetSeed(value uint64) xoroshiro {
	const (
		xl = 0x9e3779b97f4a7c15
		xh = 0x6a09e667f3bcc909
		a  = 0xbf58476d1ce4e5b9
		b  = 0x94d049bb133111eb
	)
	l := value ^ xh
	h := l + xl
	l = (l ^ (l >> 30)) * a
	h = (h ^ (h >> 30)) * a
	l = (l ^ (l >> 27)) * b
	h = (h ^ (h >> 27)) * b
	l ^= l >> 31
	h ^= h >> 31
	return xoroshiro{lo: l, hi: h}
}

// rotl64 64 位循环左移。
func rotl64(x uint64, b uint) uint64 {
	return (x << b) | (x >> (64 - b))
}

// nextLong 复刻 xNextLong。
func (x *xoroshiro) nextLong() uint64 {
	l, h := x.lo, x.hi
	n := rotl64(l+h, 17) + l
	h ^= l
	x.lo = rotl64(l, 49) ^ h ^ (h << 21)
	x.hi = rotl64(h, 28)
	return n
}

// nextInt 复刻 xNextInt（带拒绝采样）。
func (x *xoroshiro) nextInt(n uint32) int {
	r := (x.nextLong() & 0xFFFFFFFF) * uint64(n)
	if uint32(r) < n {
		for uint32(r) < (^n+1)%n {
			r = (x.nextLong() & 0xFFFFFFFF) * uint64(n)
		}
	}
	return int(r >> 32)
}

// nextDouble 复刻 xNextDouble（53 位有效精度）。
func (x *xoroshiro) nextDouble() float64 {
	return float64(x.nextLong()>>(64-53)) * 1.1102230246251565e-16
}

// lerp 线性插值。
func lerp(part, from, to float64) float64 {
	return from + part*(to-from)
}

// lerp32 单精度线性插值（spline 构建阶段使用 float 语义）。
func lerp32(part, from, to float32) float32 {
	return from + part*(to-from)
}

// fabs32 单精度绝对值。
func fabs32(x float32) float32 {
	return float32(math.Abs(float64(x)))
}
