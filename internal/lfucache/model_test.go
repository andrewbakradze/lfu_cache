package lfucache

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func testImplements[K comparable, V any]() Cache[K, V] {
	return New[K, V](1)
}

func TestWithoutInvalidation(t *testing.T) {
	t.Parallel()
	cache := New[int, int](3)
	require.Equal(t, unsafe.Sizeof((*int)(nil)), unsafe.Sizeof(cache))
	cache.Put(1, 1)
	cache.Put(2, 4)
	cache.Put(3, 9)
	requireHit(t, cache, 1, 1)
	requireHit(t, cache, 2, 4)
	requireHit(t, cache, 3, 9)
	requireHit(t, cache, 1, 1)
	requireState(t, cache, entry(1, 1, 3), entry(3, 9, 2), entry(2, 4, 2))
}

func TestIteratorOrder(t *testing.T) {
	t.Parallel()
	for _, capacity := range []int{1, 2, 5, 25} {
		for _, seed := range []uint64{1, 42, 2026} {
			t.Run(fmt.Sprintf("capacity=%d/seed=%d", capacity, seed), func(t *testing.T) {
				t.Parallel()
				cache := New[int, int](capacity)
				model := cacheModel{items: make(map[int]modelEntry), capacity: capacity}
				rng := rand.New(rand.NewPCG(seed, seed+1))
				for step := range 2000 {
					key := rng.IntN(2*capacity + 1)
					if rng.IntN(3) == 0 {
						value, found := model.get(key)
						if found {
							requireHit(t, cache, key, value)
						} else {
							requireMiss(t, cache, key)
						}
					} else {
						value := rng.IntN(100)
						cache.Put(key, value)
						model.put(key, value)
					}
					require.Equal(t, capacity, cache.Capacity(), "step %d", step)
					requireState(t, cache, model.ordered()...)
				}
			})
		}
	}
}

func TestIteratorDifferentFrequency(t *testing.T) {
	t.Parallel()
	cache := New[int, int](5)
	for i := 1; i <= 5; i++ {
		cache.Put(i, i*10)
		for range i {
			requireHit(t, cache, i, i*10)
		}
	}
	var keys, values []int
	calls := 0
	cache.All()(func(k, v int) bool {
		calls++
		require.LessOrEqual(t, calls, 3)
		require.Equal(t, k*10, v)
		if k == 3 {
			return false
		}
		keys, values = append(keys, k), append(values, v)
		return true
	})
	require.Equal(t, []int{5, 4}, keys)
	require.Equal(t, []int{50, 40}, values)
	require.Equal(t, 3, calls)
}

func TestKeyNotFound(t *testing.T) {
	t.Parallel()
	requireMiss(t, New[int, int](3), 1)
}

func TestUpdatePutFrequency(t *testing.T) {
	t.Parallel()
	cache := New[int, int](3)
	for i := 1; i <= 3; i++ {
		cache.Put(i, i*10)
	}
	for i := 3; i >= 1; i-- {
		cache.Put(i, i*10)
	}
	requireState(t, cache, entry(1, 10, 2), entry(2, 20, 2), entry(3, 30, 2))
}

func TestDefaultCapacity(t *testing.T) {
	t.Parallel()
	require.Equal(t, DefaultCapacity, New[***int, ***int]().Capacity())
}

func TestIterator(t *testing.T) {
	t.Parallel()
	cache := New[int, int](4)
	for i := 1; i <= 5; i++ {
		cache.Put(i, i*10)
	}
	seq := cache.All()
	for _, stop := range []int{1, 3, 4} {
		calls := 0
		seq(func(k, v int) bool {
			calls++
			require.LessOrEqual(t, calls, stop)
			require.Equal(t, 6-calls, k)
			require.Equal(t, k*10, v)
			return calls < stop
		})
		require.Equal(t, stop, calls)
	}
	requireState(t, cache, entry(5, 50, 1), entry(4, 40, 1), entry(3, 30, 1), entry(2, 20, 1))
}

func TestFrequencyReplacement(t *testing.T) {
	t.Parallel()
	cache := New[int, int](2)
	cache.Put(1, 10)
	cache.Put(2, 20)
	requireHit(t, cache, 1, 10)
	cache.Put(3, 30)
	requireMiss(t, cache, 2)
	requireHit(t, cache, 3, 30)
	cache.Put(4, 40)
	requireMiss(t, cache, 1)
	requireHit(t, cache, 3, 30)
	requireHit(t, cache, 4, 40)
	requireState(t, cache, entry(3, 30, 3), entry(4, 40, 2))
}

func TestCacheSize(t *testing.T) {
	t.Parallel()
	cache := New[int, int](1)
	cache.Put(1, 10)
	require.Equal(t, 1, cache.Size())
}

func TestNotPositiveCapacityPanics(t *testing.T) {
	t.Parallel()
	for _, capacity := range []int{0, -1, -2} {
		require.Panics(t, func() { New[int, int](capacity) })
	}
}

func TestGetKeyFrequencyNonExistent(t *testing.T) {
	t.Parallel()
	frequency, err := New[int, int](1).GetKeyFrequency(1)
	require.ErrorIs(t, err, ErrKeyNotFound)
	require.Zero(t, frequency)
}

