# Export options and conversion


Check and Save the source schematic and its circuit descendants, then choose
**RNL → Export RNL...**. The source is the currently edited cellview; it is
captured when the form opens.

The CIW also has **File → Export → RNL...**. In **Cell** mode, choose the
library and schematic cell directly; the options and backend are the same as
the schematic-editor entry. No editor window is required.

| Option | Behavior |
| --- | --- |
| New output directory | A new folder on the Virtuoso host; its parent must exist. Existing exports are refused. |
| Netlist format | Spectre (simulation), the default, or SPICE / CDL (LVS). |
| Root circuit | A schematic with ports becomes a subcircuit; one without ports remains the file-level circuit. |
| Include subcircuits | Keep native netlist hierarchy; never flatten. When off, only the current block remains. |
| Include referenced external cells | Export foreign schematic definitions when enabled; otherwise retain their calls and interface symbols. Available for both cell and library exports. |
| Subcircuits | Single-cell exports can use one combined file or one file per cell. Libraries always use one file per cell. |
| EDIF units per RNL unit | Defaults to 10, matching the tested native export and `eda-symbol` dimensions. |

Export runs in the background. The CIW reports progress; a modeless message
shows the output location or error. Output contains `<schematic-cell-name>.scs` (Spectre)
or `<schematic-cell-name>.sp` (CDL), optional
`<cell-name>.scs`/`.sp` files, external-library directories, `eda-symbol.rnl` and `eda-style.rnl` (both package version 0.1), and
`export-report.json`. Their RNL language version is 0.3; both packages are
referenced explicitly in every netlist file header.
The entry filename uses the exported schematic cell's name and case in every
mode, including portless schematics; the report and completion message give that path.
Separate files use source cell names and case: cells in the selected library
are saved as `<cell>.scs` / `.sp` at the export root, and foreign cells as
`<external-library>/<cell>.scs` / `.sp`. A foreign cell referenced multiple times
is saved once. The declarations and device calls inside each file retain the
native subcircuit names. Native context variants of the same source cell share
that cell's file; their distinct native declarations are retained.

A cell with ports contains its own subcircuit definitions and RNL, with no native
include statements. A portless top includes all required descendant cell files,
including enabled external cells, once each in a flat list. Descendant files do
not include each other, avoiding duplicate definitions through shared children.
Native model/Verilog-A include statements remain in portless top entries; model
files are not copied. A standalone subcircuit is a definition fragment and needs
its dependencies and model references supplied by its caller.
Exported child symbols live inside their own subcircuit definitions; parents
inherit them without local copies or per-instance symbol overrides. Load those
definitions through a caller, or open a portless top to browse the complete
hierarchy. Only children whose definitions are omitted retain local interface
symbols in the parent. Combined single-cell exports retain the original combined
native hierarchy and model references.
Unsafe filenames or case-insensitive file/directory collisions stop the export
rather than overwrite a file or invent a different name.
When the current cell has a saved `symbol` view, its native EDIF is also
converted. The symbol is embedded in the current subcircuit (when it has ports)
and exported as `cell-symbol.rnl`, with the public symbol `$"library/cell"` in
package `virtuoso/library/cell` version `0.1`. Even a portless top with a symbol
gets this separate reusable symbol file.
Keep the directory together when opening or copying the result.

## Whole-library export

Choose **Library** in the CIW form. All cells with a `schematic` view are exported;
cells with only `symbol` are exported as reusable symbols. Cells with neither
view are skipped and listed in `library-report.json`. The exporter reads saved
views and does not check/save, create or edit source cells.

Library mode always exports all of the library's own cells, one file per cell;
the Combined file option applies only to single-cell export. **Include referenced
external cells** controls foreign schematic definitions: when off, their instance
calls and symbols remain as external interfaces. When on, schematic definitions
emitted by the native hierarchy exporter are included. Native stop views, PDK
primitives and Verilog-A leaves remain leaves; model references stay external.

The common output directory contains:

- `<cell>.scs` / `.sp`: the cell's actual subcircuit or portless circuit, with its
  RNL drawing. There is no separate entry wrapper or `netlists/` directory.
- `<external-library>/<cell>.scs` / `.sp`: referenced foreign cells, saved once
  per source library/cell across the library export. Symbol/style package paths
  are relative to each file, including files inside these directories.
  Portless entries include their required descendants only; unrelated cells and
  other portless testbenches are never included.
- `symbols.rnl`: package `virtuoso-library/<library>` version `0.1`, with reusable
  symbols named `$"library/cell"`, including symbol-only cells and referenced
  interfaces. For example: `@USE-PACKAGE(name="virtuoso-library/myLib", version="0.1", path="symbols.rnl")`.
- Shared `eda-symbol.rnl`, `eda-style.rnl` and `library-report.json`. The report
  maps source cells, native subcircuit names, entry paths and exported symbols,
  and records model references, notes and skipped cells.

The queue runs one native job at a time. Results are published together only
after all cells have been attempted and the merge succeeds. Existing output
directories are refused. Failed cells are skipped; `failed_cells` records their
library, cell, view, failure stage, reason and log path. `complete` is false when
any cell failed, and the completion notice gives the failure count. A failed
cell leaves its native files in the reported work directory. A library
containing only symbols is supported, with no schematic entry files.

