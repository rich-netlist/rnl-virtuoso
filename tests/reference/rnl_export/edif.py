"""Read the EDIF 2.0.0 subset emitted by Cadence edifout.

The tree is data, never evaluated as SKILL or Python. Identifier aliases are kept
separate from display names, and geometry stays in EDIF coordinates until export.
"""
import re
from dataclasses import dataclass, field

from . import ExportError


def parse(text, skill=False):
    root, stack = [], []
    current = root
    pos = 0
    token = re.compile(r'\s+|\(|\)|"(?:[^"\\]|\\.)*"|[^\s()"]+')
    for match in token.finditer(text):
        if match.start() != pos:
            raise ExportError("Malformed EDIF string at byte %d" % pos)
        pos = match.end()
        word = match.group()
        if word.isspace():
            continue
        if word == "(":
            child = []
            current.append(child)
            stack.append(current)
            current = child
        elif word == ")":
            if not stack:
                raise ExportError("Unbalanced EDIF parentheses")
            current = stack.pop()
        else:
            value = word[1:-1] if word.startswith('"') else word
            if skill and word.startswith('"'):
                escapes = {"n": "\n", "r": "\r", "t": "\t", "\\": "\\", '"': '"'}
                value = re.sub(r'\\([0-7]{3}|[nrt\\"])', lambda m: escapes[m[1]] if m[1] in escapes else chr(int(m[1], 8)), value)
            current.append(value)
    if stack or pos != len(text) or len(root) != 1:
        raise ExportError("Incomplete EDIF document")
    return root[0]


def tag(node):
    return node[0].lower() if isinstance(node, list) and node and isinstance(node[0], str) else ""


def children(node, kind):
    return [n for n in node if tag(n) == kind.lower()]


def first(node, kind, default=None):
    return next(iter(children(node, kind)), default)


def walk(node, kind):
    for n in node:
        if isinstance(n, list):
            if tag(n) == kind.lower():
                yield n
            yield from walk(n, kind)


def decoded(value):
    return re.sub(r"%(\d+(?: +\d+)*)%", lambda m: "".join(chr(int(v)) for v in m[1].split()), value)


def ident(node):
    if isinstance(node, str):
        return node
    if tag(node) in ("rename", "name", "array", "member"):
        return ident(node[1])
    raise ExportError("Unsupported EDIF name: %r" % (node,))


def name(node):
    if isinstance(node, str):
        return decoded(node)
    if tag(node) == "rename":
        return name(node[2])
    if tag(node) == "stringdisplay":
        return decoded(node[1])
    if tag(node) in ("name", "array"):
        return name(node[1])
    raise ExportError("Unsupported EDIF display name: %r" % (node,))


def scalar_names(node):
    """Cadence array display order, distinct from EDIF's zero-based member index."""
    if tag(node) != "array":
        return [name(node)]
    if len(node) != 3:
        raise ExportError("Multidimensional EDIF arrays are not supported")
    def expand(part):
        repeated = re.fullmatch(r'<\*(\d+)>(.+)', part.strip())
        if repeated:
            count = int(repeated[1])
            if count < 1 or count > int(node[2]):
                raise ExportError("Invalid EDIF repetition count")
            return expand(repeated[2]) * count
        match = re.fullmatch(r'(.+)<(\d+):(\d+)>', part.strip())
        if match:
            start, stop = int(match[2]), int(match[3])
            return ['%s<%d>' % (match[1], i) for i in range(start, stop+(1 if stop >= start else -1), 1 if stop >= start else -1)]
        return [part.strip()]
    result = [member for part in name(node).split(',') for member in expand(part)]
    if len(result) != int(node[2]):
        raise ExportError("Cannot expand EDIF array %s into %s members" % (name(node), node[2]))
    return result


def member_ids(node):
    key = ident(node).lower()
    return [key+'[%d]' % i for i in range(len(scalar_names(node)))] if tag(node) == "array" else [key]


def point(node):
    if tag(node) != "pt" or len(node) != 3:
        raise ExportError("Invalid EDIF point: %r" % (node,))
    return float(node[1]), float(node[2])


def points(node):
    return [point(p) for p in walk(node, "pt")]


def origin(node):
    at = first(node, "origin", [])
    return point(first(at, "pt")) if at else (0.0, 0.0)


MATRICES = {
    "R0": (1, 0, 0, 1), "R90": (0, -1, 1, 0),
    "R180": (-1, 0, 0, -1), "R270": (0, 1, -1, 0),
    "MX": (1, 0, 0, -1), "MY": (-1, 0, 0, 1),
    "MXR90": (0, 1, 1, 0), "MYR90": (0, -1, -1, 0),
}


def transform(p, orient, offset=(0, 0)):
    try:
        a, b, c, d = MATRICES[orient]
    except KeyError:
        raise ExportError("Unsupported EDIF orientation %s" % orient)
    x, y = p
    return a*x + b*y + offset[0], c*x + d*y + offset[1]


@dataclass
class Instance:
    id: str
    name: str
    master: tuple
    xy: tuple
    orient: str
    bundle: str = ""
    bundle_name: str = ""


@dataclass
class Net:
    name: str
    terminals: list = field(default_factory=list)  # (instance id or None, port id)
    paths: list = field(default_factory=list)
    labels: list = field(default_factory=list)
    bits: tuple = ()


