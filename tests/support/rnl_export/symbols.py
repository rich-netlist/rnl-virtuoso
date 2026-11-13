"""Test support: a reader of package pins."""
import re


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
