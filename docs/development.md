# Development

## Layout

| Path | Purpose |
| --- | --- |
| `rnl-virtuoso.il` | Menu, form, preflight and asynchronous native-export launcher |
| `rnl-library.il` | CIW selection form and sequential library export queue |
| `cmd/rnl-virtuoso/` | Pure Go backend, parsers, geometry, routing and native job tests |
| `resources/` | Editable package snapshots and orientation corrections |
| `scripts/` | Archive builder and test driver; development tools only |
| `tests/` | Original synthetic fixtures and shared regression assertions |
| `tests/reference/` | Python reference implementation; never shipped or launched by SKILL |

The backend inherits Virtuoso's environment and invokes its native `si` and
`edifout` tools. Request files are inert S-expressions. Spectre/CDL statements
stay intact; RNL drawing records are added as comments. The converter uses no
third-party Go modules. Native files and logs stay in the temporary job directory
when an input cannot be converted without guessing.

## Checks

```sh
go test ./...
go vet ./...
python scripts/test.py --rnl /path/to/rnl
reuse lint
```

The test driver builds the host test executable and the static Linux executable,
then runs the shared fixture tests against Python and Go. An RNL CLI with bus
bundle symbol support is required for the checker/rendering assertions; use
`--rnl` or `RNL_BIN`. Without it, those assertions are reported as skipped.
Real PDK/design exports are not test fixtures and must not be committed.
Three additional Go-only PDK regression tests cover native series/parallel
expansions, boundary mapping and rejection of unexpected branches/members.
They include all eight orientations and model/name labels. Go unit tests also
check expansion delimiters, topology, shared/global terminals and CDF separators.
Go-only library fixtures check native-definition deduplication, conflicting names,
symbol-only cells, portless entries, both formats/file modes, external-cell scope,
per-library external directories, actual cell filenames, standalone child symbols,
flat top-only includes, shared-child diamonds, native context variants and portable
path collisions. Spectre read-in rejects repeated subcircuit definitions (`SFE-3`);
the exporter avoids recursive cell-file includes rather than suppressing that error.
Annotation fixtures check wire/net ownership, nearest-segment selection, repeated
paths, bus range labels, text placement and note geometry from EDIF and the
read-only `annotations.sx` snapshot. Ownership is checked through `rnl problem`.
Port fixtures check input/output/inout and unknown directions, scalar and bus
ports in all eight orientations, both native formats and replacement theme colors.
Repeated port markers retain each position, orientation and wire attachment;
their native electrical statements must remain unchanged.
Note fixtures also check dashed strokes and compatibility with older snapshots.

To run the shared assertions separately after building:

```sh
RNL_EXPORT_BACKEND=go python -m unittest discover -s tests -v
RNL_EXPORT_BACKEND=python python -m unittest discover -s tests -v
```

The Go binary exposes `--test-json` for the offline test driver. SKILL uses the
normal request-file interface, not the JSON test interface. Linux-only tests
cover atomic publication and refusal of existing files, directories and symlinks.

## Packaging

```sh
python scripts/package.py dist/rnl-virtuoso-0.7.1-linux-amd64.tar.gz
```

Use `--go /path/to/go` or `--backend /path/to/prebuilt-linux-executable` if needed.
The builder selects `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1`, checks the
ELF architecture/static linkage and writes executable permissions into the tar.
The archive excludes Python, Go source code and temporary files. Existing archives
and export directories are never overwritten.

The release binary was tested with Go 1.27.3. Its [Linux kernel baseline is 3.2](https://go.dev/wiki/MinimumRequirements);
older kernels and other architectures are not validated. Cadence's own OS and
license requirements remain separate from the converter's requirements.
