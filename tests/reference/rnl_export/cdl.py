"""Locate native CDL statements without reserializing electrical data."""
import re
from dataclasses import dataclass, field

from . import ExportError

MARKERS = {"gnd", "gnda", "gndd", "vdd", "vdda", "vddd", "vss", "vssa", "vssd", "vcc", "vee", "noconn"}


@dataclass
class Block:
    name: str
    ports: list
    start: int
    end: int = 0
    close: int = 0
    library: str = ""
    cell: str = ""
    instances: dict = field(default_factory=dict)


def statements(lines):
    pending, start = "", 0
    for i, line in enumerate(lines):
        value = line.strip()
        if value.startswith("+"):
            if not pending:
                raise ExportError("CDL continuation without a statement at line %d" % (i+1))
            pending += " " + value[1:].strip()
        else:
            if pending:
                yield start, i, pending
            pending, start = value, i
    if pending:
        yield start, len(lines), pending


def read(text):
    lines = text.splitlines(keepends=True)
    blocks, current = {}, None
    lib, cell = "", ""
    for start, end, stmt in statements(lines):
        if stmt.startswith("*"):
            match = re.match(r"\*\s*(Library|Cell) Name:\s*(\S+)", stmt, re.I)
            if match:
                if match[1].lower() == "library":
                    lib = match[2]
                else:
                    cell = match[2]
            continue
        words = stmt.split()
        if not words:
            continue
        head = words[0].lower()
        if head == ".subckt":
            if current or len(words) < 2:
                raise ExportError("Invalid or nested CDL .SUBCKT")
            if words[1].lower() in blocks:
                raise ExportError("Duplicate CDL subcircuit %s" % words[1])
            ports = []
            for word in words[2:]:
                if "=" in word or word.lower() == "params:":
                    break
                if word != "/":
                    ports.append(word)
            current = Block(words[1], ports, start, library=lib, cell=cell)
            blocks[current.name.lower()] = current
            cell = ""
        elif head == ".ends":
            if current is None:
                raise ExportError("CDL .ENDS without .SUBCKT")
            current.close, current.end = start, end
            current = None
        elif current and not head.startswith("."):
            if head in current.instances:
                raise ExportError("Duplicate CDL instance %s in %s" % (words[0], current.name))
            current.instances[head] = words
        elif not current and not head.startswith("."):
            raise ExportError("Expected hierarchical CDL: found a device outside .SUBCKT")
    if current or not blocks:
        raise ExportError("Incomplete or empty hierarchical CDL netlist")
    return lines, blocks


def mapped_name(value):
    """Documented auCdl spelling candidates; callers verify all connections."""
    value = value.replace("<", "_").replace(">", "")
    return re.sub(r"[\s$:;=,/]", "", value).lower()


def match_instances(block, cell):
    result, used = {}, set()
    for source in cell.instances:
        source_name = mapped_name(source.name)
        candidates = [words for key, words in block.instances.items()
                      if mapped_name(key) == source_name or mapped_name(key[1:]) == source_name]
        if not candidates:
            # Supplies, ground and no-connect markers have no CDL statement.
            if source.master[1].lower() in MARKERS:
                continue
            raise ExportError("No CDL instance for %s/%s; CDL may omit simulation devices. Choose Spectre (simulation) to retain them." % (block.name, source.name))
        if len(candidates) != 1 or candidates[0][0].lower() in used:
            raise ExportError("Ambiguous CDL name for %s/%s" % (block.name, source.name))
        result[source.id.lower()] = candidates[0]
        used.add(candidates[0][0].lower())
    missing = set(block.instances) - used
    if missing:
        raise ExportError("CDL instances absent from EDIF %s: %s" % (block.name, ", ".join(sorted(missing))))
    return result