@dataclass
class Cell:
    library: str
    name: str
    id: str
    ports: dict = field(default_factory=dict)
    port_groups: dict = field(default_factory=dict)
    port_labels: dict = field(default_factory=dict)
    pins: dict = field(default_factory=dict)
    symbol: list = field(default_factory=list)
    instances: list = field(default_factory=list)
    nets: list = field(default_factory=list)
    routes: list = field(default_factory=list)
    port_positions: dict = field(default_factory=dict)
    port_orientations: dict = field(default_factory=dict)
    annotations: list = field(default_factory=list)


def read(text):
    tree = parse(text)
    if tag(tree) != "edif" or first(tree, "edifversion", [])[1:] != ["2", "0", "0"]:
        raise ExportError("Only EDIF 2.0.0 is supported")
    cells, ids = {}, {}
    # Resolve references in a second pass: a library can be written after its users.
    pending, raw_nets, instance_refs = [], [], {}
    for lib in children(tree, "library") + children(tree, "external"):
        libname = name(lib[1])
        for raw in children(lib, "cell"):
            cell = Cell(libname, name(raw[1]), ident(raw[1]))
            cells[(libname, cell.name)] = cell
            ids[(ident(lib[1]).lower(), cell.id.lower())] = cell
            for view in children(raw, "view"):
                interface = first(view, "interface", [])
                for p in children(interface, "port"):
                    members = member_ids(p[1])
                    cell.ports.update(zip(members, scalar_names(p[1])))
                    cell.port_groups[ident(p[1]).lower()] = members
                    cell.port_labels[ident(p[1]).lower()] = name(p[1])
                symbol = first(interface, "symbol", [])
                if symbol:
                    cell.symbol = symbol
                    for pin in children(symbol, "portimplementation"):
                        loc = first(pin, "connectlocation", [])
                        pts = points(loc)
                        if len(pts) != 1:
                            raise ExportError("Expected one connection location for %s/%s" % (cell.name, name(pin[1])))
                        key = ident(pin[1]).lower()
                        for member in cell.port_groups[key]:
                            cell.pins[member] = pts[0]
                for contents in children(view, "contents"):
                    pages = children(contents, "page") or [contents]
                    if cell.instances or cell.nets:
                        raise ExportError("Multiple schematic views for %s/%s" % (libname, cell.name))
                    for page in pages:
                        for inst in children(page, "instance"):
                            vr = first(inst, "viewref", [])
                            cr = first(vr, "cellref", [])
                            lr = first(cr, "libraryref", [None, ident(lib[1])])
                            tr = first(inst, "transform", [])
                            if first(tr, "scale"):
                                raise ExportError("Scaled EDIF instances are not supported")
                            group = []
                            for iid, label in zip(member_ids(inst[1]), scalar_names(inst[1])):
                                ins = Instance(iid, label, (str(lr[1]).lower(), str(cr[1]).lower()), origin(tr), first(tr, "orientation", [None, "R0"])[1], ident(inst[1]).lower() if tag(inst[1]) == "array" else "", name(inst[1]))
                                cell.instances.append(ins)
                                pending.append(ins)
                                group.append(ins)
                            instance_refs[(libname, cell.name, ident(inst[1]).lower())] = group
                        for pi in children(page, "portimplementation"):
                            pts = points(first(pi, "connectlocation", []))
                            if pts:
                                key = ident(pi[1]).lower()
                                # Cadence connectLocation can name another point on
                                # the same net. The nested pin instance is its drawn location.
                                instance = first(pi, "instance", [])
                                tr = first(instance, "transform", [])
                                cell.port_positions[key] = origin(tr) if tr else pts[0]
                                cell.port_orientations[key] = first(tr, "orientation", [None, "R0"])[1]
                        for rawnet in children(page, "net"):
                            net = Net(name(rawnet[1]), bits=tuple(scalar_names(rawnet[1])))
                            for figure in children(rawnet, "figure"):
                                net.paths.extend(points(p) for p in children(figure, "path"))
                            if isinstance(rawnet[1], list):
                                net.labels = list(walk(rawnet[1], "display"))
                            cell.routes.append(net)
                            raw_nets.append((cell, net, first(rawnet, "joined", [])))
                        cell.annotations.extend(children(page, "commentgraphics"))
    for inst in pending:
        if inst.master not in ids:
            raise ExportError("EDIF omitted the symbol for %s (set externalLibList to empty)" % (inst.master,))
        c = ids[inst.master]
        inst.master = (c.library, c.name)
    for cell, route, joined in raw_nets:
        bitnets = [Net(bit) for bit in route.bits]

        def references(pr):
            if tag(pr) == "portlist":
                return [ref for p in pr[1:] for ref in references(p)]
            if tag(pr) != "portref":
                raise ExportError("Unsupported EDIF joined object %s" % tag(pr))
            ir = first(pr, "instanceref")
            group = instance_refs[(cell.library, cell.name, ident(ir[1]).lower())] if ir else [None]
            if ir and tag(ir[1]) == "member":
                group = [group[int(ir[1][2])]]
            out = []
            for ins in group:
                master = cells[ins.master] if ins else cell
                ports = master.port_groups[ident(pr[1]).lower()]
                if tag(pr[1]) == "member":
                    ports = [ports[int(pr[1][2])]]
                out.extend((ins.id if ins else None, p) for p in ports)
            return out

        for item in joined[1:]:
            refs = references(item)
            if len(bitnets) == 1:
                bitnets[0].terminals.extend(refs)
            elif len(refs) == len(bitnets):
                for net, terminal in zip(bitnets, refs):
                    net.terminals.append(terminal)
            else:
                raise ExportError("EDIF bus width mismatch in %s/%s" % (cell.name, route.name))
        cell.nets.extend(bitnets)
    return cells
