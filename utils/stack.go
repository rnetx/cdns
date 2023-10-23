package utils

type Stack[T any] struct {
	data []T
}

func NewStack[T any](size int) *Stack[T] {
	s := &Stack[T]{}
	if size > 0 {
		s.data = make([]T, 0, size)
	}
	return s
}

func (s *Stack[T]) Push(v T) {
	s.data = append(s.data, v)
}

func (s *Stack[T]) Pop() T {
	if len(s.data) == 0 {
		var v T
		return v
	}
	v := s.data[len(s.data)-1]
	s.data = s.data[:len(s.data)-1]
	return v
}

func (s *Stack[T]) Peek() T {
	if len(s.data) == 0 {
		var v T
		return v
	}
	return s.data[len(s.data)-1]
}

func (s *Stack[T]) Len() int {
	return len(s.data)
}
