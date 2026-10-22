package generated

import (
	"fmt"
	baseline "goir-scalar-test/baseline"
	constrained "goir-scalar-test/constrained"
	linear "goir-scalar-test/linear"
	optimized "goir-scalar-test/optimized"
	optstack "goir-scalar-test/optstack"
	spilled "goir-scalar-test/spilled"
	"math"
	"math/bits"
	"strings"
	"testing"
)

func requireScalarPanic(t *testing.T, message string, call func()) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil || !strings.Contains(fmt.Sprint(value), message) {
			t.Fatalf("expected panic %q, received %v", message, value)
		}
	}()
	call()
}

func TestScalar64(t *testing.T) {
	variants := []struct {
		name  string
		div   func(int64, int64) (int64, int64, int64, int64, int64, int64)
		shift func(int64, int64) (int64, int64, int64, int64, int64)
		fold  func(int64, int64) int64
	}{
		{"generated", Div64, Shift64, Fold64},
		{"baseline", baseline.Div64, baseline.Shift64, baseline.Fold64},
		{"constrained", constrained.Div64, constrained.Shift64, constrained.Fold64},
		{"spilled", spilled.Div64, spilled.Shift64, spilled.Fold64},
		{"optimized", optimized.Div64, optimized.Shift64, optimized.Fold64},
		{"optstack", optstack.Div64, optstack.Shift64, optstack.Fold64},
		{"linear", linear.Div64, linear.Shift64, linear.Fold64},
	}
	values := []int64{math.MinInt64, math.MinInt64 + 1, -1000, -65, -1, 0, 1, 31, 32, 63, 64, 65, 1000, math.MaxInt64}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for _, a := range values {
				for _, b := range values {
					if got := variant.fold(a, b); got != a-b {
						t.Fatalf("fold %d,%d: %d", a, b, got)
					}
					if b == 0 {
						requireScalarPanic(t, "integer divide by zero", func() { variant.div(a, b) })
					} else {
						q, r, u, v, x, y := variant.div(a, b)
						if q != a/b || r != a%b || u != int64(uint64(a)/uint64(b)) || v != int64(uint64(a)%uint64(b)) || x != a || y != b {
							t.Fatalf("divide %d,%d: %d,%d,%d,%d,%d,%d", a, b, q, r, u, v, x, y)
						}
					}
					if b < 0 {
						requireScalarPanic(t, "negative shift amount", func() { variant.shift(a, b) })
					} else {
						l, u, s, x, y := variant.shift(a, b)
						if l != a<<b || u != int64(uint64(a)>>b) || s != a>>b || x != a || y != b {
							t.Fatalf("shift %d,%d: %d,%d,%d,%d,%d", a, b, l, u, s, x, y)
						}
					}
				}
			}
		})
	}
}

func TestScalar32(t *testing.T) {
	variants := []struct {
		name  string
		div   func(int32, int32) (int32, int32, int32, int32, int32, int32)
		shift func(int32, int32) (int32, int32, int32, int32, int32)
		fold  func(int32, int32) int32
	}{
		{"generated", Div32, Shift32, Fold32},
		{"baseline", baseline.Div32, baseline.Shift32, baseline.Fold32},
		{"constrained", constrained.Div32, constrained.Shift32, constrained.Fold32},
		{"spilled", spilled.Div32, spilled.Shift32, spilled.Fold32},
		{"optimized", optimized.Div32, optimized.Shift32, optimized.Fold32},
		{"optstack", optstack.Div32, optstack.Shift32, optstack.Fold32},
		{"linear", linear.Div32, linear.Shift32, linear.Fold32},
	}
	values := []int32{math.MinInt32, math.MinInt32 + 1, -1000, -33, -1, 0, 1, 31, 32, 33, 63, 64, 1000, math.MaxInt32}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for _, a := range values {
				for _, b := range values {
					if got := variant.fold(a, b); got != a-b {
						t.Fatalf("fold %d,%d: %d", a, b, got)
					}
					if b == 0 {
						requireScalarPanic(t, "integer divide by zero", func() { variant.div(a, b) })
					} else {
						q, r, u, v, x, y := variant.div(a, b)
						if q != a/b || r != a%b || u != int32(uint32(a)/uint32(b)) || v != int32(uint32(a)%uint32(b)) || x != a || y != b {
							t.Fatalf("divide %d,%d: %d,%d,%d,%d,%d,%d", a, b, q, r, u, v, x, y)
						}
					}
					if b < 0 {
						requireScalarPanic(t, "negative shift amount", func() { variant.shift(a, b) })
					} else {
						l, u, s, x, y := variant.shift(a, b)
						if l != a<<b || u != int32(uint32(a)>>b) || s != a>>b || x != a || y != b {
							t.Fatalf("shift %d,%d: %d,%d,%d,%d,%d", a, b, l, u, s, x, y)
						}
					}
				}
			}
		})
	}
}