Repeated native declarations are merged only when the native name, source library/cell and
electrical statements agree. Native context variants with distinct names stay
distinct. A generated Spectre root wrapper may order its named ports differently
from a native child definition. If the port sets and device statements agree,
the cell file keeps the native child's positional port order and body.
Differences in indentation and continuation wrapping are ignored during comparison;
quoted strings retain their exact content. Conflicting native names or symbol geometry stop the export with a
diagnostic; the exporter does not rewrite device calls to invent global aliases.
Library directories do not create electrical namespaces: conflicting native
subcircuit names still stop the merge. Unsafe library directory names, names
that differ only by case, or collisions with root output files are also refused.
Large libraries can therefore require a native netlister naming policy that is
consistent across roots. CIW menu installation and reload preserve existing site
menu entries and never add a second RNL item.

```skill
rnlStartLibraryExport("myLib" "/path/to/new-library-export" t t 10.0 "spectre" nil)
; library, new directory, includeHierarchy (t), separateFiles, scale,
; optional format (spectre/cdl), includeExternalCells (default nil)
```

`rnlActiveBatch` is non-nil while a library export runs. `rnlLastBatch->status`
records the final status; `rnlLastBatch->directory` holds queue/merge diagnostics.

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
  `schematic spectre veriloga veriloga_ans`, stop views
  `spectre veriloga veriloga_ans`, hierarchical mode and command `nl`;
  CDL uses `auCdl schematic`, stop view `auCdl`, command `netlist` and the native
  CDL preservation options. Spectre retains child `subckt` definitions and the native header/footer.
  A cell with both `schematic` and `spectre` views expands its schematic;
  a primitive or behavioral cell without a schematic keeps its native stop view.
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
  Name maps decode both the SKILL string escapes and the enclosed Spectre
  identifier escapes, including names containing hyphens.
  EDIF's leading `&` escape is removed from display/database names such as
  `&_experiment`; EDIF references and literal strings retain their identity.
- Blank separator entries in CDF `termOrder` are ignored; real terminal names
  retain their spelling and order. Spectre PCell CDFs may declare optional pins
  absent from the evaluated EDIF symbol. Their absence is noted; active pins keep
  CDF order, and complete symbol coverage and native node count are required.
  Native scalar PDK series/parallel expansions
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
  For a parameterized symbol, preflight checks its saved supermaster rather
  than rejecting the transient generated submaster as an unsaved user edit.
  Instances explicitly marked `nlAction=ignore` are omitted as in the native
  netlist, with an export note. Other missing native devices remain errors.
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
  The exported Spectre copy of `eda-symbol` omits SPICE instance-letter rules:
  an instance named `I0` keeps its cell symbol, while `isource` still selects a
  current source by model. CDL copies keep the SPICE letter rules.
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
  This also applies to separate files and library exports: the parent uses the
  child's default symbol. In current-level mode, or when an external child is
  excluded, its interface symbol is embedded and assigned within the parent.
  EDIF paths, polygons, rectangles, circles and arcs are converted to RNL
  primitives. Dynamic `cdsName()` becomes the instance name; other unevaluated
  Cadence label expressions are listed in the report instead of evaluated.
- Wire export keeps each EDIF route's actual endpoints. A terminal serving as a
  connection point is referenced directly, including repeated connections and
  coincident pin contacts. Coordinate points are kept for route bends and
  junctions without a suitable terminal. Repeated paths are deduplicated; pins
  elsewhere on the same net do not acquire extra coordinate connectors. These
  rules also apply when switching to the standard symbol package after export.
- Net labels attach to the nearest drawn wire segment of that EDIF net, including
  bus labels and deduplicated routes. Their original position, rotation, alignment
  and text size are preserved. A label without a drawn path attaches to its net.
- Free schematic notes remain RNL annotations. EDIF note figures are converted,
  and a read-only snapshot supplements non-electrical shapes on the schematic's
  `text` layer that Cadence EDIF Out omits. Lines, paths, rectangles, polygons,
  ellipses and arcs retain their geometry and each cell's coordinate scale;
  elliptical arcs use cubic Bezier approximations. Colors and line widths are
  left to the presentation package. The native `text/drawing1` note purpose
  becomes `linestyle="dashed"`; solid and legacy notes keep the default stroke.
  Notes add no electrical wires or connections.
- EDIF input/output ports use local copies of the EDA `$IPIN`/`$OPIN` glyphs,
  so replacing `eda-symbol` with the standard symbols remains valid;
  inout or unspecified
  directions retain `$IO_PORT_SYMBOL`. All keep their original pin coordinates
  and orientation. Repeated schematic port markers on the same logical terminal
  are exported individually, preserving each placement and wire attachment
  without adding electrical ports. A bus has a pin-only multi-net anchor for wire connectivity
  and a visible `$IO_PORT` marker for its shape and range label. This lets the
  style package's port rules apply to buses as well, without fixed colors in
  exported symbols or extra electrical wires.
- One-dimensional bus ports become RNL bus pins, multiple nets share one bus
  wire, and iterated instances become `BUNDLE`s. Their scalar native statements
  remain intact. Range labels retain the source spelling and bit order. Repeated connections
  such as `<*64>TOP` and `<*2>BUS<3:0>` expand electrically while their array
  instances stay grouped in the drawing.

## Current boundaries

Repeated schematic port markers are supported. Multiple physical pins of the
same terminal in a symbol are not: RNL 0.3 gives each terminal one pin, with
later definitions overriding earlier ones. Such symbol pins still collapse to
one connection location.

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
