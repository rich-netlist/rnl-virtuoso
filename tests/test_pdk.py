"""Original PDK-like fixtures; no foundry code or symbol graphics."""
import os
import re
import unittest

from fixtures import primitive_drawing, drawing, spectre_netlist
from rnl_export import ExportError, edif, resources
from go_backend import convert
import test_symbols


@unittest.skipIf(os.environ.get('RNL_EXPORT_BACKEND') == 'python', 'Native PDK expansions are a Go backend extension')
class PDKTests(unittest.TestCase):
    def test_optional_cdf_terminals_require_complete_active_pin_coverage(self):
        source, _ = primitive_drawing('vsource', [(0, 3), (0, -3)], 'R0')
        native = '// Library name: work\n// Cell name: top\nX0 (N1 N2) vsource type=pwl wave=[0 0 1n 1]\n'
        orders = {('analogLib', 'vsource'): ['P1', 'P2', 'MODP', 'MODN']}
        files, report = convert(native, source, orders, resources.read('eda-symbol.rnl'), 'top', dialect='spectre')
        self.assertIn(native.splitlines()[-1], files['top.scs'])
        self.assertTrue(any('MODP, MODN' in note for note in report['notes']))
        test_symbols.SymbolTests().dump(files)
        for metadata, text in [(['P1', 'MODP'], native), (orders['analogLib', 'vsource'], native.replace('(N1 N2)', '(N1 N2 EXTRA)'))]:
            with self.assertRaisesRegex(ExportError, 'interface differ|node count differs'):
                convert(text, source, {('analogLib', 'vsource'): metadata}, resources.read('eda-symbol.rnl'), 'top', dialect='spectre')

    def test_explicitly_ignored_instance_is_not_a_missing_native_device(self):
        source = drawing().replace('(instance R0 ', '(instance R0 (property nlAction (string "ignore")) ')
        native = spectre_netlist().replace('R0 (A B) resistor r=(Rbase + 1k)\n', '')
        files, report = convert(native, source, {}, resources.read('eda-symbol.rnl'), 'top', dialect='spectre')
        self.assertEqual(report['statistics']['instances'], 1)
        self.assertTrue(any('nlAction=ignore: work/leaf/R0' in n for n in report['notes']))
        self.assertNotIn('#"R0"', files['top.scs'])
        # Suppression requires an explicit source property, never a name guess.
        with self.assertRaisesRegex(ExportError, 'No Spectre instance'):
            convert(native, drawing(), {}, resources.read('eda-symbol.rnl'), 'top', dialect='spectre')

    def test_edif_escaped_cell_and_instance_names_match_spectre(self):
        drawing, _ = primitive_drawing('res', [(0, 3), (0, -3)], 'R0')
        drawing = drawing.replace('(cell top ', '(cell &_experiment ').replace('X0', '&_R0')
        native = '// Library name: work\n// Cell name: _experiment\n_R0 (N1 N2) resistor r=1k\n'
        files, report = convert(native, drawing, {('analogLib', 'res'): ['P1', 'P2']}, resources.read('eda-symbol.rnl'), '_experiment', dialect='spectre')
        self.assertEqual(report['entry'], '_experiment.scs')
        self.assertIn('#"_R0"', files['_experiment.scs'])
        self.assertNotIn('#"&_R0"', files['_experiment.scs'])

    def fixture(self, frame='R0', kind='Series'):
        drawing, expected = primitive_drawing('segmented', [(0, 3), (0, -3), (2, 0)], frame)
        drawing = drawing.replace('analogLib', 'demoPDK')
        # Exercise model text inherited from an EDIF keyword display as well.
        drawing = drawing.replace('(symbol (portImplementation', '(symbol (keywordDisplay CELL (display text (origin (pt 0 0)))) (portImplementation', 1)
        connections = [('N1', 'mid0', 'N3'), ('mid0', 'mid1', 'N3'), ('mid1', 'N2', 'N3')]
        if kind == 'Parallel':
            connections = [('N1', 'N2', 'N3')]*3
        native = '// Original fixture\nsimulator lang=spectre\n// Library name: work\n// Cell name: top\n'
        native += '//'+kind+' configuration of X0\n'
        # Deliberately unrelated member names: membership must come from native
        # expansion delimiters, rather than an X0 prefix heuristic.
        for name, nodes in zip(('segment_a', 'segment_b', 'segment_c'), connections):
            native += name+' ('+' '.join(nodes)+') demo_res lr=11u wr=2u\n'
        native += '//End of X0\n'
        orders = {('demoPDK', 'segmented'): ['P1', ' ', 'P2', '', 'P3']}
        return native, drawing, orders, expected

    def test_series_and_parallel_keep_native_text_and_one_source_symbol(self):
        for kind in ('Series', 'Parallel'):
            for frame in edif.MATRICES:
                with self.subTest(kind=kind, frame=frame):
                    native, drawing, orders, expected = self.fixture(frame, kind)
                    files, report = convert(native, drawing, orders, resources.read('eda-symbol.rnl'), 'top', dialect='spectre')
                    output = files['top.scs']
                    plain = re.sub(r'// @RNL\b.*?^// }\n', '', output, flags=re.S | re.M)
                    self.assertEqual(plain, native)
                    self.assertEqual(report['statistics']['instances'], 3)
                    self.assertEqual(report['statistics']['expanded_instances'], 1)
                    self.assertIn('%expanded_0:p1', output)
                    self.assertIn('%expanded_0:p2', output)
                    self.assertIn('%expanded_0:p3', output)
                    self.assertEqual(output.count('@WIRE('), 3)
                    level = test_symbols.SymbolTests().dump(files)
                    item = next(i for i in level['items'] if i['id'] == '%expanded_0')
                    symbol = level['symbols'][item['symbol']]
                    actual = {p['terminals'][0]: edif.transform(p['pos'], item['orient'], item['at']) for p in symbol['pins']}
                    self.assertEqual(actual, {'p'+str(i): p for i, p in enumerate(expected, 1)})
                    self.assertEqual(len([i for i in level['items'] if level['symbols'][i['symbol']]['pins']]), 1)

    def test_expansion_boundary_must_match_the_edif_connections(self):
        native, drawing, orders, _ = self.fixture()
        with self.assertRaisesRegex(ExportError, 'connectivity disagree'):
            convert(native.replace('(mid1 N2 N3)', '(mid1 WRONG N3)'), drawing, orders, resources.read('eda-symbol.rnl'), 'top', dialect='spectre',
                    name_maps={'top': {'net': {'N2': 'N2'}}})

    def test_no_prefix_matching_or_hidden_internal_branch(self):
        native, drawing, orders, _ = self.fixture()
        for text, error in [(native+'branch (mid0 N2 N3) demo_res\n', 'external branch'),
                            (native+'X0_extra (N1 N2 N3) demo_res\n', 'absent from EDIF')]:
            with self.assertRaisesRegex(ExportError, error):
                convert(text, drawing, orders, resources.read('eda-symbol.rnl'), 'top', dialect='spectre')
