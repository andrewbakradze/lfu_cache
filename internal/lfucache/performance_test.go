package lfucache

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPutPerformance(t *testing.T) {
	cache := benchmark(t, func(b *testing.B) {
		c := New[int, int](100)
		rng := rand.New(rand.NewPCG(1, 2))
		for b.Loop() {
			c.Put(rng.IntN(10), rng.IntN(10))
			_, _ = c.Get(rng.IntN(10))
		}
	})
	emulator := benchmark(t, func(b *testing.B) {
		a := make(map[int]int)
		rng := rand.New(rand.NewPCG(1, 2))
		for b.Loop() {
			a[rng.IntN(10)]++
			a[rng.IntN(10)]++
		}
	})
	requirePerformanceRatio(t, cache, emulator, 14)
}

func TestGetPutPerformanceWithHotCache(t *testing.T) {
	const count = 1000
	cache := benchmark(t, func(b *testing.B) {
		c := New[int, int](count)
		c.Put(-1, -1)
		for i := 1; i < count; i++ {
			for range i {
				c.Put(i, i)
			}
		}
		rng := rand.New(rand.NewPCG(1, 2))
		for b.Loop() {
			c.Put(count/2+rng.IntN(100), rng.IntN(10))
			_, _ = c.Get(count/2 + rng.IntN(100))
		}
	})
	emulator := benchmark(t, func(b *testing.B) {
		a := make(map[int]int)
		rng := rand.New(rand.NewPCG(1, 2))
		for b.Loop() {
			a[count/2+rng.IntN(100)]++
			a[count/2+rng.IntN(100)]++
		}
	})
	requirePerformanceRatio(t, cache, emulator, 20)
}

func TestIteratorPerformance(t *testing.T) {
	cache := benchmark(t, func(b *testing.B) {
		c := New[int, int](10)
		for range 100_000_000 {
			c.Put(-42, -42)
		}
		for i := 1; i <= 9; i++ {
			for range i * 10_000 {
				c.Put(i, i)
			}
		}
		for b.Loop() {
			collect(c.All())
		}
	})
	emulator := benchmark(t, func(b *testing.B) {
		m := make([]int, 10)
		for i := range m {
			m[i] = i
		}
		for b.Loop() {
			collect(slices.Backward(m))
		}
	})
	requirePerformanceRatio(t, cache, emulator, 5)
}

func TestIteratorAllocs(t *testing.T) {
	isolateRuntime(t)
	for range 5 {
		c := New[int, int](10_000)
		for i := 1; i <= 10_000; i++ {
			for range i {
				c.Put(i, i)
			}
		}
		calls := 0
		bytes, _ := measureAllocations(func() {
			c.All()(func(int, int) bool { calls++; return false })
		})
		require.Equal(t, 1, calls)
		require.LessOrEqual(t, bytes, uint64(1<<12))
	}
}

func TestNodeReclaiming(t *testing.T) {
	isolateRuntime(t)
	for range 1000 {
		cache := New[int, int](149)
		for i := 1; i <= 50; i++ {
			cache.Put(i, i)
		}
		for i := 52; i <= 150; i++ {
			for range i - 50 {
				cache.Put(i, i)
			}
		}
		_, allocations := measureAllocations(func() {
			for i := -50; i < -1; i++ {
				for range -i {
					cache.Put(i, i)
				}
			}
		})
		require.Zero(t, allocations)
	}
}

func TestListReclaiming(t *testing.T) {
	isolateRuntime(t)
	for range 100 {
		cache := New[int, int](100)
		for i := 1; i <= 1000; i++ {
			for range i {
				cache.Put(i, i)
			}
		}
		_, allocations := measureAllocations(func() {
			for i := 1000; i >= 1; i-- {
				cache.Put(i, i)
			}
			cache.Put(-1, -1)
		})
		require.Zero(t, allocations)
	}
}

func TestPutAllocs(t *testing.T) {
	isolateRuntime(t)
	for range 100 {
		cache := New[int, int](100)
		cache.Put(1, 10)
		cache.Put(2, 20)
		requireHit(t, cache, 2, 20)
		for range 10_000 {
			cache.Put(27000, 27000)
		}
		for i := 3; i <= 100; i++ {
			cache.Put(i, i)
		}
		_, allocations := measureAllocations(func() {
			for i := 3; i <= 100; i++ {
				cache.Put(i, i)
			}
		})
		require.Zero(t, allocations)
	}
}

func TestInvalidationPerformance(t *testing.T) {
	hot := benchmark(t, func(b *testing.B) {
		cache := New[int, int](1)
		invalid := false
		for b.Loop() {
			for range 10000 {
				cache.Put(42, 42)
			}
			cache.Put(1, 1)
			frequency, err := cache.GetKeyFrequency(1)
			invalid = invalid || err != nil || frequency != 1
			frequency, err = cache.GetKeyFrequency(42)
			invalid = invalid || !errors.Is(err, ErrKeyNotFound) || frequency != 0
		}
		require.False(b, invalid)
	})
	cold := benchmark(t, func(b *testing.B) {
		cache := New[int, int](2)
		invalid := false
		for b.Loop() {
			for range 10000 {
				cache.Put(42, 42)
			}
			cache.Put(42, 42)
			_, err := cache.GetKeyFrequency(42)
			invalid = invalid || err != nil
			frequency, err := cache.GetKeyFrequency(43)
			invalid = invalid || !errors.Is(err, ErrKeyNotFound) || frequency != 0
		}
		require.False(b, invalid)
	})
	requirePerformanceRatio(t, hot, cold, 1.3)
}
