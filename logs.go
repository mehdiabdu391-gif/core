package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func showLogs(service string, follow bool) error {
	if err := ensureCoreDirs(); err != nil {
		return err
	}
	if service == "" {
		return showAllLogs(follow)
	}
	if !validServiceName(service) {
		return fmt.Errorf("invalid service name %q", service)
	}
	path := filepath.Join(logDir, service+".log")
	if _, err := os.Stat(path); os.IsNotExist(err) && !follow {
		fmt.Printf("No log file: %s\n", path)
		return nil
	}
	var offset int64
	for {
		next, err := printNewLines(path, offset, !follow)
		if err != nil {
			return err
		}
		offset = next
		if !follow {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func showAllLogs(follow bool) error {
	offsets := map[string]int64{}
	for {
		entries, err := os.ReadDir(logDir)
		if err != nil {
			return err
		}
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(logDir, name)
			if !follow {
				fmt.Printf("\n===== %s =====\n", name)
			}
			next, err := printNewLinesPrefixed(path, offsets[name], !follow, follow, name)
			if err != nil {
				return err
			}
			offsets[name] = next
		}
		if !follow {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func printNewLines(path string, offset int64, flushPartial bool) (int64, error) {
	return printNewLinesPrefixed(path, offset, flushPartial, false, "")
}

// printNewLinesPrefixed prints complete lines appended to path since offset and
// returns the offset just past the last byte printed. A trailing line without
// a newline is held back for the next poll unless flushPartial is set. If the
// file shrank (truncated or replaced), reading restarts from the beginning.
func printNewLinesPrefixed(path string, offset int64, flushPartial, prefix bool, name string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return offset, err
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() < offset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	r := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 && (err == nil || flushPartial) {
			if prefix {
				fmt.Printf("%s | ", name)
			}
			fmt.Print(line)
			if err != nil {
				fmt.Println()
			}
			offset += int64(len(line))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return offset, nil
			}
			return offset, err
		}
	}
}
