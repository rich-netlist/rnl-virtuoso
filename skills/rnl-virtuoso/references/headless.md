# Export with the headless CLI

First verify the remote installation supports `bin/rnl-virtuoso export --help`.
Do not require or start a bridge. Use SSH/SCP or equivalent remote execution and
file transfer on the intended host/account. Confirm the project directory and
its `cds.lib`, rather than assuming the login directory selects the right design.

## One command

With Cadence environment defaults configured or already loaded:

```sh
~/tools/rnl-virtuoso/bin/rnl-virtuoso export --project /path/project --lib myLib --cell myCell --out /path/new-output --auto-check-save=false
~/tools/rnl-virtuoso/bin/rnl-virtuoso export --project /path/project --lib myLib --out /path/new-library-output --auto-check-save=false
```

Omitting `--cell` selects the whole library. The tool generates/executes its SKILL
internally; do not reconstruct a heredoc, timer loop or custom driver. The second
example excludes foreign cell bodies by default; add `--external=true` when
requested. Relative output paths are relative to `--project`. Existing outputs
are refused. The default 30-minute timeout covers the whole run; increase it for
large libraries when justified. Keep the command running through completion.

## Environment discovery

Read the user's known environment setup or the **eda-servers** server profile.
If needed inspect the relevant launch alias in `.cshrc` and its sourced files.
An alias such as `ic6_sui` may execute `source ...; source ...; virtuoso &`; pass
only its environment file paths, in the same order:

```sh
.../bin/rnl-virtuoso export --project /path/project --lib myLib --cell myCell --out /path/new-output --auto-check-save=false --env-csh /path/cadence.cshrc --env-csh /path/site.cshrc
```

`--env-csh` is repeatable and replaces any configured source list. This uses C
shell even if the SSH caller uses Bash. Do not source C shell files in Bash or
execute a launch alias that opens another interactive session. The CLI restores
the selected project directory after setup and manages its own virtual display.

When the user wants persistent defaults, configure
`~/.config/rnl-virtuoso/config.json` (or `$XDG_CONFIG_HOME/...`) with `env_csh`
(array of setup paths) and optional `virtuoso` (executable name/path). Otherwise
prefer per-invocation flags; do not change global shell startup files. Use
`--config FILE` for a separate config, or `--no-config` to ignore automatic defaults.
Only source trusted site/user environment scripts.

The CLI skips `.cdsinit` by default. If the project depends on startup CDF/PDK
initialization, determine what is needed; `--cdsinit` explicitly enables the normal
startup behavior. Do not enable unrelated interactive/bridge startup merely to
resolve a headless error. See installed `docs/headless.md` for all flags.

## Options and result handling

- View lists: `--schematic-views "schematic schematic_ideal"` and
  `--symbol-views "symbol symbol_alt"`, searched left to right; `--view` overrides
  only the selected cell's root view.
- `--format spectre|cdl`, `--file-mode separate|combined` (cell only),
  `--hierarchy=false` (cell only), `--external=true|false`, `--scale 10`.
- `--selection-box=false` uses graphic extent instead of native selection boxes;
  `--symbols` enables the additional `symbols.rnl` collection.
- Boolean flags use `--flag=false`, not `--flag false`. Keep source writes off
  unless already authorized. A headless process cannot export unsaved edits from
  another session.
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
- If a remote transport disconnects, inspect the known output/log directory and
  dedicated process before retrying. A broken SSH connection is not proof that
  the export stopped. Do not blindly start a duplicate export.
