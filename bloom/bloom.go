package bloom

import (
	"hash/fnv"
	"math"
	"sync"
)

// Filter implements a high-performance Bloom Filter for preventing cache penetration attacks.
type Filter struct {
	mu        sync.RWMutex
	bitset    []bool
	size      uint64
	hashCount uint64
}

// NewFilter creates a new Bloom Filter tuned for expected capacity and false positive rate.
func NewFilter(capacity uint64, fpRate float64) *Filter {
	if capacity == 0 {
		capacity = 10000
	}
	if fpRate <= 0 || fpRate >= 1 {
		fpRate = 0.01
	}

	m := uint64(-1 * float64(capacity) * math.Log(fpRate) / math.Pow(math.Log(2), 2))
	k := uint64(float64(m) / float64(capacity) * math.Log(2))

	if m == 0 {
		m = 1000
	}
	if k == 0 {
		k = 3
	}

	return &Filter{
		bitset:    make([]bool, m),
		size:      m,
		hashCount: k,
	}
}

// Add inserts a key into the Bloom Filter.
func (f *Filter) Add(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := uint64(0); i < f.hashCount; i++ {
		idx := f.hash(key, i) % f.size
		f.bitset[idx] = true
	}
}

// MayContain tests if key is possibly in the set. If it returns false, the key 100% does NOT exist!
func (f *Filter) MayContain(key string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for i := uint64(0); i < f.hashCount; i++ {
		idx := f.hash(key, i) % f.size
		if !f.bitset[idx] {
			return false
		}
	}
	return true
}

func (f *Filter) hash(key string, seed uint64) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return h.Sum64() + seed*0x9e3779b97f4a7c15
}

// HyperLogLog implements probabilistic cardinality counting for unique visitors in constant memory.
type HyperLogLog struct {
	mu        sync.RWMutex
	registers []uint8
	m         uint64
}

// NewHyperLogLog creates a HyperLogLog cardinality counter.
func NewHyperLogLog() *HyperLogLog {
	m := uint64(1024)
	return &HyperLogLog{
		registers: make([]uint8, m),
		m:         m,
	}
}

// Add adds an element to the HyperLogLog set.
func (hll *HyperLogLog) Add(element string) {
	hll.mu.Lock()
	defer hll.mu.Unlock()

	h := fnv.New64a()
	_, _ = h.Write([]byte(element))
	hashVal := h.Sum64()

	idx := hashVal % hll.m
	zeros := uint8(1)
	val := hashVal >> 10
	for val&1 == 0 && zeros < 64 {
		zeros++
		val >>= 1
	}

	if zeros > hll.registers[idx] {
		hll.registers[idx] = zeros
	}
}

// EstimateCardinality returns the estimated count of unique elements.
func (hll *HyperLogLog) EstimateCardinality() uint64 {
	hll.mu.RLock()
	defer hll.mu.RUnlock()

	var sum float64
	for _, val := range hll.registers {
		sum += math.Pow(2, -float64(val))
	}
	alpha := 0.7213 / (1 + 1.079/float64(hll.m))
	estimate := alpha * float64(hll.m*hll.m) / sum
	return uint64(estimate)
}

// CountMinSketch implements frequency estimation for top-K trending items.
type CountMinSketch struct {
	mu     sync.RWMutex
	matrix [][]uint64
	depth  uint64
	width  uint64
}

// NewCountMinSketch creates a Count-Min Sketch frequency estimator.
func NewCountMinSketch() *CountMinSketch {
	d := uint64(5)
	w := uint64(2048)
	matrix := make([][]uint64, d)
	for i := range matrix {
		matrix[i] = make([]uint64, w)
	}
	return &CountMinSketch{matrix: matrix, depth: d, width: w}
}

// Add increments the frequency count for an item.
func (cms *CountMinSketch) Add(item string, count uint64) {
	cms.mu.Lock()
	defer cms.mu.Unlock()
	for i := uint64(0); i < cms.depth; i++ {
		h := fnv.New64a()
		_, _ = h.Write([]byte(item))
		idx := (h.Sum64() + i*0x9e3779b97f4a7c15) % cms.width
		cms.matrix[i][idx] += count
	}
}

// EstimateFrequency estimates the total frequency count of an item.
func (cms *CountMinSketch) EstimateFrequency(item string) uint64 {
	cms.mu.RLock()
	defer cms.mu.RUnlock()
	minVal := uint64(math.MaxUint64)
	for i := uint64(0); i < cms.depth; i++ {
		h := fnv.New64a()
		_, _ = h.Write([]byte(item))
		idx := (h.Sum64() + i*0x9e3779b97f4a7c15) % cms.width
		if cms.matrix[i][idx] < minVal {
			minVal = cms.matrix[i][idx]
		}
	}
	return minVal
}
