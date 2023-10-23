package core

import (
	"fmt"

	"github.com/rnetx/cdns/adapter"
	"github.com/rnetx/cdns/utils"
)

func sortUpstream(upstreams []adapter.Upstream) ([]adapter.Upstream, error) {
	nodeMap := make(map[string]*utils.GraphNode[adapter.Upstream])
	for _, u := range upstreams {
		nodeMap[u.Tag()] = utils.NewGraphNode(u)
	}
	for _, u := range upstreams {
		node := nodeMap[u.Tag()]
		dependencies := u.Dependencies()
		if dependencies != nil && len(dependencies) > 0 {
			for _, tag := range dependencies {
				dpNode, ok := nodeMap[tag]
				if !ok {
					return nil, fmt.Errorf("upstream [%s] depend on upstream [%s], but upstream [%s] not found", u.Tag(), tag, tag)
				}
				dpNode.AddNext(node)
				node.AddPrev(dpNode)
			}
		}
	}
	q := utils.NewQueue[string]()
	for _, u := range upstreams {
		node := nodeMap[u.Tag()]
		if !node.HasPrev() {
			q.Push(u.Tag())
		}
	}
	if q.Len() == 0 {
		// Circle
		target := upstreams[0]
		links := make([]string, 0)
		links = append(links, target.Tag())
		findCircle(nodeMap, target, target, &links)
		return nil, fmt.Errorf("circle dependencies: %s", circleStr(links))
	}
	sorted := make([]adapter.Upstream, 0, len(upstreams))
	for q.Len() > 0 {
		data := q.Pop()
		node := nodeMap[data]
		sorted = append(sorted, node.Data())
		delete(nodeMap, data)
		for _, next := range node.Next() {
			next.RemovePrev(node)
			if !next.HasPrev() {
				q.Push(next.Data().Tag())
			}
		}
	}
	if len(sorted) < len(upstreams) {
		// Circle
		var target adapter.Upstream
		for _, v := range nodeMap {
			target = v.Data()
			break
		}
		links := make([]string, 0)
		links = append(links, target.Tag())
		findCircle(nodeMap, target, target, &links)
		return nil, fmt.Errorf("circle dependencies: %s", circleStr(links))
	}
	return sorted, nil
}

func findCircle(nodeMap map[string]*utils.GraphNode[adapter.Upstream], target adapter.Upstream, now adapter.Upstream, links *[]string) {
	nowNode := nodeMap[now.Tag()]
	for _, next := range nowNode.Next() {
		if next.Data() == now {
			return
		}
		var linksCopy []string
		for _, v := range *links {
			linksCopy = append(linksCopy, v)
		}
		linksCopy = append(linksCopy, next.Data().Tag())
		findCircle(nodeMap, target, next.Data(), &linksCopy)
		if len(linksCopy) > 0 {
			*links = linksCopy
			return
		}
	}
	*links = nil
}

func circleStr(links []string) string {
	s := ""
	if len(links) > 0 {
		s += links[0]
		for i := 1; i < len(links); i++ {
			s += " -> " + links[i]
		}
		s += " -> " + links[0]
	}
	return s
}
