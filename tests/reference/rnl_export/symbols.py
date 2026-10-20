"""Style-neutral EDIF symbol graphics and the pinned eda-symbol interface."""
import math
import re

from . import ExportError
from .edif import children, first, name, points, tag, origin, walk, decoded


def quote(value):
    # RNL quotes use doubled quotes, not JSON escapes. Backslashes are markup.
    return '"' + str(value).replace('\n', ' ').replace('\r', ' ').replace('"', '""') + '"'


def number(value):
    if not math.isfinite(value):
        raise ExportError("Non-finite drawing coordinate")
    return format(round(value, 6), ".6f").rstrip("0").rstrip(".") if value else "0"


def pt(value):
    return "(%s, %s)" % tuple(number(v) for v in value)


def ref(kind, value):
    return kind + quote(value)


def literal(value):
    # Literal EDIF labels are not RNL markup or attribute expressions.
    return ''.join("\\textbackslash{}" if c == "\\" else "\\"+c if c in "_{}$%&#^" else c for c in value)


STANDARD = {
    "res": "RESISTOR", "cap": "CAPACITOR", "ind": "INDUCTOR",
    "vdc": "VOLTAGE_SOURCE", "vsource": "VOLTAGE_SOURCE", "vsin": "VSIN", "vpulse": "VPULSE",
    "idc": "CURRENT_SOURCE", "isource": "CURRENT_SOURCE", "isin": "ISIN", "ipulse": "IPULSE",
    "vcvs": "VCVS_4TERM", "vccs": "VCCS_4TERM", "switch": "VOLTAGE_CONTROLLED_SWITCH",
    "nmos": "NMOS", "nmos4": "NMOS_4TERM", "pmos": "PMOS", "pmos4": "PMOS_4TERM",
    "npn": "NPN", "pnp": "PNP", "diode": "DIODE",
}


def package_pins(text):
    """Read literal pin interfaces from the shipped, fixed version of eda-symbol."""
    starts = list(re.finditer(r'\$([A-Z][A-Z_0-9]*)\s*=\s*@DEFINE-SYMBOL\b', text))
    result = {}
    for i, match in enumerate(starts):
        body = text[match.end():starts[i+1].start() if i+1 < len(starts) else len(text)]
        pins = {}
        pattern = r'\(\s*([-+\d.]+)\s*,\s*([-+\d.]+)\s*\)\s*@PIN\(terminal\s*=\s*(\d+|"[^"]+")'
        for p in re.finditer(pattern, body):
            pins[p[3].strip('"').lower()] = (float(p[1]), float(p[2]))
        result[match[1]] = pins
    return result


def text_statement(value, display, scale, owner="", key=None, markup=False):
    at = tuple(v/scale for v in origin(display))
    justify = first(display, "justify", [None, "LOWERLEFT"])[1]
    halign = "end" if justify.endswith("RIGHT") else "middle" if justify.endswith("CENTER") else "start"
    valign = "top" if justify.startswith("UPPER") else "middle" if justify.startswith("CENTER") else "baseline"
    orientation = first(display, "orientation", [None, "R0"])[1]
    rotate = 90 if orientation in ("R90", "R270", "MXR90", "MYR90") else 0
    heights = list(walk(display, "textheight"))
    size = float(heights[0][1])/scale if heights else 1.0
    params = [quote(value if markup else literal(value)), "size="+number(size), "halign="+quote(halign), "valign="+quote(valign)]
    if rotate:
        params.append("rotate=90")
    if key:
        params.append("key="+quote(key))
    if owner:
        return "%s @TEXT(%s) => @PLACE%s" % (owner, ", ".join(params), pt(at))
    return "%s @TEXT(%s)" % (pt(at), ", ".join(params))


def graphics(node, scale):
    result = []
    for item in node:
        kind = tag(item)
        if not kind:
            continue
        ps = [tuple(v/scale for v in p) for p in points(item)]
        if kind in ("path", "polygon"):
            if kind == "polygon" and ps and ps[-1] != ps[0]:
                ps.append(ps[0])
            if len(ps) >= 2:
                result.append("@LINE {%s}" % " ".join(pt(p) for p in ps))
        elif kind == "rectangle":
            if len(ps) != 2:
                raise ExportError("EDIF rectangle needs two corners")
            result.append("@RECTANGLE {%s}" % " ".join(pt(p) for p in ps))
        elif kind == "circle":
            if len(ps) != 2:
                raise ExportError("EDIF circle needs diameter endpoints")
            center = tuple((a+b)/2 for a, b in zip(*ps))
            radius = math.hypot(ps[1][0]-ps[0][0], ps[1][1]-ps[0][1])/2
            result.append("%s @CIRCLE(radius=%s)" % (pt(center), number(radius)))
        elif kind == "arc":
            if len(ps) != 3:
                raise ExportError("EDIF arc needs three points")
            result.append("@ARC {%s}" % " ".join(pt(p) for p in ps))
        elif kind in ("figure", "shape", "openshape", "curve"):
            result.extend(graphics(item, scale))
        elif kind in ("dot", "property", "figuregroupoverride", "display"):
            continue
        else:
            raise ExportError("Unsupported EDIF graphic %s" % kind)
    return result


def symbol_definition(cell, symbol_id, terminal_map, scale, notes):
    if not cell.symbol:
        raise ExportError("No EDIF symbol geometry for %s/%s" % (cell.library, cell.name))
    box = points(first(cell.symbol, "boundingbox", []))
    suffix = ""
    if len(box) == 2:
        suffix = "(box=[%s])" % ", ".join(number(v/scale) for p in box for v in p)
    body = []
    for figure in children(cell.symbol, "figure"):
        body.extend(graphics(figure, scale))
    for cg in children(cell.symbol, "commentgraphics"):
        for sd in walk(cg, "stringdisplay"):
            value = decoded(sd[1])
            if "cds" in value and "(" in value:
                if value == "cdsName()":
                    body.append(text_statement("${name}", first(sd, "display", []), scale, "?", "name", True))
                else:
                    notes.append("Omitted unevaluated symbol annotation %s/%s: %s" % (cell.library, cell.name, value))
            else:
                body.append(text_statement(value, first(sd, "display", []), scale))
    for kd in children(cell.symbol, "keyworddisplay"):
        key = str(kd[1]).lower()
        if key in ("instance", "cell"):
            body.append(text_statement("${name}" if key == "instance" else "${model}", first(kd, "display", []), scale, "?", "name" if key == "instance" else "model", True))
    for pin in children(cell.symbol, "portimplementation"):
        from .edif import ident
        port = ident(pin[1]).lower()
        members = cell.port_groups[port]
        if any(p not in terminal_map for p in members):
            raise ExportError("Symbol pin %s/%s has no CDL terminal" % (cell.name, port))
        at = tuple(v/scale for v in cell.pins[members[0]])
        terminals = [terminal_map[p] for p in members]
        terminal_values = [str(t) if isinstance(t, int) else quote(t) for t in terminals]
        terminal_value = terminal_values[0] if len(members) == 1 else '['+', '.join(terminal_values)+']'
        body.append("%s @PIN(terminal=%s)" % (pt(at), terminal_value))
        if isinstance(pin[1], list):
            for display in children(pin[1], "display"):
                body.append(text_statement(cell.port_labels.get(port, port), display, scale))
    if not any('key="name"' in statement for statement in body):
        body.append('? @TEXT("${name}", key="name") => @PLACE(0, 1)')
    return "%s = @DEFINE-SYMBOL%s {\n  %s\n}" % (symbol_id, suffix, ";\n  ".join(body))
