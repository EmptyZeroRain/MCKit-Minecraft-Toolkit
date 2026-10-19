package mcbiome

import "sync"

// 气候参数下标（复刻 cubiomes NP_* 枚举顺序）。
const (
	npTemperature     = 0
	npHumidity        = 1
	npContinentalness = 2
	npErosion         = 3
	npShift           = 4
	npWeirdness       = 5
	npMax             = 6
)

// biomeNoise 复刻 cubiomes BiomeNoise（1.18+ 主世界）。
type biomeNoise struct {
	climate [npMax]doublePerlinNoise
	sp      *spline
	mc      version
}

// climateParam 单个气候参数的初始化配置（omin/len/盐/幅度）。
type climateParam struct {
	omin int
	len  int
	amp  []float64
	salt [2]uint64
}

// 复刻 cubiomes init_climate_seed 中的参数表（large=false 即普通世界）。
var climateParams = [npMax]climateParam{
	npTemperature: {
		omin: -10, len: 6, amp: []float64{1.5, 0, 1, 0, 0, 0},
		salt: [2]uint64{0x5c7e6b29735f0d7f, 0xf7d86f1bbc734988},
	},
	npHumidity: {
		omin: -8, len: 6, amp: []float64{1, 1, 0, 0, 0, 0},
		salt: [2]uint64{0x81bb4d22e8dc168e, 0xf1c8b4bea16303cd},
	},
	npContinentalness: {
		omin: -9, len: 9, amp: []float64{1, 1, 2, 2, 2, 1, 1, 1, 1},
		salt: [2]uint64{0x83886c9d0ae3a662, 0xafa638a61b42e8ad},
	},
	npErosion: {
		omin: -9, len: 5, amp: []float64{1, 1, 0, 1, 1},
		salt: [2]uint64{0xd02491e6058f6fd8, 0x4792512c94c17a80},
	},
	npShift: {
		omin: -3, len: 4, amp: []float64{1, 1, 1, 0},
		salt: [2]uint64{0x080518cf6af25384, 0x3f3dfb40a54febd5},
	},
	npWeirdness: {
		omin: -7, len: 6, amp: []float64{1, 2, 1, 0, 0, 0},
		salt: [2]uint64{0xefc8ef4d36102b34, 0x1beeeb324a0f24ea},
	},
}

// splineOnce / sharedSpline 缓存与种子无关的气候样条。
var (
	splineOnce   sync.Once
	sharedSpline *spline
)

// newBiomeNoise 复刻 cubiomes setBiomeSeed（large=false）。
func newBiomeNoise(seed int64, v version) *biomeNoise {
	splineOnce.Do(func() {
		sharedSpline = initBiomeSpline()
	})

	bn := &biomeNoise{sp: sharedSpline, mc: v}

	xr := xSetSeed(uint64(seed))
	xlo := xr.nextLong()
	xhi := xr.nextLong()

	for i := 0; i < npMax; i++ {
		p := climateParams[i]
		pxr := xoroshiro{lo: xlo ^ p.salt[0], hi: xhi ^ p.salt[1]}
		bn.climate[i] = xDoublePerlinInit(&pxr, p.amp, p.omin, p.len, -1)
	}
	return bn
}

// 采样标志（复刻 cubiomes SAMPLE_*）。
const (
	// sampleNoShift 跳过局部坐标扰动。
	sampleNoShift = 0x1
	// sampleNoDepth 跳过深度项（用于地表群系地图，避免高地形处误判为洞穴群系）。
	sampleNoDepth = 0x2
)

// sampleBiome 复刻 cubiomes sampleBiomeNoise（x/y/z 为 1:4 的 quart 坐标）。
func (bn *biomeNoise) sampleBiome(x, y, z int, flags uint32) int {
	return climateToBiome(bn.mc, bn.climateParamsAt(x, y, z, flags))
}

// climateParamsAt 计算 (x,y,z)（quart）处的六维气候参数（×10000 定点）。
func (bn *biomeNoise) climateParamsAt(x, y, z int, flags uint32) [6]int64 {
	px := float64(x)
	pz := float64(z)
	if flags&sampleNoShift == 0 {
		px += bn.climate[npShift].sample(float64(x), 0, float64(z)) * 4.0
		pz += bn.climate[npShift].sample(float64(z), float64(x), 0) * 4.0
	}

	c := float32(bn.climate[npContinentalness].sample(px, 0, pz))
	e := float32(bn.climate[npErosion].sample(px, 0, pz))
	w := float32(bn.climate[npWeirdness].sample(px, 0, pz))

	// 深度项：由气候 spline 的偏移与 y 共同决定。
	// 跳过时 d 保持 0，即取“地表”群系（cubiomes SAMPLE_NO_DEPTH 语义）。
	var d float32
	if flags&sampleNoDepth == 0 {
		ridges := -3.0 * (fabs32(fabs32(w)-0.6666667) - 0.33333334)
		npParam := [4]float32{c, e, ridges, w}
		off := float64(evalSpline(bn.sp, npParam)) + 0.015
		d = float32(1.0 - float64(y*4)/128.0 - 83.0/160.0 + off)
	}

	t := float32(bn.climate[npTemperature].sample(px, 0, pz))
	h := float32(bn.climate[npHumidity].sample(px, 0, pz))

	const scale = float32(10000)
	return [6]int64{
		int64(scale * t),
		int64(scale * h),
		int64(scale * c),
		int64(scale * e),
		int64(scale * d),
		int64(scale * w),
	}
}
