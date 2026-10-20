"""Native export job, run on the Virtuoso host with its inherited environment."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

from . import ExportError, __version__
from .convert import convert, load_orders
from .edif import parse
from .spectre import load_name_maps
from .behavior import load_models
from . import resources


def skill_string(value):
    # All values go into a data string, never into a SKILL expression.
    return json.dumps(str(value), ensure_ascii=False)


def _run(argv, cwd, log, env, timeout):
    print("Running " + Path(argv[0]).name, flush=True)
    with log.open("w", encoding="utf-8") as stream:
        result = subprocess.run(argv, cwd=str(cwd), env=env, stdout=stream,
                                stderr=subprocess.STDOUT, timeout=timeout, check=False)
    if result.returncode:
        lines = log.read_text(encoding="utf-8", errors="replace").splitlines()
        first = next((i for i, line in enumerate(lines) if line.startswith("ERROR ")), None)
        detail = []
        if first is not None:
            for line in lines[first:first+6]:
                if not line.strip():
                    break
                detail.append(line.strip())
        raise ExportError("%s exited with %s: %s; see %s" % (Path(argv[0]).name, result.returncode, ' '.join(detail) or "native export failed", log))


def run_job(request_path):
    request_path = Path(request_path).resolve()
    fields = parse(request_path.read_text(encoding="utf-8"), skill=True)
    if len(fields) == 12 and fields[0] == "rnl-export-job-v1":
        fields.extend(["cdl", "nil"])
    elif len(fields) == 13 and fields[0] == "rnl-export-job-v2":
        fields.append("nil")
    elif len(fields) != 14 or fields[0] != "rnl-export-job-v3":
        raise ExportError("Unrecognized RNL export request")
    _, install, library, cell, view, cds, working, destination, hierarchy, split, scale, cadence, dialect, has_symbol = fields
    if dialect not in {"cdl", "spectre"}:
        raise ExportError("Unsupported native netlist format: " + dialect)
    work = request_path.parent
    output = Path(destination).expanduser()
    if not output.is_absolute():
        output = Path(working) / output
    output = output.resolve()
    if output.exists():
        raise ExportError("Output already exists: %s. Choose a new export directory." % output)
    if not output.parent.is_dir():
        raise ExportError("Output parent directory does not exist: %s" % output.parent)
    package = resources.read("eda-symbol.rnl")
    style_package = resources.read("eda-style.rnl")
    env = os.environ.copy()
    env["PWD"] = working  # Keep project-relative $PWD references in cds.lib meaningful.
    # getInstallPath() normally returns .../tools.lnx86/dfII, while a
    # command-line request may give the installation root.
    bins = Path(cadence) / "bin"
    if not (bins / "si").is_file():
        bins = Path(cadence) / "tools" / "dfII" / "bin"
    native = work / "native"
    native.mkdir()
    (native / "cds.lib").write_text("INCLUDE " + skill_string(cds) + "\n", encoding="utf-8")
    keys = {
        "runDirectory": str(native), "library": library, "cell": cell, "viewName": view,
        "outputFile": str(native / "design.edif"), "forConcept": "FALSE",
        "scalarOption": "FALSE", "scalarWithPortsUnexpanded": "FALSE",
        "netlistOption": "FALSE", "netlistMode": "Hierarchical", "externalLibList": "",
        "outputCDFData": "FALSE", "interpretInhConn": "FALSE",
        "replaceBundleWithArray": "TRUE", "skipDupInstNameCk": "FALSE",
    }
    template = native / "edif.il"
    template.write_text("edifOutKeys = list(nil\n" + ''.join(" '%s %s\n" % (key, skill_string(value)) for key, value in keys.items()) + ")\n", encoding="utf-8")
    _run([str(bins / "si"), str(work / "si"), "-batch", "-cdslib", cds, "-command", "nl" if dialect == "spectre" else "netlist"], Path(working), work / (dialect+".log"), env, 600)
    netlist = work / "si" / "netlist"
    if not netlist.is_file() or netlist.stat().st_size == 0:
        raise ExportError("Native exporter produced no netlist; see %s" % (work / (dialect+".log")))
    electrical = netlist.read_text(encoding="utf-8")
    if dialect == "spectre":
        # si writes simulator/global declarations separately from the circuit.
        electrical = (work / "si" / "netlistHeader").read_text(encoding="utf-8") + electrical + (work / "si" / "netlistFooter").read_text(encoding="utf-8")
    _run([str(bins / "edifout"), str(template)], native, work / "edif.log", env, 600)
    drawing = native / "design.edif"
    if not drawing.is_file() or drawing.stat().st_size == 0:
        raise ExportError("EDIF produced no drawing; see %s" % (work / "edif.log"))
    root_symbol = None
    if has_symbol == "t":
        symbol_template = native / "symbol.il"
        symbol_keys = dict(keys, viewName="symbol", outputFile=str(native / "symbol.edif"))
        symbol_template.write_text("edifOutKeys = list(nil\n" + ''.join(" '%s %s\n" % (key, skill_string(value)) for key, value in symbol_keys.items()) + ")\n", encoding="utf-8")
        _run([str(bins / "edifout"), str(symbol_template)], native, work / "symbol.log", env, 600)
        root_symbol = (native / "symbol.edif").read_text(encoding="utf-8")
    behavior_models, references = load_models(electrical, working) if dialect == "spectre" else ({}, [])
    print("Converting " + dialect + " + EDIF to RNL", flush=True)
    files, report = convert(electrical, drawing.read_text(encoding="utf-8"),
                            load_orders((work / "orders.sx").read_text(encoding="utf-8")), package, cell,
                            hierarchy == "t", split == "t", float(scale), dialect=dialect,
                            name_maps=load_name_maps(work / "si") if dialect == "spectre" else None,
                            root_symbol=root_symbol, behavior_models=behavior_models,
                            style_package=style_package)
    report.update(version=__version__, source={"library": library, "cell": cell, "view": view},
                  work_directory=str(work), output_directory=str(output), format=dialect,
                  behavior_model_references=references)
    # Publish the complete bundle at once. Never replace an existing user export.
    staging = Path(tempfile.mkdtemp(prefix=".rnl-stage-", dir=str(output.parent)))
    try:
        for filename, content in files.items():
            (staging / filename).write_text(content, encoding="utf-8")
        (staging / "export-report.json").write_text(json.dumps(report, indent=2, ensure_ascii=False)+"\n", encoding="utf-8")
        # Reserve the name exclusively. On the Linux host rename replaces only
        # this newly created empty directory, never a pre-existing user folder.
        output.mkdir()
        try:
            staging.rename(output)
        except OSError:
            # rmdir refuses a non-empty directory; never remove others' files.
            try:
                output.rmdir()
            except OSError:
                pass
            raise
    finally:
        if staging.exists():
            shutil.rmtree(staging)  # Only our fresh, private staging directory.
    (work / "result.json").write_text(json.dumps(report, indent=2, ensure_ascii=False)+"\n", encoding="utf-8")
    print("Exported " + str(output / report["entry"]), flush=True)
    for note in report["notes"]:
        print("Note: " + note, flush=True)
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description="Export RNL using native Virtuoso netlisting and EDIF")
    parser.add_argument("request", help="Request written by rnl-virtuoso.il")
    args = parser.parse_args(argv)
    started = time.monotonic()
    try:
        run_job(args.request)
    except (ExportError, OSError, ValueError, subprocess.TimeoutExpired) as exc:
        message = str(exc)
        print("RNL export failed: " + message, file=sys.stderr, flush=True)
        Path(args.request).with_name("error.txt").write_text(message+"\n", encoding="utf-8")
        return 1
    print("Finished in %.1f seconds" % (time.monotonic()-started), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
