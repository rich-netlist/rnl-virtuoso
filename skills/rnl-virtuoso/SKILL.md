---
name: rnl-virtuoso
description: Export a Virtuoso schematic cell or library to RNL through an existing Virtuoso bridge session. Check the remote exporter installation, call its SKILL API, and retrieve results; install a missing exporter from GitHub only with user consent.
---

# Export Virtuoso to RNL

Use the **virtuoso** skill for the bridge connection, target session and file
transfer. This skill does not contain or install the bridge. If that skill or a
working bridge is unavailable, resolve it before export. Use **eda-servers** when
applicable for server identity/access, but install this exporter in the personal
account. For inspecting/rendering RNL, use the **rnl** skill separately.

## Check the target installation

1. Reuse the intended host, account and Virtuoso port from context; do not
   silently switch to the bridge's default session. Remote checks must use the
   same host/account as Virtuoso.
2. Through the bridge evaluate:
   ```skill
   if(isCallable('rnlExporterInfo) then rnlExporterInfo() else nil)
   ```
   API version 1 is supported. If loaded, use its `install_directory`. Otherwise
   check the specified installation and then
   `~/tools/rnl-virtuoso/rnl-virtuoso.il` on the remote host. Honor an existing
   `rnlInstallDirectory` from an older plugin. Verify the companion
   `bin/rnl-virtuoso --version`; read installed `docs/skill-api.md` when available.
   Use `tool_version` and the installed documentation to decide which optional
   keywords are supported; API 1 alone does not imply every newer option exists.
   A file alone does not prove a working installation.
3. For an existing compatible installation, load the normal entry point once if
   needed, then query info. Do not update it merely because a newer release
   exists. Do not reload while an export is active.
4. If missing, prepare the exact host, personal path, latest GitHub release and
   source using [installation](references/installation.md). **Obtain consent
   before installing on that host**, unless the current request/session already
   explicitly authorizes that installation. This gate comes from this skill's
   intended workflow. An older installation lacking API 1 needs an approved
   upgrade, not a silent replacement or guessed calls.

Source: https://github.com/rich-netlist/rnl-virtuoso. Fetch its latest published
release when installation is needed. Do not pin this skill to a release, bundle
an exporter binary/source copy, or include bridge assets. API 1 first appeared
in 0.8.0.

## Choose and start the export

Use context to determine library/cell, remote destination and options. Choose a
fresh directory if none was specified. Default cell exports to Spectre, hierarchy
enabled, external cells enabled and separate files. For a library, export all
cells with external cells disabled unless requested. Retain user overrides.

Exporter 0.9.2+ defaults to automatically Check & Save stale source schematics,
including current edits. If source writes are not authorized, pass
`?autoCheckSave nil` (requires 0.9.2+) and report stale views; otherwise retain the
default. Never modify library mappings, startup files or source circuitry to
make export pass without authorization. Real missing references and mismatches
remain failures. On older versions, never run manual Check & Save without consent.

```skill
load("~/tools/rnl-virtuoso/rnl-virtuoso.il") ; only if needed

; Required positional arguments; all options below are the defaults.
rnlExportCell("myLib" "myCell" "/home/user/exports/myCell"
  ?format "spectre"
  ?hierarchy t ?external t ?fileMode "separate" ?scale 10.0)

; Whole library: all schematic and symbol-only cells, fixed per-cell files.
rnlExportLibrary("myLib" "/home/user/exports/myLib"
  ?format "spectre" ?external nil ?scale 10.0)
```

Call **one** entry point for the requested scope. `format` is `"spectre"` or
`"cdl"`; CDL is LVS output and may omit sources. `fileMode` is `"separate"` or
`"combined"` (cell only); `hierarchy`/`external` are SKILL `t`/`nil`. Paths are
remote. Existing outputs are refused. Save the returned job ID string to poll;
it is not a completed result. Startup errors yield no ID.

Pass these optional controls only when the installed exporter supports them:

- 0.9.0+: `?schematicViews` and `?symbolViews` are space-separated view lists,
  searched left to right. Defaults are `"schematic"` and `"symbol"`; for example,
  `?schematicViews "schematic schematic_ideal"` adds a fallback schematic view.
  Leave `?view` unset for this selection; an explicit `?view` overrides the top.
- 0.9.1+: `?exportSelectionBox t` (default) uses the union of the native symbol
  selection boxes; `nil` uses the non-text symbol graphic extent.
- 0.9.4+: `?exportSymbols nil` (default) omits the extra `symbols.rnl` collection;
  `t` adds it. This applies to both cell modes and whole-library export, without
  changing embedded schematic symbols. Older exporters need an authorized upgrade
  when this control is required; do not pass an unsupported keyword.

Use the bridge skill's configured client in a temporary Python script, with
`client.execute_skill(expression)` and `client.download_file(remote, local)`.
Build SKILL string literals with escaped quotes/backslashes (for example
`json.dumps(value, ensure_ascii=False)` for ordinary names/paths, rejecting
control characters); never interpolate unescaped user text. Bridge `output`
contains printed SKILL data: check status/errors first, and do not use Python
`eval` on it. A convenient narrow query is:

```skill
cadr(assoc("state" rnlExportStatus("/tmp/job-id-from-start")))
```

## Wait and collect

- Query `rnlExportStatus(id)` in separate bridge calls, roughly every 2–5 seconds
  initially and less often for long jobs. Return control between calls; SKILL
  wait loops can prevent IPC callbacks. Keep the user informed during long runs.
- Results are association lists of string keys and two-element lists. `state`
  is `running`, `succeeded`, `partial`, or `failed`. `phase`, `completed_cells`,
  `total_cells`, `failed_cells`, and `current_cell` give progress. `report`,
  `output_directory`, `work_directory`, and `error_file` locate artifacts. These
  APIs do not create completion dialogs.
- `partial` means the library merged but some cells failed. Read
  `library-report.json`, report failed cells/reasons, and preserve usable output.
  Exit code zero alone is not complete success.
- On failure, retrieve `error_file` if present and relevant native logs in
  `work_directory`. A failed run may have no report. Do not repeatedly restart
  an unchanged failing job.
- On bridge timeout, check `rnlExporterInfo()` and the known job before retrying
  a start. Only one export runs per session; timeout does not cancel it. IDs
  persist across later API jobs/reloads, not Virtuoso restarts. Menu jobs are
  visible as busy but have no public API history.
- After success/partial, inspect the report and retrieve output when requested.
  Copy/archive the **entire directory** with relative paths intact, including
  external-library subdirectories and symbol/style files.

In 0.9.4+, `cell-symbol.rnl` is no longer emitted. Symbol-only library cells
always have their own `<cell>.rnl` output, listed in `symbol_only_files`, even
when the optional collection is off. `export_symbols` records the selected option;
`symbol_file` is null when no collection was requested. Its absence is expected,
not an export failure. Keep the primary symbol-only files with the output.

Ported files have no native includes: their caller must load dependencies once.
Portless tops include descendants once. Child symbols are inherited from exported
subcircuits; omitted cells retain local interfaces. Native model/Verilog-A
references remain external, and export does not run simulation. Report source,
options, status, result path, and relevant warnings.
