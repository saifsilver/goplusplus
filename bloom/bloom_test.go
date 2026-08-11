package bloom_test

import (
	"testing"

	"github.com/saifsilver/goplusplus/bloom"
)

func TestBloomFilter(t *testing.T) {
	filter := bloom.NewFilter(1000, 0.01)

	filter.Add("user_1001")
	filter.Add("user_1002")

	if !filter.MayContain("user_1001") {
		t.Errorf("expected MayContain('user_1001') to be true")
	}
	if filter.MayContain("non_existent_key_9999") {
		t.Errorf("expected MayContain('non_existent_key_9999') to be false")
	}
}

func TestHyperLogLog(t *testing.T) {
	hll := bloom.NewHyperLogLog()

	for i := 0; i < 100; i++ {
		hll.Add(string(rune(i)))
	}

	est := hll.EstimateCardinality()
	if est == 0 {
		t.Errorf("expected non-zero HLL cardinality estimation")
	}
}

func TestCountMinSketch(t *testing.T) {
	cms := bloom.NewCountMinSketch()

	cms.Add("item_apple", 5)
	cms.Add("item_apple", 3)

	freq := cms.EstimateFrequency("item_apple")
	if freq != 8 {
		t.Errorf("expected frequency 8 for item_apple, got %d", freq)
	}
}
