package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkspaceEnvironment(t *testing.T) {
	t.Setenv("TMPDIR", "/parent")
	env := workspaceEnvironment([]string{"TMPDIR=/old", "TEMP=/old", "TMP=/old", "CDS_LOG_PATH=/old", "PWD=/project", "PDK=keep"}, "/output/.rnl/tmp", "/output/.rnl/logs")
	got := map[string]string{}
	for _, value := range env {
		key, v, _ := strings.Cut(value, "=")
		if _, ok := got[key]; ok {
			t.Fatal("duplicate env key", key)
		}
		got[key] = v
	}
	if got["TMPDIR"] != "/output/.rnl/tmp" || got["TMP"] != got["TMPDIR"] || got["TEMP"] != got["TMPDIR"] || got["CDS_LOG_PATH"] != "/output/.rnl/logs" || got["PWD"] != "/project" || got["PDK"] != "keep" {
		t.Fatal(got)
	}
	if os.Getenv("TMPDIR") != "/parent" {
		t.Fatal("changed parent environment")
	}
}

func TestPublishInsideWorkspaceAndKeepLogs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native publication requires Linux")
	}
	output := filepath.Join(t.TempDir(), "out")
	work := filepath.Join(output, ".rnl", "work")
	if e := os.MkdirAll(work, 0700); e != nil {
		t.Fatal(e)
	}
	writeText(filepath.Join(work, "spectre.log"), "keep diagnostics")
	stage := filepath.Join(work, "stage")
	mkdir(stage)
	mkdir(filepath.Join(stage, "foreign"))
	writeText(filepath.Join(stage, "foreign", "child.scs"), "child")
	writeText(filepath.Join(stage, "top.scs"), "top")
	writeText(filepath.Join(stage, "export-report.json"), "{}")
	if e := publishExport(stage, output, work, "export-report.json"); e != nil {
		t.Fatal(e)
	}
	if readText(filepath.Join(work, "spectre.log")) != "keep diagnostics" || readText(filepath.Join(output, "foreign", "child.scs")) != "child" {
		t.Fatal("lost outputs or diagnostics")
	}
	rejects(t, "already exists", func() { validateExportWorkspace(output, work) })
}

func TestWorkspaceRefusesCollisionsAndEscapes(t *testing.T) {
	output := filepath.Join(t.TempDir(), "out")
	work := filepath.Join(output, ".rnl", "work")
	if e := os.MkdirAll(work, 0700); e != nil {
		t.Fatal(e)
	}
	rejects(t, "outside", func() { validateExportWorkspace(output, filepath.Dir(output)) })
	stage := filepath.Join(work, "stage")
	mkdir(stage)
	writeText(filepath.Join(stage, "export-report.json"), "{}")
	mkdir(filepath.Join(stage, ".rnl"))
	if e := publishExport(stage, output, work, "export-report.json"); e == nil {
		t.Fatal("reserved metadata collision accepted")
	}
	if isFile(filepath.Join(output, "export-report.json")) {
		t.Fatal("completion report written on failure")
	}
	if runtime.GOOS == "linux" {
		alias := filepath.Join(t.TempDir(), "alias")
		if e := os.Symlink(output, alias); e != nil {
			t.Fatal(e)
		}
		rejects(t, "invalid", func() { validateExportWorkspace(alias, filepath.Join(alias, ".rnl", "work")) })
	}
}

func TestNativeOutputPathRejectsShellExpansion(t *testing.T) {
	if runtime.GOOS == "linux" && validateNativeOutputPath("/output/a\\b") == nil {
		t.Fatal("accepted backslash expansion")
	}
	for _, name := range []string{"with space", "a\tb", "a;b", "a$b", "a`b", "a*b", "a?b", "a'b", "a\"b", "a(b)", "a[b]", "a&b", "a|b", "a>b"} {
		if validateNativeOutputPath(filepath.Join("/output", name)) == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if e := validateNativeOutputPath("/output/user-1/design_v2.0/.rnl/work"); e != nil {
		t.Fatal(e)
	}
}
