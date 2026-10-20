# Export resources

`eda-symbol.rnl` and `eda-style.rnl` are pinned snapshots of the split packages
provided for this integration from branch `feat/virtuoso-symbols`, on 2026-10-20.
They are CC0-1.0, as recorded in the repository's REUSE annotations and the
plugin's license notice. The plugin carries these snapshots in both source and
packaged installations so it does not depend on another worktree's location or
the former combined `libraries/eda-style` package.

The package versions are **0.1**; their RNL language version is **0.3**. Exported
files reference both packages explicitly. Replace the `eda-style` reference to
apply another presentation without editing exported symbol geometry. Pin marks
are supplied by the style package, not by the converter's custom symbols.

The source snapshot includes SPICE instance-letter rules (`I*`, `V*`, etc.).
Exported **Spectre** copies omit that section and keep model/type rules:
Spectre instance names do not identify device types. CDL copies keep the SPICE
rules. This adaptation applies to both cell and shared-library exports.

`symbol-orientations.json` describes the 18 changed symbol interfaces. Its
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
