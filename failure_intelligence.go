package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type FailureImpact struct {
	RootService      string
	DirectDependents []string
	Indirect         []string
	AllAffected      []string
	Paths            map[string][]string
}

type Incident struct {
	ID               string              `json:"id"`
	Timestamp        time.Time           `json:"timestamp"`
	RootService      string              `json:"root_service"`
	Error            string              `json:"error"`
	DirectDependents []string            `json:"direct_dependents"`
	Indirect         []string            `json:"indirect"`
	AffectedServices []string            `json:"affected_services"`
	Paths            map[string][]string `json:"paths"`
}

func BuildFailureImpact(services map[string]Service, root string) (FailureImpact, error) {
	if _, ok := services[root]; !ok {
		return FailureImpact{}, fmt.Errorf("unknown service %q", root)
	}

	reverse := make(map[string][]string)

	for name, service := range services {
		for _, dep := range service.DependsOn {
			reverse[dep] = append(reverse[dep], name)
		}
	}

	for name := range reverse {
		sort.Strings(reverse[name])
	}

	direct := append([]string(nil), reverse[root]...)
	sort.Strings(direct)

	paths := make(map[string][]string)
	visited := map[string]bool{root: true}

	type item struct {
		name string
		path []string
	}

	queue := make([]item, 0)

	for _, dependent := range direct {
		path := []string{root, dependent}
		queue = append(queue, item{name: dependent, path: path})
		visited[dependent] = true
		paths[dependent] = path
	}

	var indirect []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, next := range reverse[current.name] {
			if visited[next] {
				continue
			}

			visited[next] = true

			path := append(append([]string(nil), current.path...), next)
			paths[next] = path
			indirect = append(indirect, next)

			queue = append(queue, item{
				name: next,
				path: path,
			})
		}
	}

	sort.Strings(indirect)

	all := make([]string, 0, len(direct)+len(indirect))
	all = append(all, direct...)
	all = append(all, indirect...)
	sort.Strings(all)

	return FailureImpact{
		RootService:      root,
		DirectDependents: direct,
		Indirect:         indirect,
		AllAffected:      all,
		Paths:            paths,
	}, nil
}

func FormatFailureImpact(impact FailureImpact) string {
	var b strings.Builder

	fmt.Fprintf(&b, "ROOT FAILURE\n")
	fmt.Fprintf(&b, "  %s\n\n", impact.RootService)

	fmt.Fprintf(&b, "DIRECT DEPENDENTS\n")
	if len(impact.DirectDependents) == 0 {
		fmt.Fprintf(&b, "  none\n")
	} else {
		for _, name := range impact.DirectDependents {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}

	fmt.Fprintf(&b, "\nINDIRECT DEPENDENTS\n")
	if len(impact.Indirect) == 0 {
		fmt.Fprintf(&b, "  none\n")
	} else {
		for _, name := range impact.Indirect {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}

	fmt.Fprintf(&b, "\nTOTAL AFFECTED: %d\n", len(impact.AllAffected))

	if len(impact.Paths) > 0 {
		fmt.Fprintf(&b, "\nDEPENDENCY PATHS\n")

		names := make([]string, 0, len(impact.Paths))
		for name := range impact.Paths {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			fmt.Fprintf(&b, "  %s\n", strings.Join(impact.Paths[name], " -> "))
		}
	}

	return b.String()
}
