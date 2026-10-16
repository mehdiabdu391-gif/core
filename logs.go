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
	if !follow {
		// Older rotated files first, so the output reads in time order.
		for _, old := range rotatedLogs(path) {
			if _, err := printNewLines(old, 0, true); err != nil {
				return err
			}
		}
	}
	var t logTail
	for {
		if err := t.poll(path, !follow, false, ""); err != nil {
			return err
		}
		if !follow {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// logTail follows one log file across polls. It notices when the file is
// rotated or replaced (a different file now has that name) and when it is
// truncated, and starts the new file from the beginning in both cases.
type logTail struct {
	offset int64
	info   os.FileInfo
}

func (t *logTail) poll(path string, flushPartial, prefix bool, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.offset, t.info = 0, nil
			return nil
		}
		return err
	}
	if t.info != nil && !os.SameFile(t.info, info) {
		// Rotated: finish what was written to the old file before the
		// rename, then start the new one from the top.
		if old, err := os.Stat(rotatedLogName(path, 1)); err == nil && os.SameFile(t.info, old) {
			if _, err := printNewLinesPrefixed(rotatedLogName(path, 1), t.offset, true, prefix, name); err != nil {
				return err
			}
		}
		t.offset = 0
	}
	t.info = info
	next, err := printNewLinesPrefixed(path, t.offset, flushPartial, prefix, name)
	t.offset = next
	return err
}

func showAllLogs(follow bool) error {
	tails := map[string]*logTail{}
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
			t := tails[name]
			if t == nil {
				t = &logTail{}
				tails[name] = t
			}
			if err := t.poll(path, !follow, follow, name); err != nil {
				return err
			}
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