func TestGetIncreasesFrequency(t *testing.T) {
	t.Parallel()
	cache := New[*int, string](1)
	key := new(int)
	cache.Put(key, "zero")
	requireHit(t, cache, key, "zero")
	requireHit(t, cache, key, "zero")
	requireFrequency(t, cache, key, 3)
}

func TestUpdateValueChangeFrequency(t *testing.T) {
	t.Parallel()
	cache := New[int, string](2)
	cache.Put(1, "one")
	requireHit(t, cache, 1, "one")
	cache.Put(1, "first")
	requireHit(t, cache, 1, "first")
	requireFrequency(t, cache, 1, 4)
}

func TestAllOrdering(t *testing.T) {
	t.Parallel()
	cache := New[int, int](3)
	for i := 1; i <= 3; i++ {
		cache.Put(i, i*10)
	}
	requireHit(t, cache, 2, 20)
	requireHit(t, cache, 3, 30)
	requireHit(t, cache, 3, 30)
	requireState(t, cache, entry(3, 30, 3), entry(2, 20, 2), entry(1, 10, 1))
}

func TestWithCustomTypes(t *testing.T) {
	t.Parallel()
	type key struct{ id int }
	type value struct {
		name string
		data map[string]int
	}
	cache := New[key, value](1)
	cache.Put(key{1}, value{name: "one", data: map[string]int{"x": 1}})
	cache.Put(key{2}, value{name: "two"})
	requireMiss(t, cache, key{1})
	requireHit(t, cache, key{2}, value{name: "two"})
}

func TestAllOnEmptyCache(t *testing.T) {
	t.Parallel()
	requireState(t, New[int, int](1))
}

func TestEvictionTieBreaker(t *testing.T) {
	t.Parallel()
	cache := New[int, string](2)
	cache.Put(1, "one")
	cache.Put(2, "two")
	cache.Put(3, "three")
	requireMiss(t, cache, 1)
	requireHit(t, cache, 2, "two")
	requireHit(t, cache, 3, "three")
}

func TestAllIterator(t *testing.T) {
	t.Parallel()
	cache := New[int, int](5)
	for i := 1; i <= 5; i++ {
		cache.Put(i, i*10)
	}
	requireState(t, cache, entry(5, 50, 1), entry(4, 40, 1), entry(3, 30, 1), entry(2, 20, 1), entry(1, 10, 1))
}

func TestGetOrLoad(t *testing.T) {
	t.Parallel()
	type user struct{ name string }
	type userRow struct{ firstName, lastName string }
	cache := New[int, user](2)
	cache.Put(1, user{name: "cached"})
	loaderCalls, convertCalls := 0, 0
	loader := func(id int) (userRow, error) {
		loaderCalls++
		require.Equal(t, 2, id)
		return userRow{firstName: "Ada", lastName: "Lovelace"}, nil
	}
	convert := func(row userRow) (user, error) {
		convertCalls++
		return user{name: row.firstName + " " + row.lastName}, nil
	}
	value, err := cache.GetOrLoad[userRow](1, loader, convert)
	require.NoError(t, err)
	require.Equal(t, user{name: "cached"}, value)
	require.Zero(t, loaderCalls)
	require.Zero(t, convertCalls)
	requireFrequency(t, cache, 1, 2)
	value, err = cache.GetOrLoad[userRow](2, loader, convert)
	require.NoError(t, err)
	require.Equal(t, user{name: "Ada Lovelace"}, value)
	require.Equal(t, 1, loaderCalls)
	require.Equal(t, 1, convertCalls)
	requireFrequency(t, cache, 2, 1)
	value, err = cache.GetOrLoad[userRow](2, loader, convert)
	require.NoError(t, err)
	require.Equal(t, user{name: "Ada Lovelace"}, value)
	require.Equal(t, 1, loaderCalls)
	require.Equal(t, 1, convertCalls)
	requireFrequency(t, cache, 2, 2)
}

func TestGetOrLoadErrorsDoNotChangeCache(t *testing.T) {
	t.Parallel()
	for _, failConvert := range []bool{false, true} {
		t.Run(fmt.Sprintf("convert=%t", failConvert), func(t *testing.T) {
			t.Parallel()
			cache := New[int, string](2)
			cache.Put(1, "one")
			cache.Put(2, "two")
			wantErr := errors.New("load failed")
			loaderCalls, convertCalls := 0, 0
			value, err := cache.GetOrLoad[int](3, func(key int) (int, error) {
				loaderCalls++
				require.Equal(t, 3, key)
				if failConvert {
					return 42, nil
				}
				return 42, wantErr
			}, func(int) (string, error) {
				convertCalls++
				return "must not be returned", wantErr
			})
			require.ErrorIs(t, err, wantErr)
			require.Same(t, wantErr, err)
			require.Zero(t, value)
			require.Equal(t, 1, loaderCalls)
			if failConvert {
				require.Equal(t, 1, convertCalls)
			} else {
				require.Zero(t, convertCalls)
			}
			requireMiss(t, cache, 3)
			requireState(t, cache, entry(2, "two", 1), entry(1, "one", 1))
		})
	}
}

func TestNoSlices(t *testing.T) {
	t.Parallel()
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.ArrayType:
				require.NotNil(t, n.Len, "%s: slices are prohibited", fset.Position(n.Pos()))
			case *ast.CallExpr:
				if name, ok := n.Fun.(*ast.Ident); ok {
					require.NotEqual(t, "append", name.Name, "%s: append is prohibited", fset.Position(n.Pos()))
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
}
