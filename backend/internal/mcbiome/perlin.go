package mcbiome

import "math"

// perlinNoise 复刻 cubiomes PerlinNoise（改进型 Perlin 噪声）。
type perlinNoise struct {
	d          [257]uint8
	h2         uint8
	a, b, c    float64
	amplitude  float64
	lacunarity float64
	d2, t2     float64
}

// indexedLerp 复刻 cubiomes indexedLerp（16 个梯度方向的点积）。
func indexedLerp(idx uint8, a, b, c float64) float64 {
	switch idx & 0xf {
	case 0:
		return a + b
	case 1:
		return -a + b
	case 2:
		return a - b
	case 3:
		return -a - b
	case 4:
		return a + c
	case 5:
		return -a + c
	case 6:
		return a - c
	case 7:
		return -a - c
	case 8:
		return b + c
	case 9:
		return -b + c
	case 10:
		return b - c
	case 11:
		return -b - c
	case 12:
		return a + b
	case 13:
		return -b + c
	case 14:
		return -a + b
	default:
		return -b - c
	}
}

// xPerlinInit 复刻 cubiomes xPerlinInit（1.18+ 使用 Xoroshiro 初始化）。
func xPerlinInit(xr *xoroshiro) perlinNoise {
	var n perlinNoise
	n.a = xr.nextDouble() * 256.0
	n.b = xr.nextDouble() * 256.0
	n.c = xr.nextDouble() * 256.0
	n.amplitude = 1.0
	n.lacunarity = 1.0

	for i := 0; i < 256; i++ {
		n.d[i] = uint8(i)
	}
	for i := 0; i < 256; i++ {
		j := xr.nextInt(uint32(256-i)) + i
		n.d[i], n.d[j] = n.d[j], n.d[i]
	}
	n.d[256] = n.d[0]

	i2 := math.Floor(n.b)
	d2 := n.b - i2
	n.h2 = uint8(int(i2))
	n.d2 = d2
	n.t2 = d2 * d2 * d2 * (d2*(d2*6.0-15.0) + 10.0)
	return n
}

// samplePerlin 复刻 cubiomes samplePerlin。
func (n *perlinNoise) sample(d1, d2, d3, yamp, ymin float64) float64 {
	var h2 uint8
	var t2 float64
	if d2 == 0.0 {
		d2 = n.d2
		h2 = n.h2
		t2 = n.t2
	} else {
		d2 += n.b
		i2 := math.Floor(d2)
		d2 -= i2
		h2 = uint8(int(i2))
		t2 = d2 * d2 * d2 * (d2*(d2*6.0-15.0) + 10.0)
	}

	d1 += n.a
	d3 += n.c

	i1 := math.Floor(d1)
	i3 := math.Floor(d3)
	d1 -= i1
	d3 -= i3

	h1 := uint8(int(i1))
	h3 := uint8(int(i3))

	t1 := d1 * d1 * d1 * (d1*(d1*6.0-15.0) + 10.0)
	t3 := d3 * d3 * d3 * (d3*(d3*6.0-15.0) + 10.0)

	if yamp != 0 {
		yclamp := d2
		if ymin < d2 {
			yclamp = ymin
		}
		d2 -= math.Floor(yclamp/yamp) * yamp
	}

	idx := &n.d
	// 复刻 cubiomes 的 vec2 查表路径（uint8 运算按位截断）。
	v1a := idx[h1] + h2
	v1b := idx[h1+1] + h2
	v2a := idx[v1a] + h3
	v2b := idx[v1a+1] + h3
	v3a := idx[v1b] + h3
	v3b := idx[v1b+1] + h3
	v4a := idx[v2a]
	v4b := idx[v2a+1]
	v5a := idx[v2b]
	v5b := idx[v2b+1]
	v6a := idx[v3a]
	v6b := idx[v3a+1]
	v7a := idx[v3b]
	v7b := idx[v3b+1]

	l1 := indexedLerp(v4a, d1, d2, d3)
	l5 := indexedLerp(v4b, d1, d2, d3-1)
	l2 := indexedLerp(v6a, d1-1, d2, d3)
	l6 := indexedLerp(v6b, d1-1, d2, d3-1)
	l3 := indexedLerp(v5a, d1, d2-1, d3)
	l7 := indexedLerp(v5b, d1, d2-1, d3-1)
	l4 := indexedLerp(v7a, d1-1, d2-1, d3)
	l8 := indexedLerp(v7b, d1-1, d2-1, d3-1)

	l1 = lerp(t1, l1, l2)
	l3 = lerp(t1, l3, l4)
	l5 = lerp(t1, l5, l6)
	l7 = lerp(t1, l7, l8)

	l1 = lerp(t2, l1, l3)
	l5 = lerp(t2, l5, l7)

	return lerp(t3, l1, l5)
}

// octaveNoise 复刻 cubiomes OctaveNoise（多倍频叠加）。
type octaveNoise struct {
	octaves []perlinNoise
}

