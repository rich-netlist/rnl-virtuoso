# Export options and conversion


Check and Save the source schematic and its circuit descendants, then choose
**RNL → Export RNL...**. The source is the currently edited cellview; it is
captured when the form opens.

| Option | Behavior |
| --- | --- |
| New output directory | A new folder on the Virtuoso host; its parent must exist. Existing exports are refused. |
| Netlist format | Spectre (simulation), the default, or SPICE / CDL (LVS). |
| Root circuit | A schematic with ports becomes a subcircuit; one without ports remains the file-level circuit. |
| Include subcircuits | Keep native netlist hierarchy; never flatten. When off, only the current block remains. |
| Subcircuits | One combined file, or an entry file with native include statements. |
| EDIF units per RNL unit | Defaults to 10, matching the tested native export and `eda-symbol` dimensions. |

Export runs in the background. The CIW reports progress; a modeless message
shows the output location or error. Output contains `<schematic-cell-name>.scs` (Spectre)
or `<schematic-cell-name>.sp` (CDL), optional
`<native-subcircuit-name>.scs`/`.sp` files, `eda-symbol.rnl` and `eda-style.rnl` (both package version 0.1), and
`export-report.json`. Their RNL language version is 0.3; both packages are
referenced explicitly in every netlist file header.
The entry filename uses the exported schematic cell's name and case in every
mode, including portless schematics; the report and completion message give that path.
Separate files use the exact subcircuit names from the native netlist, including
case and any names Cadence assigns to distinguish cells from different libraries.
Their include paths use the same names. Unsafe filenames or case-insensitive
collisions with another subcircuit or the entry file stop the export rather than
overwrite a file or invent a different name.
When the current cell has a saved `symbol` view, its native EDIF is also
converted. The symbol is embedded in the current subcircuit (when it has ports)
and exported as `cell-symbol.rnl`, with the public symbol `$"library/cell"` in
package `virtuoso/library/cell` version `0.1`. Even a portless top with a symbol
gets this separate reusable symbol file.
Keep the directory together when opening or copying the result.

Use **Spectre (simulation)** for testbenches: native CDL is an LVS export and
can omit voltage/controlled sources and switches or short their terminals.
The converter rejects missing electrical devices instead of silently dropping
them from the drawing. Spectre keeps those native simulation devices, their
parameters and connections; it is not converted to SPICE syntax.

For a script-driven export, after installing:

```skill
rnlStartExport("myLib" "myCell" "schematic" "/path/to/new-export" t nil 10.0)
; arguments: library, cell, view, destination, includeHierarchy, separateFiles, scale
; optional eighth argument: "spectre" (default) or "cdl"
```

This returns the temporary job directory immediately. `rnlActiveJob` is non-nil
while the process runs; `rnlLastJob->status` is its exit status after completion.
One job runs per Virtuoso session. The job directory keeps the exact native
netlist/EDIF, `si.env`, CDF terminal metadata, native name maps, logs and any error for diagnosis.

