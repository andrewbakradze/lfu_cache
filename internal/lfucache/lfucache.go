package lfucache

import (
	"errors"
	"iter"

	"github.com/andrewbakradze/lfu_cache/internal/linkedlist"
)

var (
	ErrKeyNotFound                = errors.New("key not found")
	ErrMergeTransformNotInjective = errors.New("merge transform is not injective")
)

var DefaultCapacity = 5

type Cache[K comparable, V any] interface {
	Get(key K) (V, error)
	Put(key K, value V)
	All() iter.Seq2[K, V]
	Size() int
	Capacity() int
	GetKeyFrequency(key K) (int, error)
}

type node[K comparable, V any] struct {
	key                  K
	value                V
	frequencyListElement *linkedlist.Element[frequencyGroup[K, V]]
}

type frequencyGroup[K comparable, V any] struct {
	thisFrequencyNodes linkedlist.List[*node[K, V]]
	frequency          int
}

type cacheImpl[K comparable, V any] struct {
	frequencyList      linkedlist.List[frequencyGroup[K, V]]
	keyToElement       map[K]*linkedlist.Element[*node[K, V]]
	capacity           int
	defaultValue       V
	freeFrequencyGroup linkedlist.List[frequencyGroup[K, V]]
}

func New[K comparable, V any](capacity ...int) *cacheImpl[K, V] {
	cacheCapacity := DefaultCapacity
	if len(capacity) > 0 {
		cacheCapacity = capacity[0]
	}
	if cacheCapacity <= 0 {
		panic("cache capacity must be positive")
	}
	var defaultValue V
	return &cacheImpl[K, V]{
		frequencyList:      linkedlist.New[frequencyGroup[K, V]](),
		keyToElement:       make(map[K]*linkedlist.Element[*node[K, V]], cacheCapacity),
		capacity:           cacheCapacity,
		defaultValue:       defaultValue,
		freeFrequencyGroup: linkedlist.New[frequencyGroup[K, V]](),
	}
}

func (l *cacheImpl[K, V]) getFrequencyGroup(frequency int, mark *linkedlist.Element[frequencyGroup[K, V]]) *linkedlist.Element[frequencyGroup[K, V]] {
	group := l.freeFrequencyGroup.Front()
	if group == nil {
		group = l.freeFrequencyGroup.PushBack(frequencyGroup[K, V]{
			thisFrequencyNodes: linkedlist.New[*node[K, V]](),
		})
	}
	group.Value.frequency = frequency
	l.frequencyList.TransferBackElement(group)
	if mark == nil {
		l.frequencyList.MoveToFront(group)
	} else {
		l.frequencyList.MoveAfter(group, mark)
	}
	return group
}

func (l *cacheImpl[K, V]) updateFrequency(element *linkedlist.Element[*node[K, V]]) {
	frequencyGroup := element.Value.frequencyListElement
	frequency := frequencyGroup.Value.frequency + 1
	currentFrequencyList := &frequencyGroup.Value.thisFrequencyNodes
	nextFrequencyGroup := frequencyGroup.Next()
	if nextFrequencyGroup == nil || nextFrequencyGroup.Value.frequency != frequency {
		if (*currentFrequencyList).Len() == 1 {
			frequencyGroup.Value.frequency = frequency
			return
		}
		nextFrequencyGroup = l.getFrequencyGroup(frequency, frequencyGroup)
	}
	nextFrequencyGroup.Value.thisFrequencyNodes.TransferBackElement(element)
	element.Value.frequencyListElement = nextFrequencyGroup
	if (*currentFrequencyList).Len() == 0 {
		l.freeFrequencyGroup.TransferBackElement(frequencyGroup)
	}
}

func (l *cacheImpl[K, V]) extractLastFrequency() *linkedlist.Element[*node[K, V]] {
	del := l.frequencyList.Front().Value.thisFrequencyNodes.Front()
	l.frequencyList.Front().Value.thisFrequencyNodes.Remove(del)
	delete(l.keyToElement, del.Value.key)
	if l.frequencyList.Front().Value.thisFrequencyNodes.Len() == 0 {
		l.freeFrequencyGroup.TransferBackElement(l.frequencyList.Front())
	}
	return del
}

func (l *cacheImpl[K, V]) getStandardFrequencyElement() *linkedlist.Element[frequencyGroup[K, V]] {
	if l.frequencyList.Front() == nil || l.frequencyList.Front().Value.frequency != 1 {
		return l.getFrequencyGroup(1, nil)
	}
	return l.frequencyList.Front()
}

