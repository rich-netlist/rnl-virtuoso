# SKILL API (version 1)

Available since rnl-virtuoso 0.8.0. Load the normal entry point once:

```skill
load("~/tools/rnl-virtuoso/rnl-virtuoso.il")
rnlExporterInfo()
```

These functions use the same native exporters, connectivity checks and conversion
as the menus. They do not open an editor or show completion dialogs. By default
they Check & Save stale source schematics, including current edits. Use
`?autoCheckSave nil` when source writes are not authorized. The current Virtuoso session supplies `cds.lib`, Cadence tools and
licenses. Output paths refer to that machine. Existing output is never overwritten.

## Export a cell

```skill
job = rnlExportCell("myLib" "myCell" "/home/user/exports/myCell"
  ?schematicViews "schematic" ?symbolViews "symbol" ?format "spectre"
  ?hierarchy t ?external t ?fileMode "separate" ?scale 10.0 ?exportSymbols nil)
```

Only library, cell and destination are required. The example lists all defaults.

| Keyword | Values and meaning |
| --- | --- |
| `view` | Optional explicit top view; default `nil` selects by `schematicViews`. Overrides only the root, preserving older calls that specify `?view`. |
| `schematicViews` | Space-separated schematic view names, highest priority first; default `"schematic"`. Available since 0.9.0. |
| `symbolViews` | Space-separated symbol view names, highest priority first; default `"symbol"`. Available since 0.9.0. |
| `autoCheckSave` | `t` (default) checks and saves stale schematic connectivity, including instances whose terminal interfaces became stale after a master edit; `nil` rejects stale source views without saving. Since 0.9.2. Applies to cell and library exports. |
| `exportSymbols` | `nil` (default) omits the additional reusable collection; `t` writes `symbols.rnl`. Available in cell and library exports since 0.9.4. Does not alter embedded symbols. `cell-symbol.rnl` is never generated. |
| `exportSelectionBox` | `t` (default) uses the enclosing rectangle of all selection boxes in the selected Virtuoso symbol view; `nil` uses the non-text symbol graphics extent. Since 0.9.1. Applies to cell and library exports, including symbol-only cells. |
| `format` | `"spectre"` or `"cdl"`; CDL is LVS output and may omit simulation sources |
| `hierarchy` | `t` includes descendants; `nil` emits the current level with child interfaces |
| `external` | `t` includes referenced foreign cells; `nil` retains their interfaces only |
| `fileMode` | `"separate"` (per cell) or `"combined"` |
| `scale` | Positive EDIF units per RNL unit, default `10.0` |

`external` affects descendant definitions only when hierarchy is enabled. Native
netlisting may still traverse omitted descendants. View selection and conversion
limits are described in [export details](export.md).

## Export a library

```skill
job = rnlExportLibrary("myLib" "/home/user/exports/myLib"
  ?format "spectre" ?external nil ?scale 10.0
  ?schematicViews "schematic schematic_ideal" ?symbolViews "symbol")
```

This includes every schematic and symbol-only cell, always in separate files.
External cells are excluded by default, matching the CIW form. There is no
hierarchy or file-mode option for a library. Cell failures are recorded and
skipped; a successful merge can therefore be **partial**.

The `autoCheckSave`, `exportSelectionBox` and `exportSymbols` keywords also apply to library export.
Symbol-only library cells always export as `<cell>.rnl`, even when `exportSymbols`
is disabled. The library report lists them in `symbol_only_files`; `symbol_file`
is null when the optional collection is off. Selection boxes
are read from the selected symbol view's `instance/drawing` shapes; geometry,
labels and the cellview bbox are not used to expand them. If there are no native
selection boxes, RNL computes the graphic extent. Native box coordinates use
the same configured drawing scale as symbols and pins.

