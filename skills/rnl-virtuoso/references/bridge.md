# Export through an existing live-session connection

This mode needs an already available way to evaluate SKILL in the requested
Virtuoso session. It does not depend on another agent skill, a named MCP tool,
a Python package or a particular client API. Use the actual connection's tool
schema/documentation. If the endpoint, account, calling interface or access
method is unknown, ask the user before connecting. Do not invent client methods,
use a default session, or install/start a bridge as part of export.

If no usable connection exists, ask whether the user will provide one or wants
headless export. A bridge port alone does not establish connectivity or the
protocol. File transfer can use a separate available, authorized method.

## Check the target installation

1. Verify the intended session identity. Evaluate this SKILL expression through
   the available interface, checking transport errors and SKILL diagnostics:
   ```skill
   if(isCallable('rnlExporterInfo) then rnlExporterInfo() else nil)
   ```
2. If loaded, use the returned `install_directory` and `tool_version`. Otherwise,
   establish the installation path from the user or verified session information
   such as `rnlInstallDirectory`; do not assume a filesystem location. At that
   location, verify `bin/rnl-virtuoso -version` and read `docs/skill-api.md`.
   API version 1 is supported, but it does not guarantee every newer keyword.
3. Load the normal `rnl-virtuoso.il` entry point if needed, using its confirmed
   absolute path, and query info again. Do not reload during an active export.
4. If the exporter is missing or incompatible, follow
   [installation](installation.md). Obtain installation/upgrade consent unless
   already authorized. Reuse a compatible installation without gratuitous upgrades.

## Start the requested export

Use the confirmed library, scope and output directory. The strings below are
placeholders to replace, not machine-specific defaults. All paths are on the
Virtuoso machine. Call only the entry point for the requested scope:

```skill
load("INSTALL_DIRECTORY/rnl-virtuoso.il") ; only if needed, replace with absolute path

; One cell, full hierarchy including foreign cells, separate files.
job = rnlExportCell("LIBRARY" "CELL" "NEW_OUTPUT_DIRECTORY"
  ?format "spectre" ?hierarchy t ?external t ?fileMode "separate"
  ?scale 10.0 ?autoCheckSave nil)

; Alternatively: whole library; foreign cell bodies excluded by default.
job = rnlExportLibrary("LIBRARY" "NEW_OUTPUT_DIRECTORY"
  ?format "spectre" ?external nil ?scale 10.0 ?autoCheckSave nil)
```

The default asynchronous call returns a job ID string, not a completed result.
Startup errors yield no ID. Save the actual returned ID for later queries; do
not rely on a transport retaining local client variables or reporting success
just because the request was accepted. Existing output directories are refused.

`format` accepts `"spectre"` or `"cdl"`; CDL is LVS output and may omit sources.
`fileMode` accepts `"separate"` or `"combined"` (cell only).
`hierarchy` and `external` use SKILL `t`/`nil`.

The exporter may automatically Check & Save stale source views. The examples
explicitly disable this. `?autoCheckSave` requires 0.9.2+; without source-write
authorization, verify that this control exists before exporting. Do not enable
it or manually save stale views just to get a successful export.

Pass optional controls only when the installed exporter supports them:

- 0.9.0+: `?schematicViews` and `?symbolViews`: space-separated view lists,
  searched left to right; defaults `"schematic"` and `"symbol"`. An explicit
  `?view` overrides the selected cell's top view.
- 0.9.1+: `?exportSelectionBox t` (default): union of native selection boxes;
  `nil` uses non-text graphic extent.
- 0.9.4+: `?exportSymbols nil` (default): omit the extra `symbols.rnl` collection;
  `t` adds it without changing embedded symbols.
- 0.10.0+: `?symbolLibraries list("/path/pdk.rnl" …)`: RNL symbol libraries,
  highest priority first; `nil` (default) uses the bundled `eda-symbol`. Pass
  only package paths the user names; a `<package>.virtuoso.json` beside a
  package maps Virtuoso cells to its symbols.
- If `rnlExporterInfo()` reports `blocking_export` true, `?wait t` returns final
  status and `rnlExportWait(id)` waits for an existing job. Use this only if the
  transport supports a call lasting the entire export; asynchronous polling
  avoids ordinary bridge request timeouts.

Quote user-supplied strings for SKILL and for the transport separately. Escape
backslashes/quotes and reject control characters; do not insert raw text into
expressions. Interpret results according to the actual tool's schema. SKILL
printed data is not JSON or executable Python; do not evaluate it as code.

## Wait and collect

- Query `rnlExportStatus("JOB_ID_RETURNED_BY_START")` in separate calls, initially
  a few seconds apart and less often for long jobs. Return control between calls;
  an ad hoc SKILL wait loop can prevent completion callbacks. Keep the user informed.
- Results are association lists with string keys and two-element lists. `state`
  is `running`, `succeeded`, `partial` or `failed`. `phase`, `completed_cells`,
  `total_cells`, `failed_cells` and `current_cell` give progress. `report`,
  `output_directory`, `work_directory` and `error_file` locate artifacts.
  To query only the state, evaluate:
  ```skill
  cadr(assoc("state" rnlExportStatus("JOB_ID_RETURNED_BY_START")))
  ```
- `partial` means the library merged but some cells failed. Read
  `library-report.json`, report failed cells/reasons and retain usable output.
- On failure retrieve `error_file`, if present, and relevant native logs from
  `work_directory`. There may be no report. Do not restart an unchanged failing job.
- On transport timeout inspect `rnlExporterInfo()` and the known job before
  starting anything again. Only one export runs per session; timeout does not
  cancel it. Job IDs survive later exports/reloads, not Virtuoso restarts.
  GUI-started jobs appear busy but have no public API history.
- After success/partial, inspect the report and copy the entire directory when
  requested. Ask for a missing delivery path or transfer method. Retain relative
  paths, external-library directories and symbol/style files.

Apply the output interpretation and reporting rules in SKILL.md. Export does not
run simulation or make external native models self-contained.