func (l *cacheImpl[K, V]) GetKeyFrequency(key K) (int, error) {
	if link, ok := l.keyToElement[key]; ok {
		return link.Value.frequencyListElement.Value.frequency, nil
	}
	return 0, ErrKeyNotFound
}

func (l *cacheImpl[K, V]) Size() int {
	return len(l.keyToElement)
}

func (l *cacheImpl[K, V]) Capacity() int {
	return l.capacity
}

func (l *cacheImpl[K, V]) Get(key K) (V, error) {
	if link, ok := l.keyToElement[key]; ok {
		res := link.Value.value
		l.updateFrequency(link)
		return res, nil
	}
	return l.defaultValue, ErrKeyNotFound
}

func (l *cacheImpl[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for group := range l.frequencyList.Backward() {
			for element := range group.thisFrequencyNodes.Backward() {
				if !yield(element.key, element.value) {
					return
				}
			}
		}
	}
}

func (l *cacheImpl[K, V]) Put(key K, value V) {
	if element, ok := l.keyToElement[key]; ok {
		element.Value.value = value
		l.updateFrequency(element)
		return
	}
	if l.Size() == l.capacity {
		element := l.extractLastFrequency()
		group := l.getStandardFrequencyElement()
		node := element.Value
		node.key = key
		node.value = value
		node.frequencyListElement = group
		l.keyToElement[key] = element
		group.Value.thisFrequencyNodes.TransferBackElement(element)
		return
	}
	group := l.getStandardFrequencyElement()
	n := &node[K, V]{
		key:                  key,
		value:                value,
		frequencyListElement: group,
	}
	l.keyToElement[key] = group.Value.thisFrequencyNodes.PushBack(n)
}

func (l *cacheImpl[K, V]) GetOrLoad[T any](
	key K,
	loader func(K) (T, error),
	convert func(T) (V, error),
) (V, error) {
	if value, err := l.Get(key); err == nil {
		return value, nil
	}
	data, err := loader(key)
	if err != nil {
		return l.defaultValue, err
	}
	value, err := convert(data)
	if err != nil {
		return l.defaultValue, err
	}
	l.Put(key, value)
	return value, nil
}

func (l *cacheImpl[K, V]) mergeInsert(key K, value V, frequency int,
	frequencyCache *map[int]*linkedlist.Element[frequencyGroup[K, V]]) {
	group, ok := (*frequencyCache)[frequency]
	if !ok {
		var mark *linkedlist.Element[frequencyGroup[K, V]]
		for cursor := l.frequencyList.Front(); cursor != nil && cursor.Value.frequency < frequency; cursor = cursor.Next() {
			mark = cursor
		}
		group = l.getFrequencyGroup(frequency, mark)
		(*frequencyCache)[frequency] = group
	}
	n := &node[K, V]{
		key:                  key,
		value:                value,
		frequencyListElement: group,
	}
	l.keyToElement[key] = group.Value.thisFrequencyNodes.PushFront(n)
}

func (l *cacheImpl[K, V]) Merge[K2 comparable, V2 any](cache2 *cacheImpl[K2, V2],
	transform func(K2, V2) (K, V)) (*cacheImpl[K, V], error) {
	resultCache := New[K, V](l.Capacity() + cache2.Capacity())
	type translation struct {
		translatedValue V
		frequency       int
	}
	translatedSecond := make(map[K]translation, cache2.Size())
	keyTranslation := make(map[K2]K, cache2.Size())
	frequencyCache := make(map[int]*linkedlist.Element[frequencyGroup[K, V]], cache2.Size()+l.Size())
	for key, value := range cache2.All() {
		frequency, _ := cache2.GetKeyFrequency(key)
		translatedKey, translatedValue := transform(key, value)
		if _, ok := translatedSecond[translatedKey]; ok {
			return nil, ErrMergeTransformNotInjective
		}
		translatedSecond[translatedKey] = translation{
			translatedValue: translatedValue,
			frequency:       frequency,
		}
		keyTranslation[key] = translatedKey
	}
	for key, value := range l.All() {
		frequency, _ := l.GetKeyFrequency(key)
		if secondCacheInfo, ok := translatedSecond[key]; ok {
			value = secondCacheInfo.translatedValue
			frequency += secondCacheInfo.frequency

			delete(translatedSecond, key)
		}
		resultCache.mergeInsert(key, value, frequency, &frequencyCache)
	}
	for key := range cache2.All() {
		translatedKey := keyTranslation[key]
		if cachedInfo, ok := translatedSecond[translatedKey]; ok {
			resultCache.mergeInsert(translatedKey, cachedInfo.translatedValue, cachedInfo.frequency, &frequencyCache)
		}
	}
	return resultCache, nil
}
