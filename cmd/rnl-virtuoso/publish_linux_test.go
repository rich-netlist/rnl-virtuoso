package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishCompleteBundleAndRefuseExistingDestinations(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	mkdir(stage)
	writeText(filepath.Join(stage, "top.scs"), "complete")
	output := filepath.Join(root, "output")
	if e := publish(stage, output); e != nil {
		t.Fatal(e)
	}
	if readText(filepath.Join(output, "top.scs")) != "complete" {
		t.Fatal("Missing bundle")
	}
	for _, kind := range []string{"file", "empty-directory", "nonempty-directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			stage := filepath.Join(t.TempDir(), "stage")
			mkdir(stage)
			writeText(filepath.Join(stage, "new"), "new")
			target := filepath.Join(t.TempDir(), "existing")
			switch kind {
			case "file":
				writeText(target, "original")
			case "empty-directory":
				mkdir(target)
			case "nonempty-directory":
				mkdir(target)
				writeText(filepath.Join(target, "precious"), "original")
			case "symlink":
				if e := os.Symlink(output, target); e != nil {
					t.Fatal(e)
				}
			}
			if e := publish(stage, target); e == nil {
				t.Fatal("Replaced an existing destination")
			}
			if !isFile(filepath.Join(stage, "new")) {
				t.Fatal("Staging changed on refusal")
			}
			if kind == "file" && readText(target) != "original" {
				t.Fatal("File changed")
			}
			if kind == "nonempty-directory" && readText(filepath.Join(target, "precious")) != "original" {
				t.Fatal("Contents changed")
			}
		})
	}
}
