# Export resources

`eda-symbol.rnl` and `eda-style.rnl` are byte-for-byte snapshots of the public
packages `eda-symbol` 0.2 and `eda-style` 0.2 of
[rnl-packages](https://github.com/rich-netlist/rnl-packages). They are CC0-1.0,
as recorded in the repository's REUSE annotations and the plugin's license
notice. The plugin carries these snapshots in both source and packaged
installations, and exported files reference them with `path=`.

The package versions are **0.2**; their RNL language version is **0.4**. Exported
files name the standard collection `rnl-std-all` 0.4 first, then both packages.
Replace the `eda-style` reference to apply another presentation without editing
exported symbol geometry. Pin marks are supplied by the style package, not by the
converter's custom symbols.

The pins of `eda-symbol` 0.2 keep the coordinates of 0.1 and bind the terminal
names of the standard series 0.4 (eda-only symbols for masters without standard
terminal names stay positional). Its SPICE instance-letter rules (`I*`, `V*`,
etc.) are in a record with `language="spice"`: they act only on SPICE sections,
never on Spectre instance names, so Spectre and CDL exports ship the same copy.

`eda-symbol.virtuoso.json` is the mapping of the bundled symbol library: the
exporter's configuration of which Virtuoso cells its symbols draw, kept beside the
package because it is not RNL (RNL 0.4, 16.4). Every symbol library may have one,
named `<package file without .rnl>.virtuoso.json`:

- `schema` 1 and `package`, the name and version the package declares;
- `cells`: `library`, `cell` and `symbol`, optionally with the cell's own
  `terminals`, `package_to_source` and `align_pins`;
- `terminals`: the terminal names that each symbol with named pins binds, in node
  order; the converter writes them as `@SET-SYMBOL(terminals=[…])`;
- `symbols`: the 18 changed symbol interfaces, with their compensation;
- `ports`: the glyphs copied for input and output schematic pins.

Its
`package_to_source` is the compensation applied to the **new package's local
coordinates** before the instance's Virtuoso/EDIF orientation. For example,
`DIODE` uses `R270` and `VOLTAGE_CONTROLLED_SWITCH` uses `MYR90`. `R90` means
`(x,y) -> (-y,x)`; `MY` negates x and `MX` negates y. Coordinate formulas and
the actual package pins decide these rules, including the legacy SCHOTTKY
anode at `(0,3)`, which differs from the handoff document's description.

`align_pins` optionally chooses which positional terminals set the translation.
Use it for changes that moved individual pins, such as a transmission line's
reference terminals or a flip-flop's inputs. The drawing keeps the existing
EDIF interior route vertices and attaches the ends to terminal references, so
replacing a symbol does not leave a stub at its old pin coordinates. A rigid
transform uses all pins by default. Unknown symbols use `R0`; the rules apply to primitives mapped to
package symbols. Custom EDIF geometry retains its source orientation.

When replacing the package snapshots, review this table against their actual
pin coordinates and run the integration tests, including eight rotations and
reflections of standard diode and relay drawings.