func TestScalarFloatMemory(t *testing.T) {
	variants := []func(float64, float64) float64{FoldFloat, baseline.FoldFloat, constrained.FoldFloat, spilled.FoldFloat, optimized.FoldFloat, optstack.FoldFloat, linear.FoldFloat}
	values := []float64{0, math.Copysign(0, -1), 1, -1, math.SmallestNonzeroFloat64, math.MaxFloat64, math.Inf(1), math.Inf(-1), math.Float64frombits(0x7ff8000000000042)}
	for index, variant := range variants {
		for _, a := range values {
			for _, b := range values {
				got, want := variant(a, b), a-b
				if math.Float64bits(got) != math.Float64bits(want) && !(math.IsNaN(got) && math.IsNaN(want)) {
					t.Fatalf("variant %d: %v-%v: %v != %v", index, a, b, got, want)
				}
			}
		}
	}
}

func TestIntegerIdentities32(t *testing.T) {
	variants := []struct {
		name string
		call func(int32) (int32, int32, int32)
	}{
		{"generated", Identity32},
		{"baseline", baseline.Identity32},
		{"constrained", constrained.Identity32},
		{"spilled", spilled.Identity32},
		{"optimized", optimized.Identity32},
		{"optstack", optstack.Identity32},
		{"linear", linear.Identity32},
	}
	values := []int32{math.MinInt32, math.MinInt32 + 1, -65, -32, -1, 0, 1, 31, 32, 65, math.MaxInt32}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for _, input := range values {
				identity, zero, ones := variant.call(input)
				if identity != input || zero != 0 || ones != -1 {
					t.Fatalf("input %d: got %d,%d,%d", input, identity, zero, ones)
				}
			}
		})
	}
}

func TestRotate32(t *testing.T) {
	variants := []struct {
		name string
		run  func(int32, int32) (int32, int32, int32, int32, int32, int32)
	}{
		{"generated", Rotate32},
		{"baseline", baseline.Rotate32},
		{"constrained", constrained.Rotate32},
		{"spilled", spilled.Rotate32},
		{"optimized", optimized.Rotate32},
		{"optstack", optstack.Rotate32},
		{"linear", linear.Rotate32},
	}
	values := []int32{math.MinInt32, math.MinInt32 + 1, -1000, -65, -64, -33, -32, -1, 0, 1, 31, 32, 33, 63, 64, 65, math.MaxInt32}
	for _, v := range variants {
		for _, x := range values {
			for _, n := range values {
				a, b, c, d, e, f := v.run(x, n)
				got := [6]int32{a, b, c, d, e, f}
				want := [6]int32{int32(bits.RotateLeft32(uint32(x), int(n))), int32(bits.RotateLeft32(uint32(x), -1)), int32(bits.RotateLeft32(1, -1)), int32(bits.RotateLeft32(uint32(x), -int(n&31))), int32(bits.RotateLeft32(uint32(x), 1)), 2}
				if got != want {
					t.Fatalf("%s rotate32(%d,%d): got %v want %v", v.name, x, n, got, want)
				}
			}
		}
	}
}

func TestRotate64(t *testing.T) {
	variants := []struct {
		name string
		run  func(int64, int64) (int64, int64, int64, int64, int64, int64)
	}{
		{"generated", Rotate64},
		{"baseline", baseline.Rotate64},
		{"constrained", constrained.Rotate64},
		{"spilled", spilled.Rotate64},
		{"optimized", optimized.Rotate64},
		{"optstack", optstack.Rotate64},
		{"linear", linear.Rotate64},
	}
	values := []int64{math.MinInt64, math.MinInt64 + 1, -1000, -65, -64, -33, -32, -1, 0, 1, 31, 32, 33, 63, 64, 65, math.MaxInt64}
	for _, v := range variants {
		for _, x := range values {
			for _, n := range values {
				a, b, c, d, e, f := v.run(x, n)
				got := [6]int64{a, b, c, d, e, f}
				want := [6]int64{int64(bits.RotateLeft64(uint64(x), int(n))), int64(bits.RotateLeft64(uint64(x), -1)), int64(bits.RotateLeft64(1, -1)), int64(bits.RotateLeft64(uint64(x), -int(n&63))), int64(bits.RotateLeft64(uint64(x), 1)), 2}
				if got != want {
					t.Fatalf("%s rotate64(%d,%d): got %v want %v", v.name, x, n, got, want)
				}
			}
		}
	}
}

