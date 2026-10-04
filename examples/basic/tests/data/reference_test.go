package generated

import (
	"goir-native-test/baseline"
	"goir-native-test/constrained"
	"goir-native-test/optimized"
	"runtime"
	"sync"
	"testing"
)

func TestGoReference(t *testing.T) {
	values := []int64{-1 << 63, -65, -1, 0, 1, 63, 64, 65, 1<<63 - 1}
	for _, a := range values {
		for _, b := range values {
			want := []int64{a + b, a - b, a * b, a & b, a | b, a ^ b, a << (uint64(b) & 63), int64(uint64(a) >> (uint64(b) & 63)), a >> (uint64(b) & 63)}
			got := []int64{Op0(a, b), Op1(a, b), Op2(a, b), Op3(a, b), Op4(a, b), Op5(a, b), Op6(a, b), Op7(a, b), Op8(a, b)}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("op %d(%d, %d): got %d, want %d", i, a, b, got[i], want[i])
				}
			}
			wantBool := []bool{a == b, a != b, a < b, a <= b, uint64(a) < uint64(b), uint64(a) <= uint64(b)}
			gotBool := []bool{Cmp0(a, b), Cmp1(a, b), Cmp2(a, b), Cmp3(a, b), Cmp4(a, b), Cmp5(a, b)}
			for i := range wantBool {
				if gotBool[i] != wantBool[i] {
					t.Fatalf("cmp %d(%d, %d): got %v, want %v", i, a, b, gotBool[i], wantBool[i])
				}
			}
		}
	}
}

func TestStackGrowthAndGoCallers(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			sentinel := []int64{int64(i), 17, 29}
			for j := range 16 {
				n := int64(i + j)
				if got := LargeStack(n); got != n+768*767/2 {
					t.Errorf("large frame: got %d", got)
				}
				if baseline.LargeStack(n) != n+768*767/2 || constrained.LargeStack(n) != n+768*767/2 {
					t.Error("alternate backend large frame")
				}
				a, b, c := Mixed(true, n, false)
				if !a || b != n || c {
					t.Errorf("mixed results: %v %d %v", a, b, c)
				}
				x, y := Swap(17, 29, n)
				if n%2 == 0 && (x != 17 || y != 29) || n%2 == 1 && (x != 29 || y != 17) {
					t.Errorf("parallel copy: %d %d", x, y)
				}
				if got := Sum(n); got != n*(n+1)/2 {
					t.Errorf("sum: got %d", got)
				}
				runtime.GC()
				if sentinel[0] != int64(i) || sentinel[1] != 17 || sentinel[2] != 29 {
					t.Error("caller values changed")
				}
				runtime.KeepAlive(sentinel)
			}
		})
	}
	wg.Wait()
}

var benchmarkResult int64

func BenchmarkSumRegisters(b *testing.B) {
	for b.Loop() {
		benchmarkResult = Sum(100)
	}
}

func BenchmarkSumStack(b *testing.B) {
	for b.Loop() {
		benchmarkResult = baseline.Sum(100)
	}
}

func BenchmarkChainRegisters(b *testing.B) {
	for b.Loop() {
		benchmarkResult = LargeStack(17)
	}
}

func BenchmarkChainStack(b *testing.B) {
	for b.Loop() {
		benchmarkResult = baseline.LargeStack(17)
	}
}

func BenchmarkSimplifyRaw(b *testing.B) {
	for b.Loop() {
		benchmarkResult = SimplifyChain(17)
	}
}

func BenchmarkSimplifyOptimized(b *testing.B) {
	for b.Loop() {
		benchmarkResult = optimized.SimplifyChain(17)
	}
}
