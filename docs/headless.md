# One-command headless export

Available since 0.9.5. Check `bin/rnl-virtuoso export --help` on the installed
tool before using it; older archives may lack the command.
The CLI is part of the existing static Go executable. It needs neither Python nor
a bridge, and generates the SKILL driver internally.

```sh
~/tools/rnl-virtuoso/bin/rnl-virtuoso export --lib myLib --cell myCell --out ./myCell_rnl
~/tools/rnl-virtuoso/bin/rnl-virtuoso export --lib myLib --out ./myLib_rnl
```

The current directory must contain the project's `cds.lib`, or pass
`--project /path/to/project`. Relative output paths are resolved against that project directory; relative
`--logs` paths are resolved against the output directory. Native output/log paths must not contain spaces or shell metacharacters;
unsupported paths are rejected before launch. Existing outputs are refused; missing output parent
directories are created. Omitting `--cell` deliberately selects the whole library.

## Cadence environment

If `virtuoso` and the required PDK/license environment are already available,
no extra setup is needed. Otherwise source the site's C shell setup files:

```sh
rnl-virtuoso export --project /path/project --lib myLib --cell myCell --out ./myCell_rnl --env-csh /path/cadence.cshrc --env-csh /path/site.cshrc
```

For repeated use, put just the environment defaults in
`~/.config/rnl-virtuoso/config.json` (or `$XDG_CONFIG_HOME/rnl-virtuoso/config.json`):

```json
{
  "env_csh": ["/path/cadence.cshrc", "/path/site.cshrc"],
  "virtuoso": "virtuoso"
}
```

This makes the shorter commands above sufficient. `--config FILE` selects another
configuration; `--no-config` ignores automatic configuration. Explicit
`--env-csh` options replace the configured list; they do not append to it.
`--virtuoso` overrides the configured executable. Config-relative setup paths
are relative to the config file; command-line setup paths are relative to the
invoking directory. `~` expands to the current user's home. No startup file is
modified and no alias is evaluated. Only trusted environment scripts should be
selected: they execute as the invoking user.

If a site alias loads environments and then launches `virtuoso &`, use its
`source` files here rather than executing the alias: pass each C shell script it
sources with `--env-csh`, in the alias's order. Verify the current site definition
before reuse.

After setup, the launcher enters the specified project and clears inherited
external-display/session bindings. It uses `-nograph -nocdsinit`; Cadence provides
its own virtual display. Add `--cdsinit` only if the project needs its normal
startup customizations. Designs must be saved on disk: edits in another open
Virtuoso process are not visible to this separate process.

## Options

Run `rnl-virtuoso export --help` for the full list. Boolean flags use
`--flag=false` or `--flag=true`, not a separate word after the flag.

| Option | Default / meaning |
|---|---|
| `--format` | `spectre`; also `cdl` (LVS output may omit sources) |
| `--hierarchy` | `true`, cell only |
| `--external` | `true` for a cell; `false` for a library |
| `--file-mode` | `separate`; `combined` is cell only |
| `--view` | Explicit top view, cell only; otherwise use the view lists |
| `--schematic-views`, `--symbol-views` | `schematic`, `symbol`; quoted space-separated lists in priority order |
| `--auto-check-save` | `true`, matching GUI/API; **use `--auto-check-save=false` for read-only export** |
| `--selection-box` | `true`, union of native selection boxes |
| `--symbol-library` | Bundled `eda-symbol`; repeat for several `.rnl` symbol libraries, highest priority first. Config key `symbol_libraries` |
| `--symbol-only-cells` | `subcircuit`: each cell without schematic becomes an empty subcircuit with its symbol; `package`: one symbol package with a mapping file. Library only |
| `--symbol-package-name` | Name of that package; default the library name in lower case. Library only, with `package` |
| `--scale` | `10`, positive finite EDIF units per RNL unit |
| `--timeout` | `30m` for the entire run; e.g. `2h`, or `0` for no deadline |
| `--logs` | Default `.rnl/launcher` inside output; custom paths must stay below output/.rnl, outside reserved work/tmp directories |
| `--verbose` | Stream native output to stderr as well as the log |
| `--install-dir` | Override the installation normally found beside the executable |

`--auto-check-save=true` can save stale source schematics. A portless top includes
its descendants once; ported files have no native includes. Keep all generated
files together. Native model/Verilog-A references remain external. This command
exports a design; it does not simulate it.

## Completion, logs and automation

The command blocks through native export and final library merge. stdout contains
one JSON result; stderr gives the log directory, and optionally live native
messages. Use the shell exit status as well as JSON `state`:

| Exit | State | Meaning |
|---|---|---|
| 0 | `succeeded` | Complete export |
| 3 | `partial` | Library output exists, but some cells failed; read `library-report.json` |
| 2 | `failed` | Invalid input, startup, native export or conversion failure |
| 124 | `timed_out` | Whole-run deadline reached |
| 130 | `interrupted` | Interrupted by SIGINT/SIGTERM |

The output directory is reserved before launch. RNL files and reports stay at
its root; `.rnl/launcher/` holds launcher/CIW logs, the generated driver and final
status, `.rnl/work/` holds native inputs, intermediate netlists/EDIF and conversion
logs, and `.rnl/tmp/` holds launcher scratch. Library cell jobs and staging also
stay inside this tree. Native child processes receive local temporary/log paths. Cadence session,
license and OS housekeeping files retain their own runtime locations.
The `.rnl` name is reserved and retained on failure; directory existence alone is
not proof of success. The completion report is published after the result files.

Successful/partial results include the report and output directory; results after
launch also identify the log directory. It contains `launcher.log`, `CDS.log`,
the generated `driver.il`, and `status.sx` when SKILL reached a terminal state.
Native work/error paths are included in the final status when available.
Failures before launch may have no log directory. A missing or inconsistent final
status is an error even if Virtuoso itself exited zero.

Timeout and interruption terminate the dedicated process group. Logs and already
created artifacts are retained for diagnosis. Do not interpret them as success
or immediately repeat an unchanged failing export. No existing desktop/bridge
session is used or stopped. Required Cadence tools and licenses still apply.