func TestBitcounts32(t *testing.T) {
	variants := []struct {
		name  string
		count func(int32) (int32, int32, int32, int32, int32, int32, int32)
	}{{"generated", Count32},
		{"baseline", baseline.Count32},
		{"constrained", constrained.Count32},
		{"spilled", spilled.Count32},
		{"optimized", optimized.Count32},
		{"optstack", optstack.Count32},
		{"linear", linear.Count32}}
	values := []int32{0, 1, -1, -2, math.MinInt32, math.MaxInt32, 0x55555555, -1431655766}
	for bit := 0; bit < 32; bit++ {
		values = append(values, int32(uint32(1)<<bit), ^int32(uint32(1)<<bit))
	}
	state := uint64(0x123456789abcdef)
	for i := 0; i < 256; i++ {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		values = append(values, int32(state))
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for _, input := range values {
				clz, zc, ctz, zt, pop, zp, preserved := variant.count(input)
				if clz != int32(bits.LeadingZeros32(uint32(input))) || ctz != int32(bits.TrailingZeros32(uint32(input))) || pop != int32(bits.OnesCount32(uint32(input))) || zc != 32 || zt != 32 || zp != 0 || preserved != input {
					t.Fatalf("%d: clz=%d ctz=%d pop=%d zeros=%d,%d,%d preserved=%d", input, clz, ctz, pop, zc, zt, zp, preserved)
				}
			}
		})
	}
}

func TestBitcounts64(t *testing.T) {
	variants := []struct {
		name  string
		count func(int64) (int64, int64, int64, int64, int64, int64, int64)
	}{{"generated", Count64},
		{"baseline", baseline.Count64},
		{"constrained", constrained.Count64},
		{"spilled", spilled.Count64},
		{"optimized", optimized.Count64},
		{"optstack", optstack.Count64},
		{"linear", linear.Count64}}
	values := []int64{0, 1, -1, -2, math.MinInt64, math.MaxInt64, 0x55555555, -1431655766}
	for bit := 0; bit < 64; bit++ {
		values = append(values, int64(uint64(1)<<bit), ^int64(uint64(1)<<bit))
	}
	state := uint64(0x123456789abcdef)
	for i := 0; i < 256; i++ {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		values = append(values, int64(state))
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for _, input := range values {
				clz, zc, ctz, zt, pop, zp, preserved := variant.count(input)
				if clz != int64(bits.LeadingZeros64(uint64(input))) || ctz != int64(bits.TrailingZeros64(uint64(input))) || pop != int64(bits.OnesCount64(uint64(input))) || zc != 64 || zt != 64 || zp != 0 || preserved != input {
					t.Fatalf("%d: clz=%d ctz=%d pop=%d zeros=%d,%d,%d preserved=%d", input, clz, ctz, pop, zc, zt, zp, preserved)
				}
			}
		})
	}
}

func TestMinmax32(t *testing.T) {
	variants := []struct {
		name string
		call func(int32, int32) (int32, int32, int32, int32, int32, int32, int32, int32, int32, int32, int32, int32, int32, int32)
	}{
		{"generated", Minmax32},
		{"baseline", baseline.Minmax32},
		{"constrained", constrained.Minmax32},
		{"spilled", spilled.Minmax32},
		{"optimized", optimized.Minmax32},
		{"optstack", optstack.Minmax32},
		{"linear", linear.Minmax32},
	}
	values := []int32{math.MinInt32, math.MinInt32 + 1, -17, -1, 0, 1, 17, math.MaxInt32}
	state := uint64(0x123456789abcdef)
	for range 48 {
		state = state*6364136223846793005 + 1
		values = append(values, int32(state>>32))
	}
	for _, variant := range variants {
		for _, a := range values {
			for _, b := range values {
				smin, sminc, sminl, smax, smaxc, smaxl, umin, uminc, uminl, umax, umaxc, umaxl, x, y := variant.call(a, b)
				got := [14]int32{smin, sminc, sminl, smax, smaxc, smaxl, umin, uminc, uminl, umax, umaxc, umaxl, x, y}
				want := [14]int32{min(a, b), min(a, 17), min(17, a), max(a, b), max(a, 17), max(17, a), int32(min(uint32(a), uint32(b))), int32(min(uint32(a), 17)), int32(min(17, uint32(a))), int32(max(uint32(a), uint32(b))), int32(max(uint32(a), 17)), int32(max(17, uint32(a))), a, b}
				if got != want {
					t.Fatalf("%s minmax %d,%d: got %v want %v", variant.name, a, b, got, want)
				}
			}
		}
	}
}

