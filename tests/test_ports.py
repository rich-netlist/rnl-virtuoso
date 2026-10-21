import os
import re
import unittest

from fixtures import drawing, netlist, spectre_netlist
import test_symbols
from go_backend import convert


@unittest.skipIf(os.environ.get('RNL_EXPORT_BACKEND') == 'python', 'Port directions use the Go backend')
class PortTests(unittest.TestCase):
    def export(self, array=False, frame='R0', directions=('INPUT', 'OUTPUT'), dialect='spectre', repeated=False):
        source = drawing(array)
        for name, direction, x in zip(('A', 'B'), directions, (0, 60)):
            port = '(array (rename %s "%s<1:0>") 2)' % (name, name) if array else name
            if direction:
                source = source.replace('(port '+port+')', '(port '+port+' (direction '+direction+'))')
            marker = '(portImplementation %s (connectLocation (figure pin (dot (pt %d 0)))) (instance pin (transform (orientation %s) (origin (pt %d 0)))))' % (name, x, frame, x)
            source = source.replace('(portImplementation %s (connectLocation (figure pin (dot (pt %d 0)))))' % (name, x), marker)
            if repeated:
                # Repeat only schematic placements, not the interface symbol.
                # The transform origin, not the connectLocation, places a port.
                other = -40 if name == 'A' else 100
                duplicate = '(portImplementation %s (connectLocation (figure pin (dot (pt %d 0)))) (instance pin (transform (orientation R180) (origin (pt %d 0)))))' % (name, x, other)
                interface, schematic = source.split('(contents', 1)
                source = interface+'(contents'+schematic.replace(marker, marker+duplicate)
        native = spectre_netlist(array) if dialect == 'spectre' else netlist(array)
        files, _ = convert(native, source, {('analogLib', 'res'): ['PLUS', 'MINUS']},
                           test_symbols.PACKAGE, 'top', dialect=dialect)
        return files

    def test_repeated_scalar_and_bus_port_markers_keep_each_placement(self):
        for array in (False, True):
            for dialect in ('spectre', 'cdl'):
                for frame in ('R0', 'R90', 'R180', 'R270', 'MX', 'MY', 'MXR90', 'MYR90'):
                    with self.subTest(array=array, dialect=dialect, frame=frame):
                        original = self.export(array, frame, dialect=dialect)
                        files = self.export(array, frame, dialect=dialect, repeated=True)
                        entry = 'top.scs' if dialect == 'spectre' else 'top.sp'
                        # Placing a second glyph must not duplicate the native
                        # port list, add devices, or change native connectivity.
                        native = lambda text: [line for line in text.splitlines() if line and not line.startswith(('//', '*'))]
                        self.assertEqual(native(original[entry]), native(files[entry]))
                        for title in ('top', 'leaf'):
                            level = test_symbols.SymbolTests().dump(files, title, entry)
                            self.assertFalse(level['flylines'])
                            suffix = '_marker' if array else ''
                            for index, x, orient in ((0, 0, frame), (1, -4, 'R180'), (2, 6, frame), (3, 10, 'R180')):
                                anchor = next(i for i in level['items'] if i['id'] == '%%port_%d' % index)
                                marker = next(i for i in level['items'] if i['id'] == '%%port_%d%s' % (index, suffix))
                                self.assertEqual(marker['at'], [x, 0])
                                self.assertEqual(marker['orient'], orient)
                                self.assertEqual(anchor['at'], marker['at'])
                                expected = 'input_port_symbol' if index < 2 else 'output_port_symbol'
                                self.assertEqual(level['symbols'][marker['symbol']]['name'], expected)
                                self.assertTrue(any('%%port_%d:' % index in wire for wire in files[entry].splitlines() if '@WIRE(' in wire))

    def assert_ports(self, files, array, frame, symbols=('input_port_symbol', 'output_port_symbol'), color='#ff0000'):
        for title in ('top', 'leaf'):
            entry = 'top.scs' if 'top.scs' in files else 'top.sp'
            level = test_symbols.SymbolTests().dump(files, title, entry)
            self.assertEqual(len(level['wires']), 4)  # Two source stubs and coincident pin contacts.
            self.assertFalse(level['flylines'])
            for index, (name, expected, x) in enumerate(zip(('A', 'B'), symbols, (0, 6))):
                anchor_id = '%%port_%d' % index
                marker_id = anchor_id+'_marker' if array else anchor_id
                marker = next(i for i in level['items'] if i['id'] == marker_id)
                symbol = level['symbols'][marker['symbol']]
                self.assertEqual(symbol['name'], expected)
                self.assertEqual(marker['style']['linecolor'], color)
                self.assertEqual(marker['style']['textcolor'], color)
                self.assertEqual(marker['at'], [x, 0])
                self.assertEqual(marker['orient'], frame)
                label = name+'<1:0>' if array else name
                labels = [t for t in level['texts'] if ''.join(run.get('text', '') for line in t['lines'] for run in line) == label]
                self.assertTrue(labels)
                self.assertTrue(any(t['style']['textcolor'] == color for t in labels))
                if array:
                    anchor = next(i for i in level['items'] if i['id'] == anchor_id)
                    pin_symbol = level['symbols'][anchor['symbol']]
                    self.assertFalse(pin_symbol['prims'])
                    self.assertFalse(pin_symbol['nested'])
                    self.assertEqual(pin_symbol['pins'][0]['terminals'], ['1', '2'])
                    self.assertEqual(anchor['at'], marker['at'])
                    self.assertFalse(anchor['texts'])

    def test_scalar_and_bus_ports_keep_input_output_and_all_orientations(self):
        for array in (False, True):
            for frame in ('R0', 'R90', 'R180', 'R270', 'MX', 'MY', 'MXR90', 'MYR90'):
                with self.subTest(array=array, frame=frame):
                    self.assert_ports(self.export(array, frame), array, frame)

    def test_port_markers_follow_the_theme_in_both_native_formats(self):
        for array in (False, True):
            for dialect in ('spectre', 'cdl'):
                with self.subTest(array=array, dialect=dialect):
                    files = self.export(array, dialect=dialect)
                    files['eda-style.rnl'] = files['eda-style.rnl'].replace('#ff0000', '#123456')
                    self.assert_ports(files, array, 'R0', color='#123456')
                    self.assertNotIn('@SET-STYLE', files['top.scs' if dialect == 'spectre' else 'top.sp'])

    def test_inout_and_missing_direction_keep_the_io_marker(self):
        for array in (False, True):
            with self.subTest(array=array):
                self.assert_ports(self.export(array, directions=('INOUT', None)), array, 'R0',
                                  symbols=('$IO_PORT_SYMBOL', '$IO_PORT_SYMBOL'))

    def test_directional_ports_allow_replacing_the_symbol_package(self):
        for array in (False, True):
            for keep_style in (False, True):
                with self.subTest(array=array, keep_style=keep_style):
                    files = self.export(array)
                    files['top.scs'] = re.sub(r'^//\s*@USE-PACKAGE\(name="eda-symbol".*\n', '', files['top.scs'], flags=re.M)
                    if not keep_style:
                        files['top.scs'] = re.sub(r'^//\s*@USE-PACKAGE\(name="eda-style".*\n', '', files['top.scs'], flags=re.M)
                    for title in ('top', 'leaf'):
                        level = test_symbols.SymbolTests().dump(files, title)
                        self.assertEqual(len(level['wires']), 4)
                        self.assertFalse(level['flylines'])
                        for i, expected in enumerate(('input_port_symbol', 'output_port_symbol')):
                            marker = next(item for item in level['items'] if item['id'] == '%%port_%d%s' % (i, '_marker' if array else ''))
                            self.assertEqual(level['symbols'][marker['symbol']]['name'], expected)
                            self.assertEqual(marker['style']['linecolor'], '#ff0000' if keep_style else '#1d1d1f')
