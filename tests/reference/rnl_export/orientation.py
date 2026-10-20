"""Compose package orientation corrections with all eight EDIF transforms."""
import json

from . import ExportError
from .edif import MATRICES, transform
from .resources import read


def load_rules(text=None):
    data = json.loads(read("symbol-orientations.json") if text is None else text)
    if not isinstance(data, dict) or data.get("schema") != 1 or data.get("package") != {"name": "eda-symbol", "version": "0.1"}:
        raise ExportError("Unsupported symbol orientation configuration")
    rules = data.get("symbols")
    if not isinstance(rules, dict):
        raise ExportError("Symbol orientation configuration needs a symbols table")
    for name, rule in rules.items():
        if not isinstance(rule, dict) or rule.get("package_to_source") not in MATRICES:
            raise ExportError("Invalid package orientation for " + name)
        pins = rule.get("align_pins")
        if pins is not None and (not isinstance(pins, list) or not pins or any(type(p) is not int or p < 1 for p in pins) or len(pins) != len(set(pins))):
            raise ExportError("Invalid alignment pins for " + name)
    return rules


def compose(outer, inner):
    """Apply inner in local coordinates, then outer in schematic coordinates."""
    x = transform(transform((1, 0), inner), outer)
    y = transform(transform((0, 1), inner), outer)
    matrix = (x[0], y[0], x[1], y[1])
    return next(name for name, value in MATRICES.items() if value == matrix)


def place(symbol, source_pins, package_pins, orientation, origin, rules):
    rule = rules.get(symbol, {})
    correction = rule.get("package_to_source", "R0")
    aligned = rule.get("align_pins", list(range(1, len(source_pins) + 1)))
    if any(p > len(source_pins) for p in aligned):
        raise ExportError("Alignment pin outside symbol interface: " + symbol)
    offsets = []
    for p in aligned:
        target = transform(package_pins[p-1], correction)
        offsets.append(tuple(source_pins[p-1][axis] - target[axis] for axis in (0, 1)))
    offset = tuple(sum(p[axis] for p in offsets) / len(offsets) for axis in (0, 1))
    return transform(offset, orientation, origin), compose(orientation, correction)
