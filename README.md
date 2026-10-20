# rnl-virtuoso

[中文](README.zh.md)

Export Virtuoso schematics to [Rich Netlist (RNL)](https://github.com/rich-netlist/rnl).
Adds **RNL → Export RNL...** to the schematic editor and **File → Export → RNL...**
to the CIW. Export a selected cell or a whole library. A SKILL front end reuses
Cadence's native Spectre/CDL and EDIF exporters; a Go backend combines the native
netlist and drawing without rewriting device statements.

- Preserve hierarchy, custom block symbols, placement, orientation, wires, buses and drawn arrays.
- Export subcircuits in one file, separate files, or just the current level.
- Schematics with ports become subcircuits; portless schematics remain top-level circuits.
- Reference `eda-symbol` and `eda-style` separately, with an editable orientation table.
- Whole-library exports share deduplicated definitions and a reusable symbol package,
  with optional referenced external cells and support for symbol-only cells.

## Install

Download the Linux x86-64 archive from [Releases](https://github.com/rich-netlist/rnl-virtuoso/releases),
then extract it on the Virtuoso host:

```sh
mkdir -p ~/tools
tar -xzf rnl-virtuoso-0.6.0-linux-amd64.tar.gz -C ~/tools
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

Check and Save the schematic hierarchy, then select **RNL → Export RNL...**.
Choose a **new output directory**. **Spectre (simulation)** is recommended for
sources, switches and Verilog-A models; CDL is an LVS export and may omit them.
Keep the generated netlist, symbol/style files and report together.
External model references remain external; this does not package a runnable simulation deck.

From **CIW → File → Export → RNL...**, choose **Cell** and a library/cell without
opening its editor, or **Library** to export all schematic and symbol-only cells.
The library output shares `netlists/`, `symbols.rnl` and the two presentation
packages; open a `<cell>.scs` / `.sp` entry. The report lists entries, native
definitions and exported symbols. The external-cell option preserves interfaces
and symbols when off, and includes referenced schematic definitions when on.

See [export details](docs/export.md) for options, conversion rules and limitations.

## Build and test

Building requires Go 1.22+ and Python 3.8+ on the development machine only:

```sh
python scripts/package.py dist/rnl-virtuoso-0.6.0-linux-amd64.tar.gz
python scripts/test.py
```

The package contains a static Linux x86-64 backend. `--go /path/to/go` selects a
compiler. To enable drawing/checker assertions, pass `--rnl /path/to/rnl` to the
test script. See [development notes](docs/development.md).

## License

Code, tests and documentation: [MIT](LICENSE). Bundled symbol/style packages:
CC0-1.0. The Go runtime/standard-library notice is in [Go-LICENSE.txt](Go-LICENSE.txt).
Cadence software, library symbols and user designs are not included.
