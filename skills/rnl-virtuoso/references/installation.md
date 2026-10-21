# Install a missing remote exporter

Resolve host/account/path and release before requesting consent. An export
request alone is not installation consent; an explicit installation/upgrade
request is. Do not ask again when that authorization is already present.

## Obtain the current upstream release

Source: https://github.com/rich-netlist/rnl-virtuoso.git

Prefer its latest stable published release and static Linux x86-64 archive.
Neither Python nor Go is then needed on the Virtuoso host. Discover exact tag
and asset names dynamically, for example on a machine with `gh`:

```sh
gh release view --repo rich-netlist/rnl-virtuoso --json tagName,url,assets
gh release download TAG --repo rich-netlist/rnl-virtuoso --pattern 'rnl-virtuoso-*-linux-amd64.tar.gz*' --dir NEW_DOWNLOAD_DIRECTORY
```

Replace placeholders with discovered values; quote for the actual shell. Without
`gh`, use GitHub's latest-release API and returned asset URLs with an available
HTTP client. Record the tag/source. Verify the archive against its `.sha256`
asset before extraction. Do not use `curl | sh` or unrelated download sources.

If no suitable release exists for the host architecture/API, explain the blocker.
For an appropriate/authorized source build, clone upstream at the selected latest
revision, read its build instructions, and package on a development machine with
Go and Python. Transfer the static archive; do not install build toolchains on
the EDA host by default. Use the latest default branch only when the user asks
for an unreleased/source version or knowingly chooses it over the release.

## Publish to the personal account

1. Verify identity, home and architecture (`whoami`, `hostname`, `uname -sm`).
   Default destination is `$HOME/tools/rnl-virtuoso`, in the personal account
   running Virtuoso. Obtain installation consent if outstanding.
2. Transfer the archive and checksum through existing SSH/bridge file transfer.
   Verify the remote hash too. Inspect archive members: paths must remain under
   `rnl-virtuoso/`; reject absolute paths, `..`, and links escaping that root.
3. Extract into a new staging directory on the destination filesystem. Require
   `rnl-virtuoso.il`, `rnl-library.il`, `rnl-api.il`, `bin/rnl-virtuoso`,
   `resources/`, and docs. Check `bin/rnl-virtuoso --version` succeeds and matches
   the selected release.
4. Move the staged directory to the final destination with a no-clobber
   operation. Do not overwrite an existing installation. For an approved upgrade,
   wait until exports finish, retain the prior directory as a uniquely named
   backup, then publish. Restore it if publication fails. Do not edit `.cdsinit`
   or `cds.lib`.
5. In the selected bridge session evaluate
   `load("ABSOLUTE_INSTALL_DIRECTORY/rnl-virtuoso.il")`, then
   `rnlExporterInfo()`. Require API 1, the intended path and expected version.
   Normal load also installs menus; no separate install call is needed. Continue
   the requested export and report the version/path with its result.

Keep this skill instruction-only. Fetch the exporter from upstream as needed;
never copy its runtime or the bridge into this skill as assets.
