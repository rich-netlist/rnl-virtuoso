package main

import "sort"

// The version of the symbol packages an export generates. They move to the
// language 0.4 with this exporter, so their version moves past 0.1 too: a
// package of one name and version never has two contents.
const generatedPackageVersion = "0.2"

func reusableSymbolPackage(packageName string, symbols []librarySymbol) (string, []string) {
	unique := map[string]librarySymbol{}
	for _, symbol := range symbols {
		key := symbol.Library + "/" + symbol.Cell
		if old, ok := unique[key]; ok && old.Definition != symbol.Definition {
			fail("Conflicting symbol geometry for %s", key)
		}
		unique[key] = symbol
	}
	names := []string{}
	for key := range unique {
		names = append(names, key)
	}
	sort.Strings(names)
	text := `@RNL(type="header") { @VERSION("` + rnlVersion + `"); @PACKAGE(name=` + quote(packageName) + `, version="` + generatedPackageVersion + `") }` + "\n@RNL {\n"
	for _, key := range names {
		text += unique[key].Definition + ";\n"
	}
	return text + "}\n", names
}
