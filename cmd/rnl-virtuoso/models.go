package main

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// modelDirectory holds the copies of the Verilog-A files of an export.
const modelDirectory = "veriloga"

var ahdlIncludeLine = rx(`(?m)^(\s*ahdl_include\s+)"([^"\n]+)"([ \t\r]*)$`)

// modelFileName names the copy of a Verilog-A file. A Virtuoso veriloga view
// keeps its module in <cell>/<view>/veriloga.va, so such a file is named after
// its cell; any other file keeps its own name.
func modelFileName(source string) string {
	base := path.Base(filepath.ToSlash(source))
	if strings.EqualFold(base, "veriloga.va") {
		if cell := path.Base(path.Dir(path.Dir(filepath.ToSlash(source)))); cell != "/" && cell != "." && cell != "" {
			base = cell + ".va"
		}
	}
	return base
}

// bundleModelFiles copies the Verilog-A files named by the ahdl_include lines of
// the netlist files into veriloga/ of the export and points those lines at the
// copies, so the export is checked and simulated without the source machine. A
// relative reference is resolved against working; with no working directory it
// is left as it is. A file that cannot be read keeps its original reference and
// gives a note.
func bundleModelFiles(files map[string]string, working string) ([]map[string]string, []string) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	sources := stringSet{}
	for _, name := range names {
		for _, m := range ahdlIncludeLine.FindAllStringSubmatch(files[name], -1) {
			if p, ok := modelSource(m[2], working); ok {
				sources[p] = true
			}
		}
	}
	if len(sources) == 0 {
		return []map[string]string{}, []string{}
	}
	for _, name := range names {
		lower := strings.ToLower(name)
		if lower == modelDirectory || strings.HasPrefix(lower, modelDirectory+"/") {
			fail("Output %s is in %s/, the directory of the copied Verilog-A files", name, modelDirectory)
		}
	}
	ordered := make([]string, 0, len(sources))
	for p := range sources {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	copies, taken := map[string]string{}, stringSet{}
	report, notes := []map[string]string{}, []string{}
	for _, source := range ordered {
		text, e := os.ReadFile(source)
		if e != nil {
			notes = append(notes, "Verilog-A file "+source+" cannot be read; its ahdl_include keeps that path")
			continue
		}
		base := modelFileName(source)
		stem, extension := strings.TrimSuffix(base, path.Ext(base)), path.Ext(base)
		file := modelDirectory + "/" + base
		for n := 2; taken[strings.ToLower(file)]; n++ {
			file = modelDirectory + "/" + stem + "_" + strconv.Itoa(n) + extension
		}
		taken[strings.ToLower(file)] = true
		copies[source] = file
		files[file] = string(text)
		report = append(report, map[string]string{"source": source, "file": file})
	}
	for _, name := range names {
		files[name] = ahdlIncludeLine.ReplaceAllStringFunc(files[name], func(line string) string {
			m := ahdlIncludeLine.FindStringSubmatch(line)
			p, ok := modelSource(m[2], working)
			if !ok || copies[p] == "" {
				return line
			}
			return m[1] + quote(relativeNetlistPath(name, copies[p])) + m[3]
		})
	}
	return report, notes
}

// addModelFiles copies the Verilog-A files of an export that is about to be
// written, once, and records them in its report. Only the steps that publish an
// export call it: convert also assembles the files of a split cell export, and
// those are copied by the cell job that writes them.
func addModelFiles(files map[string]string, report map[string]any, working string) {
	copies, notes := bundleModelFiles(files, working)
	report["behavior_model_files"] = copies
	if len(notes) > 0 {
		all, _ := report["notes"].([]string)
		all = unique(append(all, notes...))
		sort.Strings(all)
		report["notes"] = all
	}
}

func modelSource(reference, working string) (string, bool) {
	if filepath.IsAbs(reference) || strings.HasPrefix(reference, "/") {
		return reference, true
	}
	if working == "" {
		return "", false
	}
	return filepath.Join(working, reference), true
}
