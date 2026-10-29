package cluster

import (
	"fmt"
	"testing"
)

func TestOwnerStableAndBalanced(t *testing.T) {
	workers := []string{"a", "b", "c"}
	counts := map[string]int{}
	before := map[string]string{}
	for i := range 3000 {
		k := fmt.Sprintf("endpoint-%d", i)
		o := Owner(k, workers)
		if o != Owner(k, []string{"c", "a", "b"}) {
			t.Fatalf("owner of %s depends on worker order", k)
		}
		counts[o]++
		before[k] = o
	}
	for w, n := range counts {
		if n < 800 || n > 1200 {
			t.Errorf("worker %s owns %d of 3000 keys, want ~1000", w, n)
		}
	}

	// Removing b moves only b's keys.
	for k, o := range before {
		after := Owner(k, []string{"a", "c"})
		if o != "b" && after != o {
			t.Fatalf("%s moved from %s to %s though %s is still live", k, o, after, o)
		}
	}
}
