# Export with the headless CLI

Resolve the required target, access, project, environment and output information
from SKILL.md first. Run commands on that machine using an available local shell
or authorized remote-execution tool. No bridge is required. Check the installed
`bin/rnl-virtuoso export --help` before using this interface.

## One command

These are POSIX-shell examples. Set `INSTALL_DIR`, `PROJECT_DIR`, `LIBRARY`,
`CELL` and `OUTPUT_DIR` to the confirmed values on the execution machine; these
names are placeholders, not assumed pre-existing environment variables. Adapt
quoting to the actual command runner. With the Cadence environment loaded:

```sh
"$INSTALL_DIR/bin/rnl-virtuoso" export --project "$PROJECT_DIR" --lib "$LIBRARY" --cell "$CELL" --out "$OUTPUT_DIR" --auto-check-save=false
```

For an explicitly requested whole-library export:

```sh
"$INSTALL_DIR/bin/rnl-virtuoso" export --project "$PROJECT_DIR" --lib "$LIBRARY" --out "$OUTPUT_DIR" --auto-check-save=false
```

Omitting `--cell` selects the whole library. The tool generates and executes its
SKILL internally; do not reconstruct a driver or timer loop. External cell bodies
are excluded by default for a library; add `--external=true` when requested.
Relative output paths are relative to `--project`. Output/log paths with spaces
or shell metacharacters are rejected before launch. Existing outputs are refused.
The default 30-minute timeout covers the whole run; increase it for large jobs
when justified. Keep the command running through completion.

## Cadence environment

Ask the user for the environment-loading method if it is unknown. Establish the
required Cadence, PDK and license setup and the shell that interprets it. Do not
assume a launch alias, a site's shared files or a previously used machine's setup.
If the user points to a startup file or alias, inspect its definition to identify
the environment setup separately from any interactive application launch.

For confirmed C shell setup files, pass one `--env-csh` per file, in order:

```sh
"$INSTALL_DIR/bin/rnl-virtuoso" export --project "$PROJECT_DIR" --lib "$LIBRARY" --cell "$CELL" --out "$OUTPUT_DIR" --auto-check-save=false --env-csh "$CADENCE_SETUP" --env-csh "$PDK_SETUP"
```

The setup variables above must contain the actual paths supplied or verified for
this target. `--env-csh` uses C shell even if the caller uses another shell; it
replaces any configured source list. For a different setup language, use its
matching shell to establish the environment before invoking the CLI. Do not pass
a Bash setup file as C shell or execute an alias that also starts a GUI session.
The CLI restores the selected project directory after setup and manages its own
virtual display.

If the user wants persistent defaults, agree on a config file and use
`--config CONFIG_FILE`. Its JSON accepts `env_csh` (ordered array of C shell setup
paths) and optional `virtuoso` (executable name/path). Inspect installed
`docs/headless.md` for automatic config discovery; do not invent or modify a
configuration location. Use per-invocation flags when persistence is not requested.
`--no-config` ignores automatic configuration. Source only trusted setup files.

The CLI skips `.cdsinit` by default. If project CDF/PDK initialization is required,
establish the needed startup behavior with the user; `--cdsinit` enables normal
startup. Do not guess that skipping it is suitable for an unknown PDK, or enable
unrelated interactive/bridge startup to work around an unexplained failure.

## Options and result handling

- View lists: `--schematic-views "schematic schematic_ideal"` and
  `--symbol-views "symbol symbol_alt"`, searched left to right; defaults are
  `schematic` and `symbol`. `--view` overrides only the selected cell's root view.
- `--format spectre|cdl`, `--file-mode separate|combined` (cell only),
  `--hierarchy=false` (cell only), `--external=true|false`, `--scale 10`.
  CDL is LVS output and can omit simulation sources.
- `--selection-box=false` uses graphic extent instead of native selection boxes;
  `--symbols` enables the additional `symbols.rnl` collection.
- Boolean flags use `--flag=false`, not `--flag false`. Keep source writes off
  unless authorized. A headless process cannot export another session's unsaved edits.
- stdout is one JSON result. stderr gives the log path, or native messages with
  `--verbose`. Capture both; do not parse console text as the result or use eval.
- Exit 0 / `succeeded` is complete; exit 3 / `partial` requires inspecting and
  reporting `library-report.json` failures while retaining usable output.
- Exit 2 / `failed` covers validation/startup/export failures. Read `message`,
  `log_directory`, `launcher.log`, `CDS.log`, and `error_file`/native work paths
  when available. Failure before launch may have no logs or report.
- Exit 124 / `timed_out` and 130 / `interrupted` are not successful exports. Logs
  and partial artifacts remain. The CLI terminates its dedicated process group;
  it does not stop another desktop session.
- If remote transport disconnects, inspect the known output/log directory and
  process before retrying. A broken connection is not proof that export stopped.
  Do not blindly start a duplicate export.

The current workspace layout keeps launcher logs in `<output>/.rnl/launcher/`,
native jobs in `.rnl/work/`, and scratch/staging inside the output tree. Keep this
hidden directory when retrieving diagnostics. `--logs` may select another
subdirectory below output/.rnl, outside work/tmp; external paths are rejected.
Read installed `docs/headless.md` or `export --help` to verify layout and flags
before using an older installation.
