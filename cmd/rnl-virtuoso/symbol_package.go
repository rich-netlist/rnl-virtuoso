package main

import "sort"

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
	text := `@RNL(type="header") { @VERSION("0.3"); @PACKAGE(name=` + quote(packageName) + `, version="0.1") }` + "\n@RNL {\n"
	for _, key := range names {
		text += unique[key].Definition + ";\n"
	}
	return text + "}\n", names
}
