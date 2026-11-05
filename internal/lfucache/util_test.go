package lfucache

import (
	"cmp"
	"iter"
	"runtime"
	"runtime/debug"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func collect[K comparable, V any](iterator iter.Seq2[K, V]) ([]K, []V) {
	keys, values := make([]K, 0), make([]V, 0)
	for k, v := range iterator {
		keys = append(keys, k)
		values = append(values, v)
	}
	return keys, values
}

type cacheEntry[K comparable, V any] struct {
	key       K
	value     V
	frequency int
}

func entry[K comparable, V any](key K, value V, frequency int) cacheEntry[K, V] {
	return cacheEntry[K, V]{key: key, value: value, frequency: frequency}
}

func requireHit[K comparable, V any](t testing.TB, cache Cache[K, V], key K, want V) {
	t.Helper()
	value, err := cache.Get(key)
	require.NoError(t, err)
	require.Equal(t, want, value)
}

func requireFrequency[K comparable, V any](t testing.TB, cache Cache[K, V], key K, want int) {
	t.Helper()
	frequency, err := cache.GetKeyFrequency(key)
	require.NoError(t, err)
	require.Equal(t, want, frequency)
}

func requireMiss[K comparable, V any](t testing.TB, cache Cache[K, V], key K) {
	t.Helper()
	value, err := cache.Get(key)
	require.ErrorIs(t, err, ErrKeyNotFound)
	require.Zero(t, value)
	frequency, err := cache.GetKeyFrequency(key)
	require.ErrorIs(t, err, ErrKeyNotFound)
	require.Zero(t, frequency)
}

func requireState[K comparable, V any](t testing.TB, cache Cache[K, V], want ...cacheEntry[K, V]) {
	t.Helper()
	require.Equal(t, len(want), cache.Size())
	index := 0
	cache.All()(func(key K, value V) bool {
		require.Less(t, index, len(want), "iterator yielded too many entries")
		require.Equal(t, want[index].key, key)
		require.Equal(t, want[index].value, value)
		requireFrequency(t, cache, key, want[index].frequency)
		index++
		return true
	})
	require.Equal(t, len(want), index)
}

type modelEntry struct {
	value     int
	frequency int
	lastUsed  int
}

type cacheModel struct {
	items    map[int]modelEntry
	capacity int
	clock    int
}

func (m *cacheModel) put(key, value int) {
	m.clock++
	if item, ok := m.items[key]; ok {
		item.value, item.frequency, item.lastUsed = value, item.frequency+1, m.clock
		m.items[key] = item
		return
	}
	if len(m.items) == m.capacity {
		ordered := m.ordered()
		delete(m.items, ordered[len(ordered)-1].key)
	}
	m.items[key] = modelEntry{value: value, frequency: 1, lastUsed: m.clock}
}

func (m *cacheModel) get(key int) (int, bool) {
	item, ok := m.items[key]
	if !ok {
		return 0, false
	}
	m.clock++
	item.frequency++
	item.lastUsed = m.clock
	m.items[key] = item
	return item.value, true
}

func (m *cacheModel) ordered() []cacheEntry[int, int] {
	result := make([]cacheEntry[int, int], 0, len(m.items))
	for key, item := range m.items {
		result = append(result, entry(key, item.value, item.frequency))
	}
	slices.SortFunc(result, func(a, b cacheEntry[int, int]) int {
		if order := cmp.Compare(b.frequency, a.frequency); order != 0 {
			return order
		}
		return cmp.Compare(m.items[b.key].lastUsed, m.items[a.key].lastUsed)
	})
	return result
}

func isolateRuntime(t *testing.T) {
	t.Helper()
	previousGC := debug.SetGCPercent(-1)
	previousProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() {
		debug.SetGCPercent(previousGC)
		runtime.GOMAXPROCS(previousProcs)
	})
}

func measureAllocations(work func()) (bytes, allocations uint64) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	work()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc, after.Mallocs - before.Mallocs
}

func benchmark(t *testing.T, work func(*testing.B)) testing.BenchmarkResult {
	t.Helper()
	result := testing.Benchmark(work)
	require.Positive(t, result.N, "benchmark correctness checks failed")
	return result
}

func requirePerformanceRatio(t *testing.T, actual, baseline testing.BenchmarkResult, limit float64) {
	t.Helper()
	require.Positive(t, baseline.NsPerOp())
	ratio := float64(actual.NsPerOp()) / float64(baseline.NsPerOp())
	t.Logf("cache %d ns/op, baseline %d ns/op, ratio %.2f, limit %.2f", actual.NsPerOp(), baseline.NsPerOp(), ratio, limit)
	require.LessOrEqual(t, ratio, limit)
}
