package utils

type GraphNode[T comparable] struct {
	data T
	prev map[T]*GraphNode[T]
	next map[T]*GraphNode[T]
}

func NewGraphNode[T comparable](data T) *GraphNode[T] {
	return &GraphNode[T]{
		data: data,
		prev: make(map[T]*GraphNode[T]),
		next: make(map[T]*GraphNode[T]),
	}
}

func (n *GraphNode[T]) Data() T {
	return n.data
}

func (n *GraphNode[T]) Prev() []*GraphNode[T] {
	nodes := make([]*GraphNode[T], 0, len(n.prev))
	for _, node := range n.prev {
		nodes = append(nodes, node)
	}
	return nodes
}

func (n *GraphNode[T]) Next() []*GraphNode[T] {
	nodes := make([]*GraphNode[T], 0, len(n.next))
	for _, node := range n.next {
		nodes = append(nodes, node)
	}
	return nodes
}

func (n *GraphNode[T]) AddPrev(node *GraphNode[T]) {
	n.prev[node.data] = node
}

func (n *GraphNode[T]) AddNext(node *GraphNode[T]) {
	n.next[node.data] = node
}

func (n *GraphNode[T]) RemovePrev(node *GraphNode[T]) {
	delete(n.prev, node.data)
}

func (n *GraphNode[T]) RemoveNext(node *GraphNode[T]) {
	delete(n.next, node.data)
}

func (n *GraphNode[T]) PrevMap() map[T]*GraphNode[T] {
	return n.prev
}

func (n *GraphNode[T]) NextMap() map[T]*GraphNode[T] {
	return n.next
}

func (n *GraphNode[T]) HasPrev() bool {
	return len(n.prev) > 0
}

func (n *GraphNode[T]) HasNext() bool {
	return len(n.next) > 0
}
