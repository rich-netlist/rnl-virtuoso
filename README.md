# rnl-virtuoso

Export Virtuoso schematics to [Rich Netlist (RNL)](https://github.com/rich-netlist/rnl).
Adds **RNL → Export RNL...** to the schematic editor and **File → Export → RNL...**
to the CIW. Export a selected cell or a whole library. A SKILL front end reuses
Cadence's native Spectre/CDL and EDIF exporters; a Go backend combines the native
netlist and drawing without rewriting device statements.

- Preserve hierarchy, custom block symbols, placement, orientation, wires, buses and drawn arrays.
- Export subcircuits in one file, separate files, or just the current level.
- Schematics with ports become subcircuits; portless schematics remain top-level circuits.
- Reference `eda-symbol` and `eda-style` separately, with an editable orientation table.
- Whole-library exports share deduplicated definitions and an optional reusable symbol package,
  with optional referenced external cells and support for symbol-only cells.

## Install

Download the Linux x86-64 archive from [Releases](https://github.com/rich-netlist/rnl-virtuoso/releases),
then extract it on the Virtuoso host:

```sh
mkdir -p ~/tools
tar -xzf rnl-virtuoso-0.9.4-linux-amd64.tar.gz -C ~/tools
```

In the CIW, load the entry point once:

```skill
load("~/tools/rnl-virtuoso/rnl-virtuoso.il")
```

The same line can go into `.cdsinit`. Loading registers the menu automatically;
repeated loads do not duplicate it. No Python, Go installation or bridge service
is required on the host. Native `si`, `edifout` and their Cadence licenses are required.
Tested with IC6.1.8-64b.500.26 on Linux x86-64.

## Use

Select **RNL → Export RNL...**. Stale schematic connectivity is automatically
checked and saved by default (0.9.2+). Clear **Automatically Check & Save stale
schematics**, or use `?autoCheckSave nil`, for read-only export.
Choose a **new output directory**. **Spectre (simulation)** is recommended for
sources, switches and Verilog-A models; CDL is an LVS export and may omit them.
Keep the generated netlist, symbol/style files and report together.
External model references remain external; this does not package a runnable simulation deck.

From **CIW → File → Export → RNL...**, choose **Cell** and a library/cell without
opening its editor, or **Library** to export all schematic and symbol-only cells.
Each cell's actual circuit is saved as `<cell>.scs` / `.sp` at the export root;
foreign cells go to `<external-library>/<cell>.scs` / `.sp`, once per source cell.
There are no entry wrappers or `netlists/` directory. Subcircuits have no native
includes; portless tops include their descendants once in a flat list. Single-cell
exports also retain the combined-file option. Optionally export a shared `symbols.rnl`; the output always shares
the two presentation packages. The report lists entries, native
definitions and exported symbols. The external-cell option preserves interfaces
and symbols when off, and includes referenced schematic definitions when on.

**Export symbols.rnl separately** is off by default (SKILL: `?exportSymbols nil`).
Enable it to add the reusable symbol collection to either cell or library exports.
Symbol-only library cells always get their own `<cell>.rnl`; `cell-symbol.rnl` is
no longer generated. Embedded schematic symbols are preserved.
Failed cells are skipped and reported with their reasons and logs; `complete`
is false when any cell failed, and the completion notice gives the failure count.

See [export details](docs/export.md) for options, conversion rules and limitations.

Both forms provide **Schematic views** and **Symbol views** fields. Enter names
separated by spaces, in descending priority, for example `schematic schematic_ideal`
and `symbol_alt symbol`. The first existing view is selected for each cell.
Interfaces compare expanded member names, allowing reversed bus ranges while
preserving symbol pin order and native electrical mapping.
The same options are available as `?schematicViews` and `?symbolViews` in the SKILL
API (0.9.0+). Defaults remain `schematic` and `symbol`.

**Export selection box** (enabled by default, 0.9.1+) uses the enclosing rectangle
of all native selection boxes in the selected symbol view. Disable it to use the
symbol graphics extent, excluding text. The SKILL keyword is `?exportSelectionBox`.

## Scripts and agents

The public SKILL API exports without an editor or completion dialog:

```skill
job = rnlExportCell("myLib" "myCell" "~/exports/myCell")
rnlExportStatus(job)
```

Use `rnlExportLibrary` for a whole library, `rnlExporterInfo` to inspect the
installation, and keyword options for format/hierarchy/external cells. Jobs are
asynchronous; see [API documentation](docs/skill-api.md) for completion states.

Copy [skills/rnl-virtuoso](skills/rnl-virtuoso) to your agent's skill directory
(Codex: `~/.codex/skills/rnl-virtuoso`). It builds on the existing `virtuoso`
bridge skill, reuses a remote installation, and asks before installing a missing
exporter in the user's home. The skill contains no exporter or bridge assets;
new installations fetch the latest release from this repository.

## Build and test

Building requires Go 1.22+ and Python 3.8+ on the development machine only:

```sh
python scripts/package.py dist/rnl-virtuoso-0.9.4-linux-amd64.tar.gz
python scripts/test.py
```

The package contains a static Linux x86-64 backend. `--go /path/to/go` selects a
compiler. To enable drawing/checker assertions, pass `--rnl /path/to/rnl` to the
test script. See [development notes](docs/development.md).

## License

Code, tests and documentation: [MIT](LICENSE). Bundled symbol/style packages:
CC0-1.0. The Go runtime/standard-library notice is in [Go-LICENSE.txt](Go-LICENSE.txt).
Cadence software, library symbols and user designs are not included.
