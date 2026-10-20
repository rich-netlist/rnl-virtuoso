import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT/'tests/reference'))
from rnl_export import ExportError, edif, orientation, resources
from rnl_export.convert import convert
from rnl_export.symbols import package_pins, symbol_definition
from rnl_export.symbols import STANDARD
from fixtures import drawing, netlist, spectre_netlist, primitive_drawing

if os.environ.get('RNL_EXPORT_BACKEND', 'go') == 'go':
    from go_backend import convert, symbol_definition, orientation_api as orientation

PACKAGE = resources.read('eda-symbol.rnl')
BINARY = Path(os.environ.get('RNL_BIN', str(ROOT/'target'/('rnl.exe' if os.name == 'nt' else 'rnl'))))


class SymbolTests(unittest.TestCase):
    def dump(self, files, title=None):
        if not BINARY.is_file():
            self.skipTest('Build rnl-cli or set RNL_BIN')
        with tempfile.TemporaryDirectory() as directory:
            for name, text in files.items():
                (Path(directory)/name).write_text(text, encoding='utf-8')
            process = subprocess.run([str(BINARY), 'dump', str(Path(directory)/'top.scs')], text=True, encoding='utf-8', capture_output=True)
            self.assertEqual(process.returncode, 0, process.stdout+process.stderr)
            result = json.loads(process.stdout)
            self.assertFalse([d for d in result['diagnostics'] if d['severity'] in ('error', 'warning')], result['diagnostics'])
            return next(level for level in result['levels'] if level['title'] == title) if title else result['levels'][0]

    @unittest.skipIf(os.environ.get('RNL_EXPORT_BACKEND') == 'python', 'Dialect-specific packages are a Go backend extension')
    def test_spectre_instance_letters_do_not_override_cell_symbols(self):
        files, _ = convert(spectre_netlist(), drawing(), {('analogLib', 'res'): ['PLUS', 'MINUS']}, PACKAGE, 'top', dialect='spectre')
        level = self.dump(files, 'top')
        item = next(i for i in level['items'] if i['id'] == '#I0')
        symbol = level['symbols'][item['symbol']]
        self.assertNotEqual(symbol['name'], '$CURRENT_SOURCE')
        self.assertEqual(len(symbol['pins']), 2)
        self.assertNotIn('@SELECT-INSTANCE(name="I*")', files['eda-symbol.rnl'])
        source, _ = primitive_drawing('idc', [(0, 3), (0, -3)], 'R0')
        source = source.replace('X0', 'I0')
        native = '// Library name: work\n// Cell name: top\nI0 (N1 N2) isource dc=1u\n'
        files, _ = convert(native, source, {('analogLib', 'idc'): ['P1', 'P2']}, PACKAGE, 'top', dialect='spectre')
        level = self.dump(files)
        item = next(i for i in level['items'] if i['id'] == '#I0')
        self.assertEqual(level['symbols'][item['symbol']]['name'], '$CURRENT_SOURCE')

    def test_custom_symbols_have_pins_without_pin_artwork_or_style(self):
        cell = edif.read(drawing())['work', 'leaf']
        text = symbol_definition(cell, '%test', {'a': 1, 'b': 2}, 10, [])
        self.assertEqual(text.count('@PIN('), 2)
        self.assertEqual(text.count('@RECTANGLE'), 1)  # Only the original body.
        for forbidden in ('#ff0000', 'width=0.8', 'direction=', 'linecolor=', 'fillcolor=', 'textcolor=', 'linewidth=', 'linestyle=', '@SET-STYLE'):
            self.assertNotIn(forbidden, text)

    def test_configuration_covers_changed_symbols_and_rejects_bad_rules(self):
        rules = orientation.load_rules()
        self.assertEqual(len(rules), 18)
        self.assertTrue(set(rules).issubset(package_pins(PACKAGE)))
        bad = json.loads(resources.read('symbol-orientations.json'))
        bad['symbols']['DIODE']['package_to_source'] = 'guessed-rotation'
        with self.assertRaisesRegex(ExportError, 'Invalid package orientation'):
            orientation.load_rules(json.dumps(bad))

    def test_standard_diodes_and_relays_keep_world_pin_positions(self):
        # Literal source pin coordinates: diode anode at top, relay contacts at left.
        cases = [('diode', [(0, 3), (0, -3)], 'diode', 'DIODE'),
                 ('switch', [(-2, 3), (-2, -3), (2, 3), (2, -3)], 'relay', 'VOLTAGE_CONTROLLED_SWITCH')]
        for cell, pins, model, symbol in cases:
            for orient in edif.MATRICES:
                with self.subTest(cell=cell, orientation=orient):
                    drawing_text, expected = primitive_drawing(cell, pins, orient)
                    native = '// Fixture\n// Library name: work\n// Cell name: top\nX0 ('+' '.join('N%d' % i for i in range(1, len(pins)+1))+') '+model+'\n'
                    files, _ = convert(native, drawing_text, {('analogLib', cell): ['P%d' % i for i in range(1, len(pins)+1)]}, PACKAGE, 'top', dialect='spectre')
                    level = self.dump(files)
                    instance = next(i for i in level['items'] if i['id'] == '#X0')
                    view = level['symbols'][instance['symbol']]
                    self.assertEqual(view['name'], '$'+symbol)
                    actual = {}
                    for pin in view['pins']:
                        actual[pin['terminals'][0]] = edif.transform(pin['pos'], instance['orient'], instance['at'])
                    for i, point in enumerate(expected, 1):
                        self.assertEqual(actual[str(i)], point)
                    # Exact pin alignment needs only the original stubs, no repair leads.
                    self.assertEqual(files['top.scs'].count('@WIRE('), len(pins))

    def test_nonrigid_pin_relocations_connect_to_current_terminal(self):
        rules = orientation.load_rules()
        pins = package_pins(PACKAGE)['TRANSMISSION_LINE']
        old = [(-4, 3), (-4, -3), (4, 3), (4, -3)]
        xy, orient = orientation.place('TRANSMISSION_LINE', old, [pins[str(i)] for i in range(1, 5)], 'MY', (20, -10), rules)
        # Signal terminals stay put; wire endpoints follow displaced references.
        for i in (0, 2):
            self.assertEqual(edif.transform(pins[str(i+1)], orient, xy), edif.transform(old[i], 'MY', (20, -10)))
        self.assertNotEqual(edif.transform(pins['2'], orient, xy), edif.transform(old[1], 'MY', (20, -10)))
        source, expected = primitive_drawing('test_tline', old, 'MY')
        with patch.dict(STANDARD, {'test_tline': 'TRANSMISSION_LINE'}):
            files, _ = convert('// Fixture\n// Library name: work\n// Cell name: top\nX0 (N1 N2 N3 N4) tline\n', source, {('analogLib', 'test_tline'): ['P1', 'P2', 'P3', 'P4']}, PACKAGE, 'top', dialect='spectre')
        self.dump(files)
        self.assertEqual(files['top.scs'].count('@WIRE('), 4)
        self.assertRegex(files['top.scs'], r'&"N2" @WIRE\(mode="straight"\) \{#"X0":2 ')
        self.assertRegex(files['top.scs'], r'&"N4" @WIRE\(mode="straight"\) \{#"X0":4 ')

    def test_rigid_package_changes_have_correct_compensation_in_all_frames(self):
        # Coordinates of the actual legacy package, independent of the code under test.
        old_pins = {
            'DIODE': [(0, 3), (0, -3)], 'ZENER_DIODE': [(0, -3), (0, 3)], 'SCHOTTKY': [(0, 3), (0, -3)],
            'SWITCH': [(3, 0), (-3, 0)], 'CLOSED_SWITCH': [(3, 0), (-3, 0)],
            'SPDT_SWITCH_SYMBOL': [(3, 1), (-3, 2), (-3, -1)],
            'SP3TSWITCH': [(4, 0), (-4, 2), (-4, 0), (-4, -2)],
            'SP4TSWITCH': [(4, 0), (-4, 3), (-4, 1), (-4, -1), (-4, -3)],
            'VOLTAGE_CONTROLLED_SWITCH': [(-2, 3), (-2, -3), (2, 3), (2, -3)],
            'TRANSFORMER': [(-2, 3), (-2, -3), (2, 3), (2, -3)],
        }
        rules, pins = orientation.load_rules(), package_pins(PACKAGE)
        for name, old in old_pins.items():
            new = [pins[name][str(i)] for i in range(1, len(old)+1)]
            for frame in edif.MATRICES:
                with self.subTest(symbol=name, frame=frame):
                    origin, result = orientation.place(name, old, new, frame, (20, -10), rules)
                    self.assertEqual(origin, (20, -10))
                    self.assertEqual([edif.transform(p, result, origin) for p in new], [edif.transform(p, frame, (20, -10)) for p in old])

    def test_symbol_and_style_packages_are_referenced_separately(self):
        files, _ = convert(netlist(), drawing(), {('analogLib', 'res'): ['PLUS', 'MINUS']}, PACKAGE, 'top')
        self.assertIn('@USE-PACKAGE(name="eda-symbol", version="0.1", path="eda-symbol.rnl")', files['top.sp'])
        self.assertIn('@USE-PACKAGE(name="eda-style", version="0.1", path="eda-style.rnl")', files['top.sp'])
        self.assertEqual(files['eda-symbol.rnl'], PACKAGE)
        self.assertNotIn('@DEFINE-SYMBOL', files['eda-style.rnl'])

    def test_custom_symbol_accepts_an_external_style(self):
        # Use a source master without a standard mapping so its symbol is exported.
        drawing_text, _ = primitive_drawing('custom_test', [(0, 3), (0, -3)], 'R0')
        native = '// Fixture\n// Library name: work\n// Cell name: top\nX0 (N1 N2) resistor r=1k\n'
        files, _ = convert(native, drawing_text, {('analogLib', 'custom_test'): ['P1', 'P2']}, PACKAGE, 'top', dialect='spectre')
        files['top.scs'] = re.sub(r'^//\s*@USE-PACKAGE\(name="eda-style".*\n', '', files['top.scs'], flags=re.M)
        files['top.scs'] += '// @RNL { $ALT = @DEFINE-STYLE(linecolor="#123456", linewidth=2) {}; # @SELECT-INSTANCE => @SET-STYLE $ALT }\n'
        level = self.dump(files)
        item = next(i for i in level['items'] if i['id'] == '#X0')
        self.assertEqual(item['style']['linecolor'], '#123456')
        self.assertEqual(item['style']['linewidth'], 2)
        self.assertTrue(all(p['dir'] is None for p in level['symbols'][item['symbol']]['pins']))

    def test_archive_contains_both_libraries_and_editable_orientation_table(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory)/'plugin.tar.gz'
            backend = os.environ.get('RNL_EXPORT_LINUX_BIN', str(ROOT/'target/rnl-virtuoso-linux-amd64'))
            result = subprocess.run([sys.executable, str(ROOT/'scripts/package.py'), str(archive), '--backend', backend], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            with tarfile.open(archive) as bundle:
                names = bundle.getnames()
                self.assertIn('rnl-virtuoso/bin/rnl-virtuoso', names)
                self.assertEqual(bundle.getmember('rnl-virtuoso/bin/rnl-virtuoso').mode, 0o755)
                self.assertFalse(any(name.endswith('.py') for name in names))
                for name in ('eda-symbol.rnl', 'eda-style.rnl', 'symbol-orientations.json'):
                    self.assertIn('rnl-virtuoso/resources/'+name, names)


if __name__ == '__main__':
    unittest.main()
