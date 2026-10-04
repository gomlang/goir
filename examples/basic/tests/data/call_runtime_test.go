package generated

import (
	"goir-calls-test/baseline"
	"goir-calls-test/constrained"
	"goir-calls-test/optimized"
	"goir-calls-test/optstack"
	"goir-calls-test/spilled"
	"runtime"
	"sync"
	"testing"
)

func TestCallsGrowStacksWithLiveGoObjects(t *testing.T) {
	recursive := []func(int64) int64{CallRecursive, baseline.CallRecursive, constrained.CallRecursive, optimized.CallRecursive, optstack.CallRecursive, spilled.CallRecursive}
	collect := []func(int64) int64{CallCollect, baseline.CallCollect, constrained.CallCollect, optimized.CallCollect, optstack.CallCollect, spilled.CallCollect}
	var group sync.WaitGroup
	for backend := range recursive {
		group.Go(func() {
			sentinel := make([]*int64, 128)
			for index := range sentinel {
				value := int64(index + backend)
				sentinel[index] = &value
			}
			if got := recursive[backend](768); got != 768*769/2 {
				t.Errorf("backend %d recursive stack/GC: %d", backend, got)
			}
			if got := collect[backend](17); got != 21*17+190 {
				t.Errorf("backend %d live across GC: %d", backend, got)
			}
			for index, pointer := range sentinel {
				if *pointer != int64(index+backend) {
					t.Errorf("backend %d caller pointer %d corrupted", backend, index)
				}
			}
			runtime.KeepAlive(sentinel)
		})
	}
	group.Wait()
}
