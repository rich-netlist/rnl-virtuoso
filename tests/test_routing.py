import copy
import json
import os
from pathlib import Path
import re
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]/'tests/reference'))
from rnl_export import edif, resources
from rnl_export.convert import convert
from rnl_export.edif import Cell, Net
from rnl_export.routing import draw_routes
from fixtures import primitive_drawing
import test_symbols

if os.environ.get('RNL_EXPORT_BACKEND', 'go') == 'go':
    from go_backend import convert, draw_routes


class RouteTests(unittest.TestCase):
    def route(self, bits, path):
        return Net(bits[0], paths=[path], bits=tuple(bits))

    def draw(self, routes, endpoints=()):
        cell = Cell('work', 'test', 'test', routes=routes)
        names = {n: n for route in routes for n in route.bits}
        names.update((n, n) for nets, _, _ in endpoints for n in nets)
        records, count = draw_routes(cell, endpoints, names, 1)
        return '\n'.join(records), count

    def test_separate_stubs_on_one_net_do_not_add_off_route_pin_tails(self):
        routes = [self.route(['A'], [(-5, 0), (0, 0)]), self.route(['A'], [(10, 0), (15, 0)])]
        endpoints = [(('A',), (0, 0), '#P:1'), (('A',), (10, 0), '#Q:1')]
        text, count = self.draw(routes, endpoints)
        self.assertEqual(count, 2)
        self.assertEqual(text.count('#P:1'), 1)
        self.assertEqual(text.count('#Q:1'), 1)
        self.assertNotIn('= (0, 0)', text)
        self.assertNotIn('= (10, 0)', text)

    def test_repeated_bus_connection_reuses_pin_without_coordinate_connector(self):
        routes = [self.route(['A'], [(-5, 0), (0, 0)]), self.route(['A', 'A'], [(-5, 0), (0, 0)])]
        text, count = self.draw(routes, [(('A',), (0, 0), '#P:1')])
        self.assertEqual(count, 1)
        self.assertEqual(text.count('#P:1'), 1)
        self.assertNotIn('= (0, 0)', text)

    def test_direct_contacts_use_pin_to_pin_without_old_coordinate(self):
        text, count = self.draw([], [(('A',), (0, 0), '#P:1'), (('A',), (0, 0), '#Q:1')])
        self.assertEqual(count, 1)
        self.assertIn('{#Q:1 #P:1}', text)
        self.assertNotIn('= (0, 0)', text)

    def test_bus_tap_keeps_shared_point_in_both_route_orders(self):
        routes = [self.route(['A1', 'A0'], [(0, 0), (10, 0)]), self.route(['A0'], [(5, 0), (5, 5)])]
        for ordered in (routes, list(reversed(routes))):
            text, count = self.draw(ordered)
            point = re.search(r'(%p\d+) = \(5, 0\)', text)[1]
            self.assertEqual(count, 2)
            self.assertEqual(len([line for line in text.splitlines() if '@WIRE' in line and point in line]), 2)
            self.assertIn('$JUNCTION @ATTACH '+point, text)

    def test_package_swap_keeps_one_terminal_endpoint_per_switch_stub(self):
        pins = [(-2, 3), (-2, -3), (2, 3), (2, -3)]
        for frame in edif.MATRICES:
            source, _ = primitive_drawing('switch', pins, frame)
            tree = edif.parse(source)
            library = next(n for n in tree if edif.tag(n) == 'library' and n[1] == 'work')
            contents = edif.first(edif.first(edif.first(library, 'cell'), 'view'), 'contents')
            repeated = copy.deepcopy(next(n for n in contents if edif.tag(n) == 'net' and n[1] == 'N3'))
            repeated[1] = ['rename', 'repeat', 'N3']
            contents.append(repeated)
            def serialize(node):
                return '('+' '.join(serialize(n) for n in node)+')' if isinstance(node, list) else json.dumps(node)
            files, _ = convert('// Fixture\n// Library name: work\n// Cell name: top\nX0 (N1 N2 N3 N4) relay\n', serialize(tree), {('analogLib', 'switch'): ['P1', 'P2', 'P3', 'P4']}, resources.read('eda-symbol.rnl'), 'top', dialect='spectre')
            for packages in ('eda', 'standard', 'standard-with-eda-style'):
                with self.subTest(orientation=frame, packages=packages):
                    drawing = dict(files)
                    if packages != 'eda':
                        drawing['top.scs'] = re.sub(r'^//\s*@USE-PACKAGE\(name="eda-symbol".*\n', '', drawing['top.scs'], flags=re.M)
                    if packages == 'standard':
                        drawing['top.scs'] = re.sub(r'^//\s*@USE-PACKAGE\(name="eda-style".*\n', '', drawing['top.scs'], flags=re.M)
                    self.assertEqual(drawing['top.scs'].count('@WIRE('), 4)
                    checker = test_symbols.SymbolTests()
                    level = checker.dump(drawing)
                    self.assertEqual(len(level['wires']), 4)
                    instance = next(i for i in level['items'] if i['id'] == '#X0')
                    symbol = level['symbols'][instance['symbol']]
                    for pin in symbol['pins']:
                        position = list(edif.transform(pin['pos'], instance['orient'], instance['at']))
                        self.assertEqual(sum(position == w['points'][0] or position == w['points'][-1] for w in level['wires']), 1)


if __name__ == '__main__':
    unittest.main()
