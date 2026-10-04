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