Both view-list keywords also apply to library export. The example adds
`schematic_ideal` as a fallback; the default remains `"schematic"`. Empty lists
are rejected and duplicates are removed without changing priority. A cell is
exported once, using its first existing schematic view; if none exists, its first
existing symbol view is exported. The same schematic order controls hierarchy
preflight, native Spectre/CDL netlisting and EDIF hierarchy expansion. Native
model stop views remain unchanged. Referenced instances keep their placed pin
coordinates for wiring; their exported symbol definition uses the first available
configured symbol view, falling back to the placed symbol when no candidate exists.
Terminal interfaces are compared by expanded member names, allowing reversed bus
ranges and equivalent grouping without changing the view or native netlist order.
Actual missing/extra bits fail with the cell/views and missing member names.
Hierarchical EDIF page-port aliases are resolved from explicit joined members,
keeping page bus direction and labels separate from symbol/native port order
(0.9.3+).
Automatic Check & Save acquires append access only when needed and reports lock,
check and save failures. Managed views use Cadence batch auto-checkout to avoid
interactive checkout dialogs. It refreshes descendants before final parent snapshots;
missing references and unresolved connectivity remain failures. Symbol views are
never automatically edited or saved. Unsaved but current schematics must still
be saved explicitly.

## Observe completion

Export functions return a job ID (the native work directory) after preflight and
process launch. This means **started**, not completed. Startup errors raise a
SKILL error and return no ID. One export can run per session; concurrent starts
raise an error.

```skill
rnlExportStatus(job)
; or rnlExportStatus() for the most recent public-API job
cadr(assoc("state" rnlExportStatus(job)))
```

Status and info return association lists of two-element lists with string keys,
for example `(("state" "succeeded") ...)`. Use `cadr(assoc(...))` to read values.
Keep the ID: prior API jobs remain queryable after later jobs and repeated loads
in the same session. IDs are not recovered after Virtuoso restarts; menu/internal
calls are not registered in this API history.

| Status field | Meaning |
| --- | --- |
| `id`, `kind` | Job ID and `"cell"` / `"library"` |
| `state` | `"running"`, `"succeeded"`, `"partial"`, or `"failed"` |
| `phase` | `"cell"` for a single cell; `"cells"` / `"merge"` for a library |
| `exit_code` | `nil` until completion, then worker exit code; partial merges can return 0 |
| `output_directory`, `report` | Output and `export-report.json` / `library-report.json` |
| `work_directory`, `error_file` | Native diagnostics directory and worker `error.txt` path |
| `completed_cells`, `total_cells`, `failed_cells` | Attempt counts; completed includes failed attempts |
| `current_cell` | Current library cell while exporting cells, otherwise `nil` |

Paths identify expected artifacts; a failed job may have no final output/report.
After success or partial completion, read the report. The library report has
`complete`, `successful_cells` and `failed_cells` including reasons and logs.
Conversion success does not mean the circuit was simulated.

Return control to Virtuoso between polls so IPC callbacks can run. Bridge callers
should issue separate `execute_skill` calls every few seconds. Do not busy-wait
or sleep in a SKILL loop. On bridge timeout check `rnlExporterInfo()` before
relaunching: a timeout does not cancel an existing export.

`rnlExporterInfo()` returns `api_version` (integer 1), `tool_version`,
`install_directory`, `busy`, and `active_id`. The active ID can describe a menu
job; then wait for `busy` to clear instead of querying public job history.

## Output and agent use

Source-library cells are at the output root; foreign cells are under their library
names. Ported cells have no native includes: their caller must load dependencies.
Portless tops include descendants once. Exported child symbols live in their own
subcircuits; omitted children keep local interface symbols. Keep the entire
output directory together. Model references remain external.

Install the repository's `skills/rnl-virtuoso` directory into your agent's skills
directory (Codex: `~/.codex/skills/rnl-virtuoso`). It uses the existing `virtuoso`
bridge skill, checks the host installation, and requests consent before installing
a missing tool in the user's home. The skill contains instructions only; the
exporter and bridge are obtained separately.