Validate or render with the [RNL toolkit](https://github.com/rich-netlist/rnl):

```sh
rnl check /path/to/new-export/myCell.scs
rnl render /path/to/new-export/myCell.scs -o /path/to/rendered-directory
```

Array exports require an RNL runtime whose package symbol rules select the
bundle after membership has been resolved. Older viewers may
report errors about drawing hidden bundle members.

## Conversion contract

- Electrical statements come from native `si`: Spectre uses switch views
  `spectre schematic veriloga veriloga_ans`, stop views
  `spectre veriloga veriloga_ans`, hierarchical mode and command `nl`;
  CDL uses `auCdl schematic`, stop view `auCdl`, command `netlist` and the native
  CDL preservation options. Spectre retains child `subckt` definitions and the native header/footer.
  The current circuit gets a `subckt` wrapper if its EDIF interface has ports;
  a portless CDL root has its native wrapper removed. Device statements stay
  intact, and global declarations/model references stay outside the wrapper.
  The converter adds RNL comments without rewriting device nodes, values or
  model calls. File splitting adds includes; current-level mode removes lower
  definitions. Neither mode translates simulator syntax.
- EDIF supplies symbol geometry, placements, all eight orthogonal orientations,
  wire vertices, bus membership and labels. Primitive terminal order comes
  from the selected simulator's CDF; child terminal order comes from the native
  subcircuit header. Spectre uses the netlister's own instance/net name maps
  (including bus names and ground aliases); CDL uses native prefix/spelling
  candidates. Terminal-to-net correspondence is checked, and ambiguous or
  missing mappings fail. Spectre names retain their case sensitivity.
- Blank separator entries in CDF `termOrder` are ignored; real terminal names
  retain their spelling and order. Native scalar PDK series/parallel expansions
  are recognized only through the netlister's explicit `//Series configuration
  of ...` / `//Parallel configuration of ...` and matching `//End of ...`
  delimiters. Each keeps all native device statements while a composite RNL usage
  shows the original EDIF symbol, source name/model, orientation and external pins.
  Its terminal roles bind actual member terminals. Native members get empty
  symbols so they remain electrically present without duplicate bodies/labels.
  Series chains, shared terminals, model/interface consistency and external/global
  branches are checked before grouping. Device-name prefixes are never used to
  guess expansion membership. `statistics.instances` counts native electrical
  instances; `expanded_instances`, when present, counts source expansion groups.
- Preflight follows the selected stop view and skips `nlAction=ignore/stop`
  markers such as ground and no-connect instead of requiring their schematic
  views to be checked and saved. A no-connect at a wire end is retained as a
  figure on that wire point; a marker on a device pin uses `$NO_CONNECT`.
- Spectre behavioral leaves use the selected Verilog-A module's declared port
  order, checked against EDIF, instead of guessing from a missing CDF. Native
  `ahdl_include` lines are preserved, with their original model file references
  recorded in the export report. Model files remain external; copying the RNL
  bundle does not make a portable simulation deck. Ordinary `veriloga` takes
  precedence over `veriloga_ans` when both exist.
- Common `analogLib` primitives use `eda-symbol`. Package orientation changes
  are compensated before the source orientation is applied, then the origin is
  adjusted to align the pins. Wire ends reference the current terminal directly;
  interior EDIF route vertices are retained. Other primitives use their EDIF symbol,
  with CDF terminal order.
- Exported custom symbols contain geometry, layout/text size and `@PIN`s with
  unknown direction omitted. They contain no generated pin squares or fixed
  colors, line widths or style assignments. Presentation and optional pin marks
  come from the separately referenced `eda-style` package. Replace that header
  reference to apply another style.
- [symbol-orientations.json](../resources/symbol-orientations.json) is
  the editable table for all 18 documented symbol changes. Its transforms use
  RNL/EDIF matrix conventions, including `MY` for negating x. The configuration
  and pinned package snapshots are described in [resource notes](../resources/README.md).
- A child symbol is embedded in its own subcircuit definition with `# @SET-SYMBOL`.
  In current-level mode it is embedded and assigned within the parent.
  EDIF paths, polygons, rectangles, circles and arcs are converted to RNL
  primitives. Dynamic `cdsName()` becomes the instance name; other unevaluated
  Cadence label expressions are listed in the report instead of evaluated.
- Wire export keeps each EDIF route's actual endpoints. A terminal serving as a
  connection point is referenced directly, including repeated connections and
  coincident pin contacts. Coordinate points are kept for route bends and
  junctions without a suitable terminal. Repeated paths are deduplicated; pins
  elsewhere on the same net do not acquire extra coordinate connectors. These
  rules also apply when switching to the standard symbol package after export.
- One-dimensional bus ports become RNL bus pins, multiple nets share one bus
  wire, and iterated instances become `BUNDLE`s. Their scalar native statements
  remain intact. Range labels retain the source spelling and bit order. Repeated connections
  such as `<*64>TOP` and `<*2>BUS<3:0>` expand electrically while their array
  instances stay grouped in the drawing.

## Current boundaries

This version targets saved schematic hierarchies with the fixed view lists
above. Config views, arbitrary stop/switch view policies, HSPICE output,
layout extraction, mosaic/scaled instances and multidimensional arrays are
not implemented. Verilog-A interface reading currently supports non-ANSI
module headers, scalar ports and buses with literal integer bounds; it does
not evaluate macros or parameterized port ranges. Unsupported geometry or
an unresolvable electrical mapping stops the job; the native files remain in
the job directory. EDIF library/style colors and fonts are not copied: the
result uses `eda-symbol` geometry with `eda-style` as the default presentation,
and text is rendered by RNL. PDK coverage beyond the
tested primitives still needs validation on actual designs.
PDK expansion groups inside drawn instance arrays and expansion patterns without
the supported native delimiters are not implemented; they fail explicitly.

The output is for schematic interchange. Device model files and simulation
analysis settings are not packaged. Native design variables remain symbolic;
the exporter does not resolve ADE values or run a simulation. Including only
the current layer intentionally leaves its child electrical definitions external while retaining their symbols.
