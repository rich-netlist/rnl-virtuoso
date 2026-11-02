---
name: rnl-virtuoso
description: Export saved Virtuoso schematic cells or libraries to RNL with the headless CLI, or use an existing Virtuoso bridge session. Check the installed exporter and retrieve results; install or upgrade from upstream only with user consent.
---

# Export Virtuoso to RNL

The exporter is obtained from https://github.com/rich-netlist/rnl-virtuoso;
this skill contains instructions only, not runtime code or bridge assets.
Use **eda-servers** when applicable for host identity and environment setup.
Use **rnl** separately to inspect or render exported content.

## Choose the execution mode

- **Headless CLI:** preferred for saved-on-disk designs when no particular live
  session is required. It starts its own Virtuoso, waits and exits; no bridge
  installation or connection is necessary. Read [headless use](references/headless.md).
- **Existing bridge:** use when the user names a live session/port or needs that
  session's loaded environment or current edits. Use the **virtuoso** skill and
  [bridge use](references/bridge.md). Do not switch away from a requested session
  or silently fall back between modes. If the user explicitly asks for headless
  operation, that choice takes precedence over an earlier session reference.

Independent headless processes read saved design files, not another session's
unsaved edits. Do not save source edits merely to make them visible without
existing authorization. Headless execution still requires Cadence, licenses,
correct PDK initialization and the intended project's `cds.lib`.

## Check installation before export

Reuse the requested host/account and install path, otherwise check
`~/tools/rnl-virtuoso`. On that host inspect `bin/rnl-virtuoso -version` and the
installed documentation. For headless capability, require
`bin/rnl-virtuoso export --help` to succeed; API version 1 or a version string
alone does not prove this subcommand exists. The command is newer than the
published 0.9.4 interface. Bridge mode additionally verifies `rnlExporterInfo()`
in the intended session, as described in its reference.

Reuse an existing compatible installation. Do not update it just because a newer
release exists. If missing or missing a required capability, resolve the exact
upstream version and personal installation path with [installation](references/installation.md),
then obtain installation/upgrade consent unless already authorized in the current
request/session. This skill requires that consent; an export request alone is not
installation authorization. Do not install a bridge for headless use. Do not
modify `.cshrc`, `.cdsinit`, `cds.lib` or design circuitry to force an export.

## Choose options and collect results

Honor user options. Defaults: Spectre, full hierarchy and separate files;
external cells enabled for a cell export, disabled for a library. Use a fresh
output directory. A library always exports per-cell files and includes symbol-only
cells. Inspect installed help/docs for supported view-list and symbol options.

GUI/API and CLI default to automatically Check & Save stale schematics. If source
writes are not authorized, use CLI `--auto-check-save=false` or API
`?autoCheckSave nil`; report stale-source errors rather than repairing them.

After completion, inspect the report and distinguish complete success from a
partial library export. Retrieve the entire output directory with relative paths
intact when requested, including external-library directories and presentation
packages. Report host, source, mode/options, status, failed cells/reasons, output
and log paths. Export success does not establish electrical simulation success.

The optional `symbols.rnl` collection is off by default; `symbol_file: null` is
expected. `cell-symbol.rnl` is no longer emitted. Pure symbol-only library cells
still have their own `<cell>.rnl` files, listed in `symbol_only_files`; keep them.
Ported files have no native includes; callers load their dependencies once.
Portless tops include descendants once. Native model and Verilog-A references
remain external.
