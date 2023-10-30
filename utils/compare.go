package utils

type compareItem struct {
	old bool
	new bool
}

func Compare[T comparable](old, new []T) ([]T, []T) {
	m := make(map[T]compareItem)
	for _, v := range old {
		m[v] = compareItem{old: true}
	}
	for _, v := range new {
		item, ok := m[v]
		if ok {
			item.new = true
			m[v] = item
		} else {
			m[v] = compareItem{new: true}
		}
	}
	added := make([]T, 0, len(new))
	removed := make([]T, 0, len(old))
	for k, v := range m {
		if v.old && !v.new {
			removed = append(removed, k)
		} else if !v.old && v.new {
			added = append(added, k)
		}
	}
	return added, removed
}
