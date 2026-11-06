---
name: rnl-virtuoso
description: Export Virtuoso schematic cells or libraries to RNL using a headless command or an available live-session connection. Gather missing environment details from the user, check the exporter, and install or upgrade only with consent.
---

# Export Virtuoso to RNL

Obtain the exporter from https://github.com/rich-netlist/rnl-virtuoso.
This skill is self-contained: it requires no other agent skill, server profile,
specific bridge client or bundled runtime. Use the execution and file-transfer
tools actually available in the current environment. Cadence Virtuoso and its
licenses must be available on the machine that performs the export.

For optional inspection or rendering of the result, the **rnl** skill is available
from https://github.com/rich-netlist/rnl. It is not required for exporting.

## Establish the target and missing information

Use information supplied in this conversation or verified on the intended target.
Ask the user for any required information still unknown before the dependent step;
do not guess a host, account, session, installation path or environment setup from
this skill, another agent's setup, the login directory or a placeholder example.
Ask only for missing information, grouping related questions where practical.

| Required information | What to establish |
|---|---|
| Execution target and access | Local or remote machine; for remote work, host/account and an available authorized connection and file-transfer method. Obtain any required SSH port, jump host or access instructions from the user. Use the environment's credential flow rather than asking for passwords/private keys in chat. |
| Design and scope | Source library; selected cell or whole library. A missing cell name does not by itself authorize whole-library export. |
| Project and Cadence setup | Project directory and its `cds.lib`; how to load the required Cadence/PDK/license environment, including shell type and ordered setup files if needed. Confirm whether project startup initialization is required. |
| Existing exporter | Installation directory or a verified executable on the target's PATH, or confirmation that it is not installed. If its location cannot be established, ask; not being on PATH does not prove absence. |
| Output and delivery | New output directory on the execution machine; if results are to be copied back, the destination and usable transfer method. |
| Live session, when requested | Exact session/endpoint and an available way to evaluate SKILL in it, including the client's actual calling interface. A port number alone is insufficient. |

Options with documented defaults below need no extra question unless the request
makes the intended value ambiguous. Preserve all user-specified options.

## Choose the execution mode

- **Headless CLI:** preferred for saved designs when no live session is required.
  Starts its own Virtuoso, waits and exits. Read [headless use](references/headless.md).
- **Existing live-session connection:** use when the user requests a particular
  session or needs its loaded state. Read [bridge use](references/bridge.md).
  This is an optional transport, not a dependency on a bridge skill. If no usable
  connection is available, ask the user to provide access or choose headless mode.
  Do not silently switch sessions/modes or install a bridge.

An explicit headless request takes precedence over an earlier session reference.
Independent processes read saved design files, not another session's unsaved
edits. Do not save source edits merely to make them visible without authorization.

## Check the exporter

At the confirmed installation, inspect `bin/rnl-virtuoso -version` and its docs.
Version 0.9.5 introduced the headless CLI, `blocking_export` and `output_workspace`.
For new installations or authorized upgrades, discover the latest stable release
rather than pinning this minimum version. For headless mode require `bin/rnl-virtuoso export --help` to succeed; a version
string or API version alone does not prove the subcommand exists. In a live
session, query `rnlExporterInfo()` as described in the bridge reference. When all
exporter files must stay in the output directory, require `output_workspace: t`.

Reuse a compatible installation. If missing or lacking required capabilities,
follow [installation](references/installation.md): determine the upstream build
and user-approved destination, then obtain installation/upgrade consent unless
already authorized. An export request alone is not installation authorization.
Do not modify shell startup files, `.cdsinit`, `cds.lib` or design circuitry to
force an export.

## Options and results

Defaults: Spectre, full hierarchy, separate files; external cells enabled for a
cell export, disabled for a library. A library exports all schematic and
symbol-only cells with per-cell files. Use a new output directory without spaces
or shell metacharacters (a native netlister limitation). Exporters with
`output_workspace` capability keep all work files and diagnostics under its `.rnl/`
subdirectory, including failures; directory existence does not mean completion.

The exporter defaults to automatically Check & Save stale schematics. Without
source-write authorization, use CLI `--auto-check-save=false` or API
`?autoCheckSave nil`; report stale-source errors instead of repairing them.

Inspect the report; distinguish complete success from a partial library export.
When retrieving results, copy the entire output directory with relative paths
intact, including external-library and presentation-package files. Report the
source/target, mode/options, status, failed cells/reasons and output/log paths.

The optional `symbols.rnl` collection is off by default; `symbol_file: null` is
expected. `cell-symbol.rnl` is no longer emitted. Symbol-only cells still have
`<cell>.rnl` files, listed in `symbol_only_files`; keep them. Ported files have no
native includes; callers load dependencies once. Portless tops include descendants
once. Native model and Verilog-A references remain external. Export success does
not establish simulation success.
