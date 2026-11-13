package main

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// A symbol library is an RNL package of symbols that the exporter may assign to
// leaf instances instead of their EDIF symbols. Every library is handled alike;
// the bundled eda-symbol is only the default list's single entry.
//
// How a symbol corresponds to a Virtuoso cell is exporter configuration, so it
// is not written in RNL (RNL 0.4, 16.4): it lives in a mapping file beside the
// package, <package file without .rnl>.virtuoso.json. A library without one is
// matched by its own RNL: @DEVICE statements and model rules.

// libraryInput names a symbol library: a package file (Path or Text) and its
// optional mapping (beside Path, or Mapping).
type libraryInput struct {
	Path    string `json:"path"`
	File    string `json:"file"`
	Text    string `json:"text"`
	Mapping string `json:"mapping"`
}

type mappedCell struct {
	Library    string      `json:"library"`
	Cell       string      `json:"cell"`
	Symbol     string      `json:"symbol"`
	Terminals  []string    `json:"terminals"`
	Pins       [][2]string `json:"pins"`
	Correction string      `json:"package_to_source"`
	Align      []int       `json:"align_pins"`
}

type libraryMapping struct {
	Schema      int                        `json:"schema"`
	Package     map[string]string          `json:"package"`
	Description string                     `json:"description"`
	Ports       map[string]string          `json:"ports"`
	Cells       []mappedCell               `json:"cells"`
	Symbols     map[string]json.RawMessage `json:"symbols"`
	Terminals   map[string][]string        `json:"terminals"`
}

type modelRule struct {
	Model, Name, Language, Symbol string
	Terminals                     []string
}

type deviceNames struct {
	Model, Name, Language string
	Terminals             []string
}

type symbolLibrary struct {
	File, Name, Version, Text string
	Pins                      map[string]map[string]point
	Defined                   stringSet
	Mapping                   *libraryMapping
	Rules                     map[string]orientationRule
	Rule                      []modelRule
	Devices                   []deviceNames
}

// symbolChoice is the symbol that a library gives an instance.
type symbolChoice struct {
	Library   *symbolLibrary
	Rank      int
	Symbol    string
	Terminals []string // symbol terminal names in node order, "" where none is paired; nil binds by position
	Rule      orientationRule
	Mode      string
}

var packageHeader = rx(`@PACKAGE\(\s*name="([^"]+)"\s*,\s*version="([^"]+)"\s*\)`)

func loadSymbolLibrary(in libraryInput) *symbolLibrary {
	text, mapping, file := in.Text, in.Mapping, in.File
	if text == "" {
		if in.Path == "" {
			fail("A symbol library needs a package file")
		}
		text = readText(in.Path)
		if mapping == "" {
			if sidecar := strings.TrimSuffix(in.Path, ".rnl") + ".virtuoso.json"; isFile(sidecar) {
				mapping = readText(sidecar)
			}
		}
	}
	if file == "" {
		file = filepath.Base(in.Path)
	}
	if file == "" || file == "." || !strings.HasSuffix(file, ".rnl") || strings.ContainsAny(file, "/\\\":<>|?*") {
		fail("Unsafe symbol library filename %s", file)
	}
	m := packageHeader.FindStringSubmatch(text)
	if m == nil {
		fail("Symbol library %s is not an RNL package (no @PACKAGE)", file)
	}
	lib := &symbolLibrary{File: file, Name: m[1], Version: m[2], Text: text, Pins: packagePins(text), Defined: stringSet{}, Rules: map[string]orientationRule{}}
	for name := range lib.Pins {
		lib.Defined[name] = true
	}
	if strings.TrimSpace(mapping) != "" {
		lib.Mapping = loadMapping(mapping, lib)
	} else {
		lib.Rule, lib.Devices = packageRules(text)
	}
	return lib
}

