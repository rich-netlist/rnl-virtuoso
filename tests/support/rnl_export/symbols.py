"""Test support: the standard symbol table sent to the Go backend, and a reader of package pins."""
import re


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
