package main

import (
	"fmt"
	"sort"
	"strings"
)

type MinimumCut struct {
	Target   string
	Services []string
	Size     int
	Roots    []string
	Isolated bool
}

func AnalyzeMinimumCut(services map[string]Service, target string) (MinimumCut, error) {
	if _, ok := services[target]; !ok {
		return MinimumCut{}, fmt.Errorf("unknown service %q", target)
	}

	roots := dependencyRoots(services)

	if len(roots) == 0 {
		return MinimumCut{
			Target:   target,
			Isolated: true,
		}, nil
	}

	if len(services[target].DependsOn) == 0 {
		return MinimumCut{
			Target:   target,
			Roots:    roots,
			Isolated: true,
		}, nil
	}

	graph := buildCutGraph(services, target, roots)
	maxFlow(graph, target, roots)

	cut := extractMinimumCut(graph, target)

	sort.Strings(cut)

	return MinimumCut{
		Target:   target,
		Services: cut,
		Size:     len(cut),
		Roots:    roots,
		Isolated: len(cut) == 0,
	}, nil
}

func dependencyRoots(services map[string]Service) []string {
	var roots []string

	for name, service := range services {
		if len(service.DependsOn) == 0 {
			roots = append(roots, name)
		}
	}

	sort.Strings(roots)
	return roots
}

type cutEdge struct {
	to       int
	rev      int
	capacity int
	original int
}

type cutGraph struct {
	nodes int
	edges [][]cutEdge
	names []string
	index map[string]int
}

func buildCutGraph(services map[string]Service, target string, roots []string) *cutGraph {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	index := make(map[string]int, len(names))
	for i, name := range names {
		index[name] = i
	}

	g := &cutGraph{
		nodes: len(names) * 2,
		edges: make([][]cutEdge, len(names)*2),
		names: names,
		index: index,
	}

	const infinite = int(^uint(0) >> 1)

	for _, name := range names {
		inCap := 1
		if name == target {
			inCap = infinite
		}

		addCutEdge(g, nodeIn(index[name]), nodeOut(index[name]), inCap)

		for _, dep := range services[name].DependsOn {
			addCutEdge(
				g,
				nodeOut(index[name]),
				nodeIn(index[dep]),
				infinite,
			)
		}
	}

	_ = roots

	return g
}

func nodeIn(i int) int {
	return i * 2
}

func nodeOut(i int) int {
	return i*2 + 1
}

func addCutEdge(g *cutGraph, from, to, capacity int) {
	forward := cutEdge{
		to:       to,
		rev:      len(g.edges[to]),
		capacity: capacity,
		original: capacity,
	}

	backward := cutEdge{
		to:       from,
		rev:      len(g.edges[from]),
		capacity: 0,
		original: 0,
	}

	g.edges[from] = append(g.edges[from], forward)
	g.edges[to] = append(g.edges[to], backward)
}

func maxFlow(g *cutGraph, target string, roots []string) int {
	source := nodeOut(g.index[target])
	sink := len(g.edges)

	g.edges = append(g.edges, nil)

	const infinite = int(^uint(0) >> 1)

	for _, root := range roots {
		if root == target {
			continue
		}

		addCutEdge(
			g,
			nodeIn(g.index[root]),
			sink,
			infinite,
		)
	}

	flow := 0

	for {
		parentNode := make([]int, len(g.edges))
		parentEdge := make([]int, len(g.edges))

		for i := range parentNode {
			parentNode[i] = -1
		}

		queue := []int{source}
		parentNode[source] = source

		for len(queue) > 0 && parentNode[sink] == -1 {
			current := queue[0]
			queue = queue[1:]

			for i, edge := range g.edges[current] {
				if edge.capacity <= 0 {
					continue
				}

				if parentNode[edge.to] != -1 {
					continue
				}

				parentNode[edge.to] = current
				parentEdge[edge.to] = i
				queue = append(queue, edge.to)

				if edge.to == sink {
					break
				}
			}
		}

		if parentNode[sink] == -1 {
			break
		}

		bottleneck := infinite
		for current := sink; current != source; {
			prev := parentNode[current]
			edgeIndex := parentEdge[current]

			if g.edges[prev][edgeIndex].capacity < bottleneck {
				bottleneck = g.edges[prev][edgeIndex].capacity
			}

			current = prev
		}

		for current := sink; current != source; {
			prev := parentNode[current]
			edgeIndex := parentEdge[current]

			reverse := g.edges[prev][edgeIndex].rev

			g.edges[prev][edgeIndex].capacity -= bottleneck
			g.edges[current][reverse].capacity += bottleneck

			current = prev
		}

		flow += bottleneck
	}

	return flow
}

func extractMinimumCut(g *cutGraph, target string) []string {
	source := nodeOut(g.index[target])

	reachable := make([]bool, len(g.edges))
	queue := []int{source}
	reachable[source] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range g.edges[current] {
			if edge.capacity <= 0 || reachable[edge.to] {
				continue
			}

			reachable[edge.to] = true
			queue = append(queue, edge.to)
		}
	}

	var cut []string

	for i, name := range g.names {
		if name == target {
			continue
		}

		in := nodeIn(i)
		out := nodeOut(i)

		if reachable[in] && !reachable[out] {
			cut = append(cut, name)
		}
	}

	return cut
}

func FormatMinimumCut(c MinimumCut) string {
	var b strings.Builder

	fmt.Fprintln(&b, "MINIMUM FAILURE CUT")
	fmt.Fprintln(&b, "===================")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "TARGET")
	fmt.Fprintf(&b, "  %s\n\n", c.Target)

	if c.Isolated {
		fmt.Fprintln(&b, "RESULT")
		fmt.Fprintln(&b, "  No dependency cut required.")
		return b.String()
	}

	fmt.Fprintln(&b, "MINIMUM CUT")
	fmt.Fprintf(&b, "  %d service(s)\n", c.Size)

	for _, name := range c.Services {
		fmt.Fprintf(&b, "  - %s\n", name)
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "DEPENDENCY ROOTS")

	if len(c.Roots) == 0 {
		fmt.Fprintln(&b, "  none")
	} else {
		for _, root := range c.Roots {
			fmt.Fprintf(&b, "  - %s\n", root)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "INTERPRETATION")

	switch c.Size {
	case 1:
		fmt.Fprintln(&b, "  A single service failure can sever the target from its dependency roots.")
	default:
		fmt.Fprintf(&b, "  %d independent service failures are required to sever the target from its dependency roots.\n", c.Size)
	}

	return b.String()
}