func loadMapping(text string, lib *symbolLibrary) *libraryMapping {
	var m libraryMapping
	if e := json.Unmarshal([]byte(text), &m); e != nil {
		fail("Invalid symbol mapping for %s: %s", lib.File, e)
	}
	if m.Schema != 1 || m.Package["name"] != lib.Name || m.Package["version"] != lib.Version {
		fail("The symbol mapping of %s is not for package %s %s", lib.File, lib.Name, lib.Version)
	}
	if m.Symbols != nil {
		lib.Rules = loadRules(text)
	}
	for name, names := range m.Terminals {
		checkTerminalNames(name, names)
	}
	for _, c := range m.Cells {
		if c.Library == "" || c.Cell == "" || !lib.Defined[c.Symbol] {
			fail("Symbol mapping of %s names %s/%s with a symbol the package does not define: %s", lib.File, c.Library, c.Cell, c.Symbol)
		}
		checkTerminalNames(c.Symbol, c.Terminals)
		checkAlignPins(c.Library+"/"+c.Cell, c.Align)
		if c.Correction != "" {
			if _, ok := matrices[c.Correction]; !ok {
				fail("Invalid package orientation for %s/%s", c.Library, c.Cell)
			}
		}
	}
	for kind, name := range m.Ports {
		if (kind != "input" && kind != "output") || !lib.Defined[name] {
			fail("Invalid port glyph %s=%s in the symbol mapping of %s", kind, name, lib.File)
		}
		// Port wires end at the glyph's pin by its name, %port_N::pin.
		if !glyphPin.MatchString(packageSymbolDefinition(lib.Text, name, "$"+name)) {
			fail("Port glyph $%s of %s has no pin named pin (%%pin = (x, y) @PIN(...))", name, lib.File)
		}
	}
	return &m
}

var glyphPin = rx(`(^|[\s{;])%pin\s*=`)

func checkTerminalNames(symbol string, names []string) {
	seen := stringSet{}
	for _, n := range names {
		if n == "" || seen[strings.ToLower(n)] || strings.Trim(n, "0123456789") == "" {
			fail("Invalid terminal names for %s", symbol)
		}
		seen[strings.ToLower(n)] = true
	}
}

var (
	recordHead   = rx(`@RNL(\(([^)]*)\))?\s*\{`)
	ruleForm     = rx(`#\s*@SELECT-INSTANCE\(([^)]*)\)\s*=>\s*@SET-SYMBOL(\(\s*terminals\s*=\s*\[([^\]]*)\]\s*\))?\s*\$([A-Z_][A-Z0-9_]*)`)
	deviceForm   = rx(`@DEVICE\(([^)]*)\)`)
	condition    = rx(`(\w+)\s*=\s*("(?:[^"]|"")*"|\[[^\]]*\]|[A-Za-z0-9_.]+)`)
	languageHead = rx(`language\s*=\s*"([a-z]+)"`)
)

// packageRules reads the RNL a library already has for instances: rules of the
// form `# @SELECT-INSTANCE(model=…) => @SET-SYMBOL[(terminals=[…])] $X` and
// @DEVICE terminal names. Rules with other conditions are left to the viewer.
func packageRules(text string) ([]modelRule, []deviceNames) {
	heads := recordHead.FindAllStringSubmatchIndex(text, -1)
	languageAt := func(pos int) string {
		language := ""
		for _, h := range heads {
			if h[0] > pos {
				break
			}
			language = ""
			if h[4] >= 0 {
				if m := languageHead.FindStringSubmatch(text[h[4]:h[5]]); m != nil {
					language = m[1]
				}
			}
		}
		return language
	}
	rules := []modelRule{}
	for _, m := range ruleForm.FindAllStringSubmatchIndex(text, -1) {
		r := modelRule{Symbol: text[m[8]:m[9]], Language: languageAt(m[0])}
		usable := true
		for _, c := range condition.FindAllStringSubmatch(text[m[2]:m[3]], -1) {
			switch c[1] {
			case "model":
				r.Model = unquote(c[2])
			case "name":
				r.Name = unquote(c[2])
			case "recursive":
			default:
				usable = false
			}
		}
		if m[6] >= 0 {
			r.Terminals = stringList(text[m[6]:m[7]])
		}
		if usable && (r.Model != "" || r.Name != "") {
			rules = append(rules, r)
		}
	}
	devices := []deviceNames{}
	for _, m := range deviceForm.FindAllStringSubmatchIndex(text, -1) {
		d := deviceNames{Language: languageAt(m[0])}
		for _, c := range condition.FindAllStringSubmatch(text[m[2]:m[3]], -1) {
			switch c[1] {
			case "model":
				d.Model = unquote(c[2])
			case "name":
				d.Name = unquote(c[2])
			case "terminals":
				d.Terminals = stringList(strings.Trim(c[2], "[]"))
			}
		}
		if len(d.Terminals) > 0 {
			devices = append(devices, d)
		}
	}
	return rules, devices
}

