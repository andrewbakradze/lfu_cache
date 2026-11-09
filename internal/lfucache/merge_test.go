package lfucache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergePreservesFrequencyAndOrder(t *testing.T) {
	t.Parallel()
	first := New[int, string](10)
	first.Put(1, "first")
	requireHit(t, first, 1, "first")
	requireHit(t, first, 1, "first")
	second := New[string, []byte](10)
	second.Put("2", []byte("second"))
	for range 4 {
		requireHit(t, second, "2", []byte("second"))
	}
	calls := 0
	merged, err := first.Merge(second, func(key string, value []byte) (int, string) {
		calls++
		require.Equal(t, "2", key)
		return 2, string(value)
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, 20, merged.Capacity())
	requireState(t, merged, entry(2, "second", 5), entry(1, "first", 3))
	requireState(t, first, entry(1, "first", 3))
	requireState(t, second, entry("2", []byte("second"), 5))
}

func TestMergeCollidingKeyCombinesFrequency(t *testing.T) {
	t.Parallel()
	dst := New[int, string](1)
	dst.Put(1, "destination")
	src := New[string, string](1)
	src.Put("1", "source")
	requireHit(t, src, "1", "source")
	merged, err := dst.Merge(src, func(string, string) (int, string) { return 1, "source" })
	require.NoError(t, err)
	require.Equal(t, 2, merged.Capacity())
	requireState(t, merged, entry(1, "source", 3))
	requireState(t, dst, entry(1, "destination", 1))
	requireState(t, src, entry("1", "source", 2))
}

func TestMergeRejectsNonInjectiveTransform(t *testing.T) {
	t.Parallel()
	dst := New[int, string](2)
	dst.Put(1, "destination")
	src := New[string, string](2)
	src.Put("first", "one")
	src.Put("second", "two")
	merged, err := dst.Merge(src, func(string, string) (int, string) { return 1, "source" })
	require.ErrorIs(t, err, ErrMergeTransformNotInjective)
	require.Nil(t, merged)
	requireState(t, dst, entry(1, "destination", 1))
	requireState(t, src, entry("second", "two", 1), entry("first", "one", 1))
}

func TestMergeIsGenericOverSourceCache(t *testing.T) {
	t.Parallel()
	dst := New[int, string](1)
	src := New[uint64, struct{ name string }](1)
	src.Put(42, struct{ name string }{name: "answer"})
	merged, err := dst.Merge(src, func(key uint64, value struct{ name string }) (int, string) {
		require.Equal(t, uint64(42), key)
		return int(key), value.name
	})
	require.NoError(t, err)
	requireState(t, merged, entry(42, "answer", 1))
}

func TestMergeUsesDestinationFirstForEqualFrequency(t *testing.T) {
	t.Parallel()
	dst := New[int, string](1)
	dst.Put(1, "destination")
	src := New[string, string](1)
	src.Put("2", "source")
	merged, err := dst.Merge(src, func(string, string) (int, string) { return 2, "source" })
	require.NoError(t, err)
	requireState(t, merged, entry(1, "destination", 1), entry(2, "source", 1))
}

func TestMergePreservesDestinationOrderForCollidingKeys(t *testing.T) {
	t.Parallel()
	dst, src := New[int, int](2), New[int, int](2)
	dst.Put(1, 10)
	dst.Put(2, 20)
	src.Put(2, 200)
	src.Put(1, 100)
	calls := make(map[int]int)
	merged, err := dst.Merge(src, func(key, value int) (int, int) {
		calls[key]++
		return key, value
	})
	require.NoError(t, err)
	require.Equal(t, map[int]int{1: 1, 2: 1}, calls)
	require.Equal(t, 4, merged.Capacity())
	requireState(t, merged, entry(2, 200, 2), entry(1, 100, 2))
	requireState(t, dst, entry(2, 20, 1), entry(1, 10, 1))
	requireState(t, src, entry(1, 100, 1), entry(2, 200, 1))
}

func TestMergeOrderAndIndependence(t *testing.T) {
	t.Parallel()
	dst, src := New[int, int](2), New[int, int](2)
	dst.Put(1, 10)
	dst.Put(2, 20)
	src.Put(3, 30)
	src.Put(4, 40)
	merged, err := dst.Merge(src, func(key, value int) (int, int) { return key, value })
	require.NoError(t, err)
	requireState(t, merged, entry(2, 20, 1), entry(1, 10, 1), entry(4, 40, 1), entry(3, 30, 1))
	merged.Put(4, 400)
	requireState(t, src, entry(4, 40, 1), entry(3, 30, 1))
	dst.Put(1, 100)
	src.Put(3, 300)
	requireState(t, merged, entry(4, 400, 2), entry(2, 20, 1), entry(1, 10, 1), entry(3, 30, 1))
}

func TestMergeEmptyCaches(t *testing.T) {
	t.Parallel()
	dst, src := New[int, int](2), New[int, int](3)
	calls := 0
	merged, err := dst.Merge(src, func(key, value int) (int, int) { calls++; return key, value })
	require.NoError(t, err)
	require.Zero(t, calls)
	require.Equal(t, 5, merged.Capacity())
	requireState(t, merged)
	dst.Put(1, 10)
	merged, err = dst.Merge(src, func(key, value int) (int, int) { calls++; return key, value })
	require.NoError(t, err)
	require.Zero(t, calls)
	requireState(t, merged, entry(1, 10, 1))
}
