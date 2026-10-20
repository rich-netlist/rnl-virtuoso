"""Read native si Spectre structure, retaining the original electrical text."""
import re

from . import ExportError
from .cdl import Block, MARKERS
from .edif import parse


def load_name_maps(directory):
    """Use si's own mappings for bus spelling, global nets and renamed instances."""
    directory = directory.resolve()
    index = parse((directory / "map" / "current").read_text(encoding="utf-8"), skill=True)
    result = {}
    for entry in index:
        if not isinstance(entry, list) or len(entry) != 2 or not isinstance(entry[1], str):
            continue
        path = (directory / entry[1]).resolve()
        path.relative_to(directory)  # A mapping file must stay in this native job.
        if not path.exists():
            # si omits identity-only maps (but keeps their index entries).
            result[entry[0]] = {}
            continue
        sections = parse(path.read_text(encoding="utf-8"), skill=True)
        result[entry[0]] = {section[0]: dict(section[1:]) for section in sections if section and section[0] in {"net", "inst"}}
    return result


def statements(lines):
    pending, start = "", 0
    for i, line in enumerate(lines):
        value = line.strip()
        if not pending:
            start = i
        if value.endswith("\\"):
            pending += value[:-1] + " "
        else:
            yield start, i + 1, pending + value
            pending = ""
    if pending:
        raise ExportError("Incomplete Spectre continuation")


def names(text):
    # Native names escape punctuation (including bus brackets) with backslashes.
    return [re.sub(r"\\(.)", r"\1", token) for token in re.findall(r"(?:\\.|[^\s])+", text)]


def read(text, top):
    lines = text.splitlines(keepends=True)
    blocks, current, root = {}, None, None
    lib, cell = "", ""
    for start, end, stmt in statements(lines):
        if not stmt or stmt.startswith("//"):
            match = re.match(r"//\s*(Library|Cell) name:\s*(\S+)", stmt, re.I)
            if match:
                if match[1].lower() == "library":
                    lib = match[2]
                else:
                    cell = match[2]
            continue
        head = stmt.split()[0]
        if head == "subckt":
            words = names(stmt)
            if current or root or len(words) < 2 or words[1] in blocks:
                raise ExportError("Invalid, nested or duplicate native Spectre subckt")
            current = Block(words[1], words[2:], start, library=lib, cell=cell)
            blocks[current.name] = current
        elif head == "ends":
            if current is None:
                raise ExportError("Spectre ends without subckt")
            words = names(stmt)
            if len(words) > 1 and words[1] != current.name:
                raise ExportError("Spectre ends name differs from subckt")
            current.close, current.end = start, end
            current = None
        elif head in {"simulator", "global", "parameters", "ahdl_include"}:
            if head == "simulator" and stmt != "simulator lang=spectre":
                raise ExportError("Mixed simulator languages are not supported by this exporter")
        else:
            match = re.match(r"((?:\\.|[^\s(])+)\s+\((.*?)\)\s+((?:\\.|[^\s])+)", stmt)
            if not match:
                raise ExportError("Unsupported native Spectre statement at line %d: %s" % (start + 1, head))
            words = names(match[1]) + names(match[2]) + names(match[3])
            if current is None:
                if root is None:
                    if top in blocks:
                        raise ExportError("Spectre root and subckt have the same name")
                    root = Block(top, [], start, library=lib, cell=cell or top)
                    blocks[top] = root
                block = root
                root.close = root.end = end
            else:
                block = current
            if words[0] in block.instances:
                raise ExportError("Duplicate Spectre instance %s" % words[0])
            block.instances[words[0]] = words
    if current or root is None:
        raise ExportError("Incomplete or empty native Spectre circuit")
    return lines, blocks


def match_instances(block, cell, mapping=None):
    result, used = {}, set()
    mapping = (mapping or {}).get("inst", {})
    for source in cell.instances:
        actual = mapping.get(source.name, source.name)
        words = block.instances.get(actual)
        if words is None:
            if source.master[1].lower() in MARKERS:
                continue
            raise ExportError("No Spectre instance for %s/%s" % (block.name, source.name))
        if actual in used:
            raise ExportError("Duplicate EDIF instance %s/%s" % (block.name, source.name))
        result[source.id.lower()] = words
        used.add(actual)
    missing = set(block.instances) - used
    if missing:
        raise ExportError("Spectre instances absent from EDIF %s: %s" % (block.name, ', '.join(sorted(missing))))
    return result
