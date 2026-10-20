package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (n *node) UnmarshalJSON(data []byte) error {
	var value any
	if e := json.Unmarshal(data, &value); e != nil {
		return e
	}
	var build func(any) node
	build = func(v any) node {
		switch x := v.(type) {
		case string:
			return atom(x)
		case []any:
			out := node{group: true, list: []node{}}
			for _, c := range x {
				out.list = append(out.list, build(c))
			}
			return out
		case nil:
			return node{}
		default:
			fail("Invalid JSON EDIF node")
		}
		return node{}
	}
	*n = build(value)
	return nil
}
func guarded(fn func()) (message string) {
	defer func() {
		if p := recover(); p != nil {
			if e, ok := p.(exportError); ok {
				message = e.Error()
			} else {
				panic(p)
			}
		}
	}()
	fn()
	return ""
}
func testJSON(dir string) {
	var req struct {
		Op string `json:"op"`
		convertInput
		Cell                *cell             `json:"cell"`
		Endpoints           []endpoint        `json:"endpoints"`
		NetMap              map[string]string `json:"netmap"`
		SymbolID            string            `json:"symbol_id"`
		Terminals           map[string]string `json:"terminals"`
		Text                string            `json:"text"`
		Source, Destination []point
		Orient, Symbol      string
		At                  point
		Rules               map[string]orientationRule
		Request             string
	}
	out := map[string]any{}
	message := guarded(func() {
		if e := json.NewDecoder(os.Stdin).Decode(&req); e != nil {
			fail("Invalid conversion JSON: %s", e)
		}
		switch req.Op {
		case "", "convert":
			files, report := convert(req.convertInput, dir)
			out["files"], out["report"] = files, report
		case "routes":
			scale := 1.0
			if req.Scale != nil {
				scale = *req.Scale
			}
			records, count := drawRoutes(req.Cell, req.Endpoints, req.NetMap, scale)
			out["records"], out["count"] = records, count
		case "symbol":
			notes := []string{}
			scale := 10.0
			if req.Scale != nil {
				scale = *req.Scale
			}
			out["text"] = symbolDefinition(req.Cell, req.SymbolID, req.Terminals, scale, &notes)
			out["notes"] = notes
		case "place":
			xy, orient := place(req.Symbol, req.Source, req.Destination, req.Orient, req.At, req.Rules)
			out["xy"], out["orient"] = xy, orient
		case "rules":
			text := req.Text
			if text == "" {
				text = resource(dir, "symbol-orientations.json")
			}
			out["rules"] = loadRules(text)
		case "interfaces":
			out["interfaces"] = interfaces(req.Text)
		default:
			fail("Unknown test operation %s", req.Op)
		}
	})
	if message != "" {
		out = map[string]any{"error": message}
	}
	if e := json.NewEncoder(os.Stdout).Encode(out); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func main() {
	exe, e := os.Executable()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	dir := flag.String("resources", filepath.Join(filepath.Dir(exe), "..", "resources"), "Symbol packages and orientation table directory")
	showVersion := flag.Bool("version", false, "Print backend version")
	test := flag.Bool("test-json", false, "Read one offline conversion/test request from stdin")
	flag.Parse()
	if *showVersion {
		fmt.Println("rnl-virtuoso " + version + " (Go)")
		return
	}
	if *test {
		testJSON(*dir)
		return
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: rnl-virtuoso [--resources directory] request.sx")
		os.Exit(2)
	}
	request := flag.Arg(0)
	start := time.Now()
	message := guarded(func() { runJob(request, *dir) })
	if message != "" {
		fmt.Fprintln(os.Stderr, "RNL export failed: "+message)
		if e := os.WriteFile(filepath.Join(filepath.Dir(request), "error.txt"), []byte(message+"\n"), 0644); e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}
	fmt.Printf("Finished in %.1f seconds\n", time.Since(start).Seconds())
}
