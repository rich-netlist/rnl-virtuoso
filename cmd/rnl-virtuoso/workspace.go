package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Native netlisting builds unquoted shell commands from its working path.
// Reject these paths before creating anything, rather than writing elsewhere.
func validateNativeOutputPath(path string) error {
	for _, ch := range path {
		if ch == '\\' && filepath.Separator == '\\' {
			continue
		} // Host-only parser tests; execution requires Linux.
		if ch <= 32 || ch == 127 || strings.ContainsRune("\"'`$\\;&|<>()*?[]{}!", ch) {
			return fmt.Errorf("native export paths cannot contain spaces, control characters or shell metacharacters: %s; choose a plain output path", path)
		}
	}
	return nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// The caller exclusively reserves the output before any export writes. A worker
// may only publish into that root, with requests and scratch under .rnl/work.
func validateExportWorkspace(output, work string) {
	if e := validateNativeOutputPath(output); e != nil {
		fail("%s", e)
	}
	if filepath.Clean(work) != filepath.Join(output, ".rnl", "work") {
		fail("Output already exists or request is outside its export workspace: %s; start a new export with the current SKILL entry point", output)
	}
	for _, p := range []string{output, filepath.Join(output, ".rnl"), work} {
		s, err := os.Lstat(p)
		if err != nil || !s.IsDir() || s.Mode()&os.ModeSymlink != 0 {
			fail("Missing or invalid export workspace directory: %s", p)
		}
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		fail("%s", err)
	}
	for _, entry := range entries {
		if entry.Name() != ".rnl" {
			fail("Output already exists: %s", filepath.Join(output, entry.Name()))
		}
	}
}

// Files become visible without replacing anything. The report is published last
// so an interrupted/failed export cannot appear to be a completed bundle.
func publishExport(staging, output, work, report string) error {
	validateExportWorkspace(output, work)
	if !pathWithin(work, staging) || staging == work {
		return fmt.Errorf("staging is outside export work directory")
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return err
	}
	if !isFile(filepath.Join(staging, report)) {
		return fmt.Errorf("missing completion report")
	}
	for _, entry := range entries {
		if entry.Name() == ".rnl" || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("reserved or invalid output entry: %s", entry.Name())
		}
		if _, e := os.Lstat(filepath.Join(output, entry.Name())); !os.IsNotExist(e) {
			return fmt.Errorf("output entry already exists: %s", entry.Name())
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name() == report {
			return false
		}
		if entries[j].Name() == report {
			return true
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		src, dst := filepath.Join(staging, entry.Name()), filepath.Join(output, entry.Name())
		if entry.IsDir() {
			err = publish(src, dst)
		} else {
			err = os.Link(src, dst)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func workspaceEnvironment(env []string, temp, logs string) []string {
	out := []string{}
	for _, value := range env {
		key, _, _ := strings.Cut(value, "=")
		if key != "TMPDIR" && key != "TMP" && key != "TEMP" && key != "CDS_LOG_PATH" {
			out = append(out, value)
		}
	}
	return append(out, "TMPDIR="+temp, "TMP="+temp, "TEMP="+temp, "CDS_LOG_PATH="+logs)
}
