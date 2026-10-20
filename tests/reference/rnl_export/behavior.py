"""Read Verilog-A module interfaces without evaluating model code."""
import re
from pathlib import Path

from . import ExportError


def interfaces(text):
    text = re.sub(r'/\*.*?\*/|//[^\n]*', '', text, flags=re.S)
    result = {}
    for match in re.finditer(r'\bmodule\s+(\w+)\s*\((.*?)\)\s*;(.*?)\bendmodule\b', text, re.S):
        model, header, body = match.groups()
        ports = [p.strip() for p in header.split(',') if p.strip()]
        if any(not re.fullmatch(r'[a-zA-Z_]\w*', p) for p in ports):
            raise ExportError('Unsupported Verilog-A port declaration in '+model)
        ranges = {}
        declared = set()
        for decl in re.finditer(r'\b(?:input|output|inout)\s+([^;]+);', body):
            spec = decl[1].strip()
            bus = re.match(r'\[\s*(-?\d+)\s*:\s*(-?\d+)\s*\]\s*(.*)', spec, re.S)
            if '[' in spec and bus is None:
                raise ExportError('Verilog-A port ranges must use integer bounds: '+model)
            bound = (int(bus[1]), int(bus[2])) if bus else None
            for port in (bus[3] if bus else spec).split(','):
                port = port.strip()
                if not re.fullmatch(r'[a-zA-Z_]\w*', port):
                    raise ExportError('Unsupported Verilog-A interface: '+model)
                if port in declared:
                    raise ExportError('Duplicate Verilog-A port: '+port)
                declared.add(port)
                ranges[port] = bound
        if set(ports) != declared or len(set(ports)) != len(ports):
            raise ExportError('Incomplete Verilog-A interface: '+model)
        order = []
        for port in ports:
            bound = ranges[port]
            if bound is None:
                order.append(port)
            else:
                a, b = bound
                order.extend('%s<%d>' % (port, i) for i in range(a, b + (1 if b >= a else -1), 1 if b >= a else -1))
        if model in result:
            raise ExportError('Duplicate Verilog-A module: '+model)
        result[model] = order
    if not result:
        raise ExportError('No supported Verilog-A module interface found')
    return result


def load_models(netlist, working):
    models, references = {}, []
    for match in re.finditer(r'^\s*ahdl_include\s+"([^"\n]+)"\s*$', netlist, re.M):
        path = Path(match[1])
        if not path.is_absolute():
            path = Path(working) / path
        parsed = interfaces(path.read_text(encoding='utf-8'))
        if set(parsed).intersection(models):
            raise ExportError('Duplicate Verilog-A modules in native includes')
        models.update(parsed)
        references.append(str(path))
    return models, references
