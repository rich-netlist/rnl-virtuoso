# Install or upgrade the exporter

Resolve the execution machine, account, architecture, installation destination
and required capability first. Ask the user for required information that is
still unknown. There is no assumed installation directory. An export request
alone is not installation consent; an explicit installation/upgrade request is.
Do not ask again for consent already given for the same action and destination.

## Obtain an appropriate upstream build

Source: https://github.com/rich-netlist/rnl-virtuoso.git

Discover the latest published release and its assets. Prefer a matching static
Linux archive when it supports the requested interface; the runtime then needs
neither Python nor Go. Do not assume an architecture, asset name or version.
For example, if GitHub CLI is available:

```sh
gh release view --repo rich-netlist/rnl-virtuoso --json tagName,url,assets
gh release download TAG --repo rich-netlist/rnl-virtuoso --pattern ASSET_PATTERN --dir NEW_DOWNLOAD_DIRECTORY
```

Replace placeholders with the discovered values and quote for the actual shell.
Without `gh`, use an available web/HTTP tool and GitHub's release metadata to
obtain asset URLs. Record the chosen version and source. Verify the archive
against its published `.sha256` file before extraction.

Older releases may lack the headless CLI or other requested API options. Check
the selected version's documentation and verify capability after staging. If no
suitable release exists, explain this and ask whether to build current upstream
source. For an authorized source build, resolve the latest appropriate revision,
read its build instructions, and build/package on a suitable development machine.
Ask where to build if no such environment is established; do not assume local
toolchains or install them on the EDA machine as a workaround.

Before installation/upgrade, present the concrete target/account, destination,
selected release or source revision and reason. Obtain consent if outstanding.
This is the consent requirement of this skill; it does not require repeated
approval for an already authorized installation.

## Install in the agreed location

1. On the confirmed execution machine, verify identity, home and architecture
   using available tools (for a POSIX shell: `whoami`, `hostname`, `uname -sm`).
   Verify the agreed user-writable destination; do not infer it from another host.
2. Transfer the archive and checksum using an available authorized method, if
   building/downloading elsewhere. Verify the target copy's hash. Inspect archive
   members: paths must stay under the package root, with no absolute paths,
   parent traversal or links escaping it.
3. Extract into a new staging directory on the destination filesystem. Require
   `rnl-virtuoso.il`, `rnl-library.il`, `rnl-api.il`, `bin/rnl-virtuoso`,
   `resources/` and docs. Check `bin/rnl-virtuoso -version` against the build.
4. Verify the needed capability before replacing an existing installation. For
   headless mode, require `bin/rnl-virtuoso export --help` and read
   `docs/headless.md`. For live-session mode, inspect `docs/skill-api.md` for the
   required API/options, then perform the session check after loading below.
5. Publish without overwriting an unexpected installation. For an approved
   upgrade, wait until exports finish and retain the prior installation as a
   uniquely named backup. Restore it if publication fails. Do not change project
   library mappings or shell/Virtuoso startup files.
6. In live-session mode, load the normal entry point from the confirmed absolute
   installation path, then query `rnlExporterInfo()` in the intended session.
   Check API, install path and requested capabilities. Normal load installs menus
   automatically. Headless mode needs no bridge or live-session load.
7. Continue the export and report the actual version/revision and install path.

This skill contains instructions only. Obtain the exporter from upstream; do not
copy its runtime or a bridge into skill assets.
