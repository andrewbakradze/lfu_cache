package linkedlist

import "iter"

type List[T any] interface {
	Len() int
	Front() *Element[T]
	Back() *Element[T]
	PushFront(v T) *Element[T]
	PushBack(v T) *Element[T]
	PushBackList(other *listImpl[T])
	PushFrontList(other *listImpl[T])
	InsertBefore(v T, mark *Element[T]) *Element[T]
	InsertAfter(v T, mark *Element[T]) *Element[T]
	Remove(e *Element[T]) T
	MoveToFront(e *Element[T])
	MoveToBack(e *Element[T])
	MoveBefore(e, mark *Element[T])
	MoveAfter(e, mark *Element[T])
	TransferBackElement(e *Element[T])
	All() iter.Seq[T]
	Backward() iter.Seq[T]
}

type listImpl[T any] struct {
	root Element[T]
	len  int
}

func New[T any]() List[T] {
	return new(listImpl[T]).Init()
}

type Element[T any] struct {
	next, prev *Element[T]
	list       *listImpl[T]
	Value      T
}

func (e *Element[T]) Next() *Element[T] {
	if p := e.next; e.list != nil && p != &e.list.root {
		return p
	}
	return nil
}

func (e *Element[T]) Prev() *Element[T] {
	if p := e.prev; e.list != nil && p != &e.list.root {
		return p
	}
	return nil
}

func (l *listImpl[T]) Init() *listImpl[T] {
	l.root.next = &l.root
	l.root.prev = &l.root
	l.len = 0
	return l
}

func (l *listImpl[T]) Len() int { return l.len }

func (l *listImpl[T]) Front() *Element[T] {
	if l.len == 0 {
		return nil
	}
	return l.root.next
}

func (l *listImpl[T]) Back() *Element[T] {
	if l.len == 0 {
		return nil
	}
	return l.root.prev
}

func (l *listImpl[T]) insert(e, at *Element[T]) *Element[T] {
	e.prev = at
	e.next = at.next
	e.prev.next = e
	e.next.prev = e
	e.list = l
	l.len++
	return e
}

func (l *listImpl[T]) insertValue(v T, at *Element[T]) *Element[T] {
	return l.insert(&Element[T]{Value: v}, at)
}

func (l *listImpl[T]) remove(e *Element[T]) {
	e.prev.next = e.next
	e.next.prev = e.prev
	e.next = nil
	e.prev = nil
	e.list = nil
	l.len--
}

func (l *listImpl[T]) move(e, at *Element[T]) {
	if e == at {
		return
	}
	l.remove(e)
	l.insert(e, at)
}

func (l *listImpl[T]) Remove(e *Element[T]) T {
	if e.list == l {
		l.remove(e)
	}
	return e.Value
}

func (l *listImpl[T]) PushFront(v T) *Element[T] {
	return l.insertValue(v, &l.root)
}

func (l *listImpl[T]) PushBack(v T) *Element[T] {
	return l.insertValue(v, l.root.prev)
}

func (l *listImpl[T]) InsertBefore(v T, mark *Element[T]) *Element[T] {
	if mark.list != l {
		return nil
	}
	return l.insertValue(v, mark.prev)
}

func (l *listImpl[T]) InsertAfter(v T, mark *Element[T]) *Element[T] {
	if mark.list != l {
		return nil
	}
	return l.insertValue(v, mark)
}

func (l *listImpl[T]) MoveToFront(e *Element[T]) {
	if e.list != l || l.root.next == e {
		return
	}
	l.move(e, &l.root)
}

func (l *listImpl[T]) MoveToBack(e *Element[T]) {
	if e.list != l || l.root.prev == e {
		return
	}
	l.move(e, l.root.prev)
}

func (l *listImpl[T]) MoveBefore(e, mark *Element[T]) {
	if e.list != l || e == mark || mark.list != l {
		return
	}
	l.move(e, mark.prev)
}

func (l *listImpl[T]) MoveAfter(e, mark *Element[T]) {
	if e.list != l || e == mark || mark.list != l {
		return
	}
	l.move(e, mark)
}

func (l *listImpl[T]) PushBackList(other *listImpl[T]) {
	for i, e := other.Len(), other.Front(); i > 0; i, e = i-1, e.Next() {
		l.insertValue(e.Value, l.root.prev)
	}
}

func (l *listImpl[T]) PushFrontList(other *listImpl[T]) {
	for i, e := other.Len(), other.Back(); i > 0; i, e = i-1, e.Prev() {
		l.insertValue(e.Value, &l.root)
	}
}

func (l *listImpl[T]) TransferBackElement(e *Element[T]) {
	if e.list != nil {
		e.list.remove(e)
	}
	l.insert(e, l.root.prev)
}

func (l *listImpl[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for cur := l.Front(); cur != nil; cur = cur.Next() {
			if !yield(cur.Value) {
				return
			}
		}
	}
}

func (l *listImpl[T]) Backward() iter.Seq[T] {
	return func(yield func(T) bool) {
		for cur := l.Back(); cur != nil; cur = cur.Prev() {
			if !yield(cur.Value) {
				return
			}
		}
	}
}