func unquote(s string) string {
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
	}
	return s
}

func stringList(s string) []string {
	out := []string{}
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, unquote(item))
		}
	}
	return out
}

// wildcard compares as RNL 6.12 does: `*` is any run of characters, and SPICE
// ignores case while Spectre does not.
func wildcard(pattern, value, dialect string) bool {
	if dialect == "cdl" {
		pattern, value = strings.ToLower(pattern), strings.ToLower(value)
	}
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	return regexp.MustCompile(re).MatchString(value)
}

func (l *symbolLibrary) languageApplies(language, dialect string) bool {
	return language == "" || (language == "spice") == (dialect == "cdl")
}

// match gives the symbol of this library for a leaf instance, or nil.
func (l *symbolLibrary) match(source cellKey, name, model, dialect string, nodes []string) *symbolChoice {
	if l.Mapping != nil {
		for _, c := range l.Mapping.Cells {
			if c.Library != source.Library || !strings.EqualFold(c.Cell, source.Cell) {
				continue
			}
			rule := l.Rules[c.Symbol]
			if rule.Correction == "" {
				rule.Correction = "R0"
			}
			if c.Correction != "" {
				rule.Correction = c.Correction
			}
			if c.Align != nil {
				rule.Pins = c.Align
			}
			names := c.Terminals
			if names == nil {
				names = l.Mapping.Terminals[c.Symbol]
			}
			if c.Pins != nil {
				// Pairs name each terminal of the cell; a terminal without a pair
				// gets no name, so the instance keeps its EDIF symbol.
				names = []string{}
				for _, node := range nodes {
					found := ""
					for _, pair := range c.Pins {
						if strings.EqualFold(pair[0], node) {
							found = pair[1]
						}
					}
					names = append(names, found)
				}
			}
			return &symbolChoice{Library: l, Symbol: c.Symbol, Terminals: names, Rule: rule, Mode: "mapping"}
		}
		return nil
	}
	var chosen *modelRule
	for i := range l.Rule {
		r := &l.Rule[i]
		if !l.languageApplies(r.Language, dialect) || !l.Defined[r.Symbol] {
			continue
		}
		if (r.Model == "" || wildcard(r.Model, model, dialect)) && (r.Name == "" || wildcard(r.Name, name, dialect)) {
			chosen = r // a later rule of the package overrides an earlier one
		}
	}
	if chosen == nil {
		return nil
	}
	names := chosen.Terminals
	if names == nil {
		for _, d := range l.Devices {
			if l.languageApplies(d.Language, dialect) && (d.Model == "" || wildcard(d.Model, model, dialect)) && (d.Name == "" || wildcard(d.Name, name, dialect)) {
				names = d.Terminals
			}
		}
		// Terminal names bind only when the symbol's pins carry them.
		for _, n := range names {
			if _, ok := l.Pins[chosen.Symbol][strings.ToLower(n)]; !ok {
				names = nil
				break
			}
		}
	}
	return &symbolChoice{Library: l, Symbol: chosen.Symbol, Terminals: names, Rule: orientationRule{Correction: "R0"}, Mode: "rules"}
}

// chooseSymbol asks the libraries in priority order; the first that maps the
// instance gives its symbol.
func chooseSymbol(libraries []*symbolLibrary, source cellKey, name, model, dialect string, nodes []string) *symbolChoice {
	for rank, l := range libraries {
		if c := l.match(source, name, model, dialect, nodes); c != nil {
			c.Rank = rank
			return c
		}
	}
	return nil
}

