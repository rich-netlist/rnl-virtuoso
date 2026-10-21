# SKILL API (version 1)

Available since rnl-virtuoso 0.8.0. Load the normal entry point once:

```skill
load("~/tools/rnl-virtuoso/rnl-virtuoso.il")
rnlExporterInfo()
```

These functions use the same native exporters, saved-view checks and conversion
as the menus. They do not open an editor, save source designs or show completion
dialogs. The current Virtuoso session supplies `cds.lib`, Cadence tools and
licenses. Output paths refer to that machine. Existing output is never overwritten.

## Export a cell

```skill
job = rnlExportCell("myLib" "myCell" "/home/user/exports/myCell"
  ?view "schematic" ?format "spectre"
  ?hierarchy t ?external t ?fileMode "separate" ?scale 10.0)
```

Only library, cell and destination are required. The example lists all defaults.

| Keyword | Values and meaning |
| --- | --- |
| `view` | Schematic view name, default `"schematic"` |
| `format` | `"spectre"` or `"cdl"`; CDL is LVS output and may omit simulation sources |
| `hierarchy` | `t` includes descendants; `nil` emits the current level with child interfaces |
| `external` | `t` includes referenced foreign cells; `nil` retains their interfaces only |
| `fileMode` | `"separate"` (per cell) or `"combined"` |
| `scale` | Positive EDIF units per RNL unit, default `10.0` |

`external` affects descendant definitions only when hierarchy is enabled. Native
netlisting may still traverse omitted descendants. Fixed switch/stop view lists
and conversion limits are described in [export details](export.md).

## Export a library

```skill
job = rnlExportLibrary("myLib" "/home/user/exports/myLib"
  ?format "spectre" ?external nil ?scale 10.0)
```

This includes every schematic and symbol-only cell, always in separate files.
External cells are excluded by default, matching the CIW form. There is no
hierarchy or file-mode option for a library. Cell failures are recorded and
skipped; a successful merge can therefore be **partial**.

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