func TestMinmax64(t *testing.T) {
	variants := []struct {
		name string
		call func(int64, int64) (int64, int64, int64, int64, int64, int64, int64, int64, int64, int64, int64, int64, int64, int64)
	}{
		{"generated", Minmax64},
		{"baseline", baseline.Minmax64},
		{"constrained", constrained.Minmax64},
		{"spilled", spilled.Minmax64},
		{"optimized", optimized.Minmax64},
		{"optstack", optstack.Minmax64},
		{"linear", linear.Minmax64},
	}
	values := []int64{math.MinInt64, math.MinInt64 + 1, -17, -1, 0, 1, 17, math.MaxInt64}
	state := uint64(0x123456789abcdef)
	for range 48 {
		state = state*6364136223846793005 + 1
		values = append(values, int64(state>>0))
	}
	for _, variant := range variants {
		for _, a := range values {
			for _, b := range values {
				smin, sminc, sminl, smax, smaxc, smaxl, umin, uminc, uminl, umax, umaxc, umaxl, x, y := variant.call(a, b)
				got := [14]int64{smin, sminc, sminl, smax, smaxc, smaxl, umin, uminc, uminl, umax, umaxc, umaxl, x, y}
				want := [14]int64{min(a, b), min(a, 17), min(17, a), max(a, b), max(a, 17), max(17, a), int64(min(uint64(a), uint64(b))), int64(min(uint64(a), 17)), int64(min(17, uint64(a))), int64(max(uint64(a), uint64(b))), int64(max(uint64(a), 17)), int64(max(17, uint64(a))), a, b}
				if got != want {
					t.Fatalf("%s minmax %d,%d: got %v want %v", variant.name, a, b, got, want)
				}
			}
		}
	}
}

func TestBswap32(t *testing.T) {
	variants := []struct {
		name string
		swap func(int32) (int32, int32, int32, int32)
	}{
		{"generated", Bswap32},
		{"baseline", baseline.Bswap32},
		{"constrained", constrained.Bswap32},
		{"spilled", spilled.Bswap32},
		{"optimized", optimized.Bswap32},
		{"optstack", optstack.Bswap32},
		{"linear", linear.Bswap32},
	}
	values := []int32{0, -1, 1, 128, math.MinInt32, math.MaxInt32}
	state := uint64(0x123456789abcdef)
	for i := 0; i < 512; i++ {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		values = append(values, int32(state))
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for _, input := range values {
				got, twice, constant, preserved := variant.swap(input)
				want := int32(bits.ReverseBytes32(uint32(input)))
				if got != want || twice != input || constant != math.MinInt32 || preserved != input {
					t.Fatalf("%d: got=%d want=%d twice=%d constant=%d preserved=%d", input, got, want, twice, constant, preserved)
				}
			}
		})
	}
}

func TestBswap64(t *testing.T) {
	variants := []struct {
		name string
		swap func(int64) (int64, int64, int64, int64)
	}{
		{"generated", Bswap64},
		{"baseline", baseline.Bswap64},
		{"constrained", constrained.Bswap64},
		{"spilled", spilled.Bswap64},
		{"optimized", optimized.Bswap64},
		{"optstack", optstack.Bswap64},
		{"linear", linear.Bswap64},
	}
	values := []int64{0, -1, 1, 128, math.MinInt64, math.MaxInt64}
	state := uint64(0x123456789abcdef)
	for i := 0; i < 512; i++ {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		values = append(values, int64(state))
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			for _, input := range values {
				got, twice, constant, preserved := variant.swap(input)
				want := int64(bits.ReverseBytes64(uint64(input)))
				if got != want || twice != input || constant != math.MinInt64 || preserved != input {
					t.Fatalf("%d: got=%d want=%d twice=%d constant=%d preserved=%d", input, got, want, twice, constant, preserved)
				}
			}
		})
	}
}
