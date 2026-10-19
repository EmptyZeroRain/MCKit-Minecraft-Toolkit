package mcbiome

// spline 复刻 cubiomes Spline / FixSpline。
// 气候参数到“深度偏移”的样条求值使用它。
type spline struct {
	typ    int
	loc    []float32
	der    []float32
	val    []*spline
	fixVal float32
	isFix  bool
}

// spline 类型（对应 cubiomes SP_* 枚举，同时也是 np_param 的下标）。
const (
	spContinentalness = 0
	spErosion         = 1
	spRidges          = 2
	spWeirdness       = 3
)

// add 追加一个样条控制点。
func (s *spline) add(loc float32, val *spline, der float32) {
	s.loc = append(s.loc, loc)
	s.val = append(s.val, val)
	s.der = append(s.der, der)
}

// fix 创建一个常量样条（cubiomes FixSpline）。
func fix(v float32) *spline {
	return &spline{fixVal: v, isFix: true}
}

// evalSpline 复刻 cubiomes getSpline。
func evalSpline(s *spline, vals [4]float32) float32 {
	if s.isFix {
		return s.fixVal
	}
	f := vals[s.typ]
	n := len(s.loc)
	i := 0
	for i < n && !(s.loc[i] >= f) {
		i++
	}
	if i == 0 || i == n {
		if i != 0 {
			i--
		}
		v := evalSpline(s.val[i], vals)
		return v + s.der[i]*(f-s.loc[i])
	}
	sp1, sp2 := s.val[i-1], s.val[i]
	g, h := s.loc[i-1], s.loc[i]
	k := (f - g) / (h - g)
	lm, m := s.der[i-1], s.der[i]
	nn := evalSpline(sp1, vals)
	o := evalSpline(sp2, vals)
	// C 中该表达式以 double 计算后截断为 float。
	p := float64(lm)*(float64(h)-float64(g)) - (float64(o) - float64(nn))
	q := -float64(m)*(float64(h)-float64(g)) + (float64(o) - float64(nn))
	r := lerp(float64(k), float64(nn), float64(o)) +
		float64(k)*(1.0-float64(k))*lerp(float64(k), p, q)
	return float32(r)
}

// getOffsetValue 复刻 cubiomes getOffsetValue。
func getOffsetValue(weirdness, continentalness float32) float32 {
	f0 := 1.0 - (1.0-continentalness)*0.5
	f1 := 0.5 * (1.0 - continentalness)
	f2 := (weirdness + 1.17) * 0.46082947
	off := f2*f0 - f1
	if weirdness < -0.7 {
		if off > -0.2222 {
			return off
		}
		return -0.2222
	}
	if off > 0 {
		return off
	}
	return 0
}

// createSpline38219 复刻 cubiomes createSpline_38219。
func createSpline38219(f float32, bl bool) *spline {
	sp := &spline{typ: spRidges}

	i := getOffsetValue(-1.0, f)
	k := getOffsetValue(1.0, f)
	l := 1.0 - (1.0-f)*0.5
	u := 0.5 * (1.0 - f)
	l = u/(0.46082947*l) - 1.17

	if -0.65 < l && l < 1.0 {
		u = getOffsetValue(-0.65, f)
		p := getOffsetValue(-0.75, f)
		q := (p - i) * 4.0
		r := getOffsetValue(l, f)
		s := (k - r) / (1.0 - l)

		sp.add(-1.0, fix(i), q)
		sp.add(-0.75, fix(p), 0)
		sp.add(-0.65, fix(u), 0)
		sp.add(l-0.01, fix(r), 0)
		sp.add(l, fix(r), s)
		sp.add(1.0, fix(k), s)
	} else {
		u = (k - i) * 0.5
		if bl {
			iv := i
			if iv < 0.2 {
				iv = 0.2
			}
			sp.add(-1.0, fix(iv), 0)
			sp.add(0.0, fix(lerp32(0.5, i, k)), u)
		} else {
			sp.add(-1.0, fix(i), u)
		}
		sp.add(1.0, fix(k), u)
	}
	return sp
}

// createFlatOffsetSpline 复刻 cubiomes createFlatOffsetSpline。
func createFlatOffsetSpline(f, g, h, i, j, k float32) *spline {
	sp := &spline{typ: spRidges}

	l := 0.5 * (g - f)
	if l < k {
		l = k
	}
	m := 5.0 * (h - g)

	sp.add(-1.0, fix(f), l)
	mm := l
	if m < l {
		mm = m
	}
	sp.add(-0.4, fix(g), mm)
	sp.add(0.0, fix(h), m)
	sp.add(0.4, fix(i), 2.0*(i-h))
	sp.add(1.0, fix(j), 0.7*(j-i))
	return sp
}

// createLandSpline 复刻 cubiomes createLandSpline。
func createLandSpline(f, g, h, i, j, k float32, bl bool) *spline {
	sp1 := createSpline38219(lerp32(i, 0.6, 1.5), bl)
	sp2 := createSpline38219(lerp32(i, 0.6, 1.0), bl)
	sp3 := createSpline38219(i, bl)
	ih := 0.5 * i
	sp4 := createFlatOffsetSpline(f-0.15, ih, ih, ih, i*0.6, 0.5)
	sp5 := createFlatOffsetSpline(f, j*i, g*i, ih, i*0.6, 0.5)
	sp6 := createFlatOffsetSpline(f, j, j, g, h, 0.5)
	sp7 := createFlatOffsetSpline(f, j, j, g, h, 0.5)

	sp8 := &spline{typ: spRidges}
	sp8.add(-1.0, fix(f), 0)
	sp8.add(-0.4, sp6, 0)
	sp8.add(0.0, fix(h+0.07), 0)

	sp9 := createFlatOffsetSpline(-0.02, k, k, g, h, 0.0)

	sp := &spline{typ: spErosion}
	sp.add(-0.85, sp1, 0)
	sp.add(-0.7, sp2, 0)
	sp.add(-0.4, sp3, 0)
	sp.add(-0.35, sp4, 0)
	sp.add(-0.1, sp5, 0)
	sp.add(0.2, sp6, 0)
	if bl {
		sp.add(0.4, sp7, 0)
		sp.add(0.45, sp8, 0)
		sp.add(0.55, sp8, 0)
		sp.add(0.58, sp7, 0)
	}
	sp.add(0.7, sp9, 0)
	return sp
}

// initBiomeSpline 复刻 cubiomes initBiomeNoise 中的样条构建。
func initBiomeSpline() *spline {
	sp := &spline{typ: spContinentalness}

	sp1 := createLandSpline(-0.15, 0.00, 0.0, 0.1, 0.00, -0.03, false)
	sp2 := createLandSpline(-0.10, 0.03, 0.1, 0.1, 0.01, -0.03, false)
	sp3 := createLandSpline(-0.10, 0.03, 0.1, 0.7, 0.01, -0.03, true)
	sp4 := createLandSpline(-0.05, 0.03, 0.1, 1.0, 0.01, 0.01, true)

	sp.add(-1.10, fix(0.044), 0.0)
	sp.add(-1.02, fix(-0.2222), 0.0)
	sp.add(-0.51, fix(-0.2222), 0.0)
	sp.add(-0.44, fix(-0.12), 0.0)
	sp.add(-0.18, fix(-0.12), 0.0)
	sp.add(-0.16, sp1, 0.0)
	sp.add(-0.15, sp1, 0.0)
	sp.add(-0.10, sp2, 0.0)
	sp.add(0.25, sp3, 0.0)
	sp.add(1.00, sp4, 0.0)
	return sp
}
