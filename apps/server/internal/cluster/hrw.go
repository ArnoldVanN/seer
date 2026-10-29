// Package cluster decides which worker owns which endpoint.
package cluster

import "hash/fnv"

// Owner picks the worker for key with rendezvous (highest random weight)
// hashing. Every worker computes the same answer from the same live set, and
// when a worker joins or leaves only ~1/N of the keys move.
func Owner(key string, workers []string) string {
	var best string
	var bestScore uint64
	for _, w := range workers {
		h := fnv.New64a()
		h.Write([]byte(w))
		h.Write([]byte{0})
		h.Write([]byte(key))
		// fnv on near-identical inputs clusters; mix it before comparing.
		s := mix(h.Sum64())
		if best == "" || s > bestScore || (s == bestScore && w < best) {
			best, bestScore = w, s
		}
	}
	return best
}

// mix is the splitmix64 finalizer.
func mix(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// Offset spreads endpoints with the same interval across it, so checks don't
// all fire on the same second. It's derived from the key alone, so a new
// owner keeps the same schedule.
func Offset(key string, intervalMs int64) int64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return int64(mix(h.Sum64()) % uint64(intervalMs))
}