// md5OctaveN 复刻 cubiomes md5_octave_n（"octave_-12".."octave_0" 的盐）。
var md5OctaveN = [13][2]uint64{
	{0xb198de63a8012672, 0x7b84cad43ef7b5a8},
	{0x0fd787bfbc403ec3, 0x74a4a31ca21b48b8},
	{0x36d326eed40efeb2, 0x5be9ce18223c636a},
	{0x082fe255f8be6631, 0x4e96119e22dedc81},
	{0x0ef68ec68504005e, 0x48b6bf93a2789640},
	{0xf11268128982754f, 0x257a1d670430b0aa},
	{0xe51c98ce7d1de664, 0x5f9478a733040c45},
	{0x6d7b49e7e429850a, 0x2e3063c622a24777},
	{0xbd90d5377ba1b762, 0xc07317d419a7548d},
	{0x53d39c6752dac858, 0xbcd1c5a80ab65b3e},
	{0xb4a24d7a84e7677b, 0x023ff9668e89b5c4},
	{0xdffa22b534c5f608, 0xb9b67517d3665ca9},
	{0xd50708086cef4d7c, 0x6e1651ecc7f43309},
}

// lacunaIni 复刻 cubiomes lacuna_ini（-omin = 3..12，索引 0 对应 -omin=0 的占位）。
var lacunaIni = [13]float64{
	1, .5, .25, 1. / 8, 1. / 16, 1. / 32, 1. / 64, 1. / 128, 1. / 256, 1. / 512, 1. / 1024, 1. / 2048, 1. / 4096,
}

// persistIni 复刻 cubiomes persist_ini（len = 4..9 起有效）。
var persistIni = [11]float64{
	0, 1, 2. / 3, 4. / 7, 8. / 15, 16. / 31, 32. / 63, 64. / 127, 128. / 255, 256. / 511,
}

// xOctaveInit 复刻 cubiomes xOctaveInit。
func xOctaveInit(xr *xoroshiro, amplitudes []float64, omin, length, nmax int) octaveNoise {
	lacuna := lacunaIni[-omin]
	persist := persistIni[length]
	xlo := xr.nextLong()
	xhi := xr.nextLong()

	var out octaveNoise
	for i, n := 0, 0; i < length && n != nmax; i, lacuna, persist = i+1, lacuna*2.0, persist*0.5 {
		if amplitudes[i] == 0 {
			continue
		}
		salt := md5OctaveN[12+omin+i]
		pxr := xoroshiro{lo: xlo ^ salt[0], hi: xhi ^ salt[1]}
		p := xPerlinInit(&pxr)
		p.amplitude = amplitudes[i] * persist
		p.lacunarity = lacuna
		out.octaves = append(out.octaves, p)
		n++
	}
	return out
}

// doublePerlinNoise 复刻 cubiomes DoublePerlinNoise。
type doublePerlinNoise struct {
	amplitude float64
	octA      octaveNoise
	octB      octaveNoise
}

// ampIni 复刻 cubiomes amp_ini（len = 2..9）。
var ampIni = [11]float64{
	0, 5. / 6, 10. / 9, 15. / 12, 20. / 15, 25. / 18, 30. / 21, 35. / 24, 40. / 27, 45. / 30,
}

// xDoublePerlinInit 复刻 cubiomes xDoublePerlinInit。
func xDoublePerlinInit(xr *xoroshiro, amplitudes []float64, omin, length, nmax int) doublePerlinNoise {
	na, nb := -1, -1
	if nmax > 0 {
		na = (nmax + 1) >> 1
		nb = nmax - na
	}
	var out doublePerlinNoise
	out.octA = xOctaveInit(xr, amplitudes, omin, length, na)
	out.octB = xOctaveInit(xr, amplitudes, omin, length, nb)

	// 修剪首尾为 0 的幅度，用于查 amp_ini。
	l := length
	for i := l - 1; i >= 0 && amplitudes[i] == 0.0; i-- {
		l--
	}
	for i := 0; amplitudes[i] == 0.0; i++ {
		l--
	}
	out.amplitude = ampIni[l]
	return out
}

// sampleOctave 复刻 cubiomes sampleOctave。
func (n *octaveNoise) sample(x, y, z float64) float64 {
	var v float64
	for i := range n.octaves {
		p := &n.octaves[i]
		lf := p.lacunarity
		ax := x * lf
		ay := y * lf
		az := z * lf
		v += p.amplitude * p.sample(ax, ay, az, 0, 0)
	}
	return v
}

// sampleDoublePerlin 复刻 cubiomes sampleDoublePerlin。
func (n *doublePerlinNoise) sample(x, y, z float64) float64 {
	const f = 337.0 / 331.0
	v := n.octA.sample(x, y, z)
	v += n.octB.sample(x*f, y*f, z*f)
	return v * n.amplitude
}
