// Package mcrand 复刻 Java java.util.Random 的核心算法（48 位 LCG）。
// Minecraft 结构生成与史莱姆区块判定依赖此随机序列。
package mcrand

// Rand Java Random 的等价实现。
type Rand struct {
	seed int64
}

const (
	mask48  = int64((1 << 48) - 1)
	multiplier = 0x5DEECE66D
	addend     = 0xB
)

// New 以种子创建随机数生成器。
func New(seed int64) *Rand {
	// Java: setSeed(seed) = (seed ^ multiplier) & mask
	return &Rand{seed: (seed ^ multiplier) & mask48}
}

// next 产生 16+bits 位随机数（Java Random.next(bits)）。
func (r *Rand) next(bits int) int64 {
	r.seed = (r.seed*multiplier + addend) & mask48
	return r.seed >> (48 - bits)
}

// NextLong 返回 64 位随机长整数（Java Random.nextLong()）。
func (r *Rand) NextLong() int64 {
	return r.next(32)<<32 + r.next(32)
}

// NextInt 返回 [0, bound) 内的随机整数（Java Random.nextInt(bound)）。
// 完全复刻 Java 的拒绝采样逻辑，保证序列一致。
func (r *Rand) NextInt(bound int) int {
	if bound <= 0 {
		panic("bound must be positive")
	}
	if bound&(bound-1) == 0 && bound != 0 {
		// 2 的幂：低 31 位直接截断
		return int((int64(bound) * r.next(31)) >> 31)
	}
	var bits, val int64
	for {
		bits = r.next(31)
		val = bits % int64(bound)
		if bits-val+int64(bound)-1 >= 0 {
			break
		}
	}
	return int(val)
}

// NextDouble 返回 [0,1) 双精度（Java Random.nextDouble()）。
func (r *Rand) NextDouble() float64 {
	return float64(r.next(26)) / float64(1<<26) +
		float64(r.next(27)) / float64(1<<53)
}

// NextFloat 返回 [0,1) 单精度（Java Random.nextFloat()）。
func (r *Rand) NextFloat() float32 {
	return float32(r.next(24)) / float32(1<<24)
}