// hiddenBy reports a library of higher priority that defines the same name:
// the header places it later, so the name would resolve to that definition.
func hiddenBy(libraries []*symbolLibrary, c *symbolChoice) *symbolLibrary {
	for _, l := range libraries[:c.Rank] {
		if l.Defined[c.Symbol] {
			return l
		}
	}
	return nil
}

var includesSymbol = rx(`(?:^|[{;])\s*\$[A-Za-z_"]`)

// localCopy copies a self-contained symbol definition of a library into the
// level as a % definition. A definition that includes other symbols of its
// package cannot be copied alone.
func localCopy(l *symbolLibrary, symbol, id string) (string, bool) {
	text := packageSymbolDefinition(l.Text, symbol, id)
	body := text[strings.Index(text, "{")+1:]
	if includesSymbol.MatchString(body) {
		return "", false
	}
	return text, true
}

// portGlyph is the first library, by priority, whose mapping names a glyph
// for schematic pins of this direction.
func portGlyph(libraries []*symbolLibrary, kind string) (*symbolLibrary, string) {
	for _, l := range libraries {
		if l.Mapping != nil && l.Mapping.Ports[kind] != "" {
			return l, l.Mapping.Ports[kind]
		}
	}
	return nil, ""
}

// nativeModel is the master or model name of a native instance line.
func nativeModel(words []string, dialect string) string {
	if len(words) == 0 {
		return ""
	}
	if dialect == "spectre" {
		return words[len(words)-1]
	}
	for i := len(words) - 1; i > 0; i-- {
		w := words[i]
		if strings.Contains(w, "=") {
			continue
		}
		return strings.TrimSuffix(strings.TrimPrefix(w, "$["), "]")
	}
	return ""
}

func loadSymbolLibraries(inputs []libraryInput, resourceDir string) []*symbolLibrary {
	if len(inputs) == 0 {
		inputs = []libraryInput{{Path: filepath.Join(resourceDir, "eda-symbol.rnl")}}
	}
	out := []*symbolLibrary{}
	files, names := stringSet{}, stringSet{}
	for _, in := range inputs {
		l := loadSymbolLibrary(in)
		if files[strings.ToLower(l.File)] || l.File == "eda-style.rnl" {
			fail("Two symbol libraries would be written as %s", l.File)
		}
		if names[l.Name] {
			fail("Symbol library %s is given twice", l.Name)
		}
		files[strings.ToLower(l.File)], names[l.Name] = true, true
		out = append(out, l)
	}
	return out
}

// libraryHeaderLines are the USE-PACKAGE lines of the symbol libraries, the
// highest priority last: of two packages at the root, the one placed later
// is looked up first (RNL 0.4, 16.2).
func libraryHeaderLines(libraries []*symbolLibrary) []string {
	lines := []string{}
	for i := len(libraries) - 1; i >= 0; i-- {
		l := libraries[i]
		lines = append(lines, `@USE-PACKAGE(name=`+quote(l.Name)+`, version=`+quote(l.Version)+`, path=`+quote(l.File)+`)`)
	}
	return lines
}

// libraryRecords are the symbol libraries that a library capture refers to:
// their paths, or their texts when they have none. Nil means the default list.
func libraryRecords(inputs []libraryInput, libraries []*symbolLibrary) []libraryInput {
	if len(inputs) == 0 {
		return nil
	}
	out := []libraryInput{}
	for i, in := range inputs {
		if in.Path != "" {
			out = append(out, libraryInput{Path: in.Path})
		} else {
			out = append(out, libraryInput{File: libraries[i].File, Text: in.Text, Mapping: in.Mapping})
		}
	}
	return out
}

func libraryReport(libraries []*symbolLibrary) []map[string]string {
	out := []map[string]string{}
	for _, l := range libraries {
		mode := "mapping"
		if l.Mapping == nil {
			mode = "rules"
		}
		out = append(out, map[string]string{"file": l.File, "name": l.Name, "version": l.Version, "mode": mode})
	}
	return out
}
