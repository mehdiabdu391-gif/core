package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

func attachService() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: core attach NAME /absolute/path/to/executable")
		return
	}

	name := os.Args[2]
	path := os.Args[3]

	if !validServiceName(name) {
		fmt.Printf("Invalid service name %q\n", name)
		return
	}

	if !filepath.IsAbs(path) {
		fmt.Println("Service path must be absolute")
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("Unable to access executable: %v\n", err)
		return
	}

	if info.IsDir() {
		fmt.Printf("Service path is a directory: %s\n", path)
		return
	}

	if info.Mode()&0111 == 0 {
		fmt.Printf("Service is not executable: %s\n", path)
		return
	}

	cfg := Config{
		Services: make(map[string]ServiceConfig),
	}

	if data, err := os.ReadFile("core.yaml"); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			fmt.Printf("Unable to read core.yaml: %v\n", err)
			return
		}
		if cfg.Services == nil {
			cfg.Services = make(map[string]ServiceConfig)
		}
	} else if !os.IsNotExist(err) {
		fmt.Printf("Unable to read core.yaml: %v\n", err)
		return
	}

	if _, exists := cfg.Services[name]; exists {
		fmt.Printf("Service %q already exists\n", name)
		return
	}

	cfg.Services[name] = ServiceConfig{
		Command: path,
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		fmt.Printf("Unable to write configuration: %v\n", err)
		return
	}

	if err := os.WriteFile("core.yaml", data, 0600); err != nil {
		fmt.Printf("Unable to write core.yaml: %v\n", err)
		return
	}

	fmt.Printf("Attached service %q\n", name)
}
