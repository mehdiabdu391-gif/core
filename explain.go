package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func explain() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: core explain SERVICE")
		return
	}

	services, err := DetectServices()
	if err != nil {
		fatal(err)
	}

	serviceMap := make(map[string]Service, len(services))
	for _, s := range services {
		serviceMap[s.Name] = s
	}

	fmt.Print(explainService(os.Args[2], serviceMap))
}

func explainService(name string, services map[string]Service) string {
	service, ok := services[name]
	if !ok {
		return fmt.Sprintf("Service %q not found.\n", name)
	}

	var out strings.Builder

	out.WriteString("\nCORE EXPLAIN\n")
	out.WriteString("============\n\n")

	fmt.Fprintf(&out, "SERVICE: %s\n", service.Name)
	fmt.Fprintf(&out, "COMMAND: %s", service.Command)

	if len(service.Args) > 0 {
		fmt.Fprintf(&out, " %s", strings.Join(service.Args, " "))
	}

	out.WriteString("\n")

	if len(service.DependsOn) == 0 {
		out.WriteString("DEPENDS ON: none\n")
	} else {
		deps := append([]string(nil), service.DependsOn...)
		sort.Strings(deps)
		fmt.Fprintf(&out, "DEPENDS ON: %s\n", strings.Join(deps, ", "))
	}

	affected := dependentServices(name, services)

	if len(affected) == 0 {
		out.WriteString("AFFECTS: none\n")
	} else {
		sort.Strings(affected)
		fmt.Fprintf(&out, "AFFECTS: %s\n", strings.Join(affected, ", "))
	}

	return out.String()
}

func dependentServices(name string, services map[string]Service) []string {
	seen := make(map[string]bool)
	var affected []string

	var visit func(string)

	visit = func(target string) {
		for serviceName, service := range services {
			if seen[serviceName] {
				continue
			}

			for _, dep := range service.DependsOn {
				if dep == target {
					seen[serviceName] = true
					affected = append(affected, serviceName)
					visit(serviceName)
					break
				}
			}
		}
	}

	visit(name)
	return affected
}
