"""Run the existing fixture assertions against the compiled Go backend.

Python is a development test driver only; no Python files ship in the plugin.
"""
import json
import os
from pathlib import Path
import subprocess
from types import SimpleNamespace

from rnl_export import ExportError
from rnl_export.symbols import STANDARD

ROOT = Path(__file__).resolve().parents[1]
BINARY = Path(os.environ.get('RNL_EXPORT_BIN', ROOT/'target'/('rnl-virtuoso.exe' if os.name == 'nt' else 'rnl-virtuoso')))
RESOURCES = ROOT/'resources'


def call(**request):
    result = subprocess.run([str(BINARY), '--resources', str(RESOURCES), '--test-json'],
                            input=json.dumps(request), text=True, encoding='utf-8', capture_output=True)
    if result.returncode:
        raise AssertionError(result.stderr)
    answer = json.loads(result.stdout)
    if 'error' in answer:
        raise ExportError(answer['error'])
    return answer


def convert(netlist, drawing, orders, package, top, hierarchy=True, split=False, scale=10.0, **options):
    answer = call(netlist=netlist, drawing=drawing,
                  orders=[dict(library=k[0], cell=k[1], ports=v) for k, v in orders.items()],
                  package=package, top=top, hierarchy=hierarchy, split=split, scale=scale,
                  standard=STANDARD, **options)
    return answer['files'], answer['report']


def cell_json(cell):
    return dict(Library=cell.library, Name=cell.name, ID=cell.id, Ports=cell.ports,
                PortOrder=list(cell.ports), Groups=cell.port_groups, GroupOrder=list(cell.port_groups),
                Labels=cell.port_labels, Pins=cell.pins, Symbol=cell.symbol or None,
                Routes=[dict(Name=n.name, Bits=n.bits, Paths=n.paths, Labels=n.labels) for n in cell.routes])


def symbol_definition(cell, symbol_id, terminal_map, scale, notes):
    answer = call(op='symbol', cell=cell_json(cell), symbol_id=symbol_id,
                  terminals={p: str(v) if isinstance(v, int) else '"'+v.replace('"', '""')+'"' for p, v in terminal_map.items()}, scale=scale)
    notes.extend(answer['notes'])
    return answer['text']


def draw_routes(cell, endpoints, netmap, scale):
    answer = call(op='routes', cell=cell_json(cell),
                  endpoints=[dict(Nets=n, At=p, Ref=r) for n, p, r in endpoints], netmap=netmap, scale=scale)
    return answer['records'], answer['count']


def load_rules(text=None):
    return call(op='rules', text=text or '')['rules']


def place(symbol, source_pins, package_pins, orientation, origin, rules):
    answer = call(op='place', symbol=symbol, source=source_pins, destination=package_pins,
                  orient=orientation, at=origin, rules=rules)
    return tuple(answer['xy']), answer['orient']


orientation_api = SimpleNamespace(load_rules=load_rules, place=place)


def interfaces(text):
    return call(op='interfaces', text=text)['interfaces']


def run_job(request):
    result = subprocess.run([str(BINARY), '--resources', str(RESOURCES), str(request)],
                            text=True, encoding='utf-8', capture_output=True)
    if result.returncode:
        raise ExportError(result.stderr)
    return json.loads(Path(request).with_name('result.json').read_text(encoding='utf-8'))
