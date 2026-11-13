import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT/'tests/support'))
from rnl_export import ExportError
from rnl_export import edif
from rnl_export import resources
from fixtures import drawing, netlist, spectre_netlist, marker_drawing, clean_check
from go_backend import convert, run_job, interfaces

PACKAGE = resources.read('eda-symbol.rnl')
ORDERS = {('analogLib', 'res'): ['PLUS', 'MINUS']}


class ExportTests(unittest.TestCase):
    def export(self, array=False, **options):
        electrical = spectre_netlist(array) if options.get('dialect') == 'spectre' else netlist(array)
        return convert(electrical, drawing(array), ORDERS, PACKAGE, 'top', **options)

    def native_leaf(self, name, dialect):
        # Cell metadata and EDIF still say work/leaf; only the native netlist
        # chooses a different subcircuit identifier, as library disambiguation can.
        if dialect == 'spectre':
            escaped = re.sub(r'[^a-zA-Z0-9_]', lambda m: '\\'+m[0], name)
            return (spectre_netlist().replace('subckt leaf ', 'subckt '+escaped+' ')
                    .replace('ends leaf\n', 'ends '+escaped+'\n')
                    .replace(') leaf\n', ') '+escaped+'\n'))
        return (netlist().replace('.SUBCKT leaf ', '.SUBCKT '+name+' ')
                .replace('/ leaf\n', '/ '+name+'\n'))
    def test_split_filenames_follow_source_cell_and_keep_native_subcircuit_name(self):
        for dialect, extension, prefix in [('cdl', 'sp', '.'), ('spectre', 'scs', '')]:
            with self.subTest(dialect=dialect):
                name = 'Leaf__otherLib_2'
                files, report = convert(self.native_leaf(name, dialect), drawing(), ORDERS, PACKAGE, 'top', dialect=dialect, split=True)
                filename = 'leaf.'+extension
                self.assertIn(filename, files)
                self.assertNotIn(name+'.'+extension, files)
                self.assertNotIn(prefix+'include ', files[report['entry']])
                self.assertIn(('subckt ' if dialect == 'spectre' else '.SUBCKT ')+name+' A B\n', files[filename])
                self.assertIn((') ' if dialect == 'spectre' else '/ ')+name+'\n', files[report['entry']])
    def test_split_native_alias_does_not_conflict_with_cell_filename(self):
        files, _ = convert(self.native_leaf('TOP', 'spectre'), drawing(), ORDERS, PACKAGE, 'top', dialect='spectre', split=True)
        self.assertIn('leaf.scs', files)
        self.assertIn('subckt TOP A B', files['leaf.scs'])
        self.assertIn('subckt top A B', files['top.scs'])

    def test_entry_uses_exported_cell_name_in_every_mode(self):
        for dialect, extension in [('cdl', 'sp'), ('spectre', 'scs')]:
            for has_ports in (False, True):
                if has_ports:
                    native = spectre_netlist() if dialect == 'spectre' else netlist()
                    source = drawing()
                else:
                    native = ('// Library name: work\n// Cell name: top\nR0 (A 0) resistor r=1k\n' if dialect == 'spectre' else
                              '* Fixture\n* Library Name: work\n* Cell Name: top\n.SUBCKT top\nR0 A 0 1k\n.ENDS\n')
                    source = marker_drawing()
                source = source.replace('(cell top ', '(cell MyADC ')
                native = (native.replace('Cell Name: top\n', 'Cell Name: MyADC\n')
                          .replace('Cell name: top\n', 'Cell name: MyADC\n')
                          .replace('.SUBCKT top', '.SUBCKT native_root_alias'))
                for options in ({}, {'split': True}, {'hierarchy': False}):
                    with self.subTest(dialect=dialect, has_ports=has_ports, options=options):
                        files, report = convert(native, source, ORDERS, PACKAGE, 'MyADC', dialect=dialect, **options)
                        self.assertEqual(report['entry'], 'MyADC.'+extension)
                        self.assertIn(report['entry'], files)
                        self.assertNotIn('top.'+extension, files)
                        self.assertEqual(report['root_kind'], 'subcircuit' if has_ports else 'top')

    def test_spectre_preserves_electrical_text_and_native_hierarchy(self):
        files, _ = self.export(dialect='spectre')
        plain = re.sub(r'// @RNL\b.*?^// }\n', '', files['top.scs'], flags=re.S | re.M)
        self.assertEqual(plain.replace('subckt top A B\n', '').replace('ends top\n', ''), spectre_netlist())
        self.assertIn('subckt top A B\n', plain)
        split, _ = self.export(dialect='spectre', split=True)
        self.assertNotIn('include ', split['top.scs'])
        self.assertNotIn('#"I0" @SET-SYMBOL', split['top.scs'])
        self.assertIn('# @SET-SYMBOL %block_symbol', split['leaf.scs'])
        current, _ = self.export(dialect='spectre', hierarchy=False)
        self.assertNotIn('subckt leaf', current['top.scs'])
        self.assertIn('#"I0" @SET-SYMBOL %symbol_0', current['top.scs'])

    def test_spectre_connectivity_disagreement_fails(self):
        # The parser cases of this test are Go unit tests (parsers_test.go).
        with self.assertRaisesRegex(ExportError, 'connectivity disagrees'):
            convert(spectre_netlist().replace('R0 (A B)', 'R0 (a B)'), drawing(), ORDERS, PACKAGE, 'top', dialect='spectre')

    def test_spectre_uses_native_bus_name_maps(self):
        maps = {name: {'net': {p+'<'+i+'>': p+'_'+i for p in ('A', 'B') for i in ('1', '0')}} for name in ('leaf', 'top')}
        maps['leaf']['inst'] = {'R0<1>': 'R0_1', 'R0<0>': 'R0_0'}
        native = spectre_netlist(True).replace('\\<', '_').replace('\\>', '')
        files, _ = convert(native, drawing(True), ORDERS, PACKAGE, 'top', dialect='spectre', name_maps=maps)
        self.assertIn('{#"R0_1" #"R0_0"} @BUNDLE', files['top.scs'])
        self.assertIn('@PIN(terminal=["A_1", "A_0"])', files['top.scs'])
        with self.assertRaisesRegex(ExportError, 'name map and EDIF connectivity disagree'):
            convert(native.replace('R0_1 (A_1 B_1)', 'R0_1 (A_0 B_1)'), drawing(True), ORDERS, PACKAGE, 'top', dialect='spectre', name_maps=maps)

    def test_current_symbol_export_and_ports_choose_subcircuit(self):
        # Reuse the original fixture's block shape as a standalone top symbol.
        tree = edif.parse(drawing())
        lib = next(n for n in tree if edif.tag(n) == 'library' and n[1] == 'work')
        leaf = next(n for n in lib if edif.tag(n) == 'cell' and n[1] == 'leaf')
        import copy
        symbol = copy.deepcopy(leaf)
        symbol[1] = 'top'
        view = next(n for n in symbol if edif.tag(n) == 'view')
        view[:] = [n for n in view if edif.tag(n) != 'contents']
        def serialize(node):
            return '('+' '.join(serialize(n) for n in node)+')' if isinstance(node, list) else json.dumps(node)
        source = serialize(['edif', 'test', ['edifVersion', '2', '0', '0'], ['library', 'work', symbol]])
        for dialect in ('cdl', 'spectre'):
            for options in ({}, {'split': True}, {'hierarchy': False}):
                files, report = self.export(dialect=dialect, root_symbol=source, **options)
                self.assertEqual(report['root_kind'], 'subcircuit')
                self.assertIsNone(report['symbol_file'])
                self.assertNotIn('cell-symbol.rnl', files)
                self.assertNotIn('symbols.rnl', files)
                text = files[report['entry']]
                self.assertIn('# @SET-SYMBOL %block_symbol', text.split('subckt top' if dialect == 'spectre' else '.SUBCKT top')[1])
        files, report = convert('* Fixture\n* Library Name: work\n* Cell Name: top\n.SUBCKT top\nR0 A 0 1k\n.ENDS\n', marker_drawing(), ORDERS, PACKAGE, 'top')
        self.assertEqual(report['root_kind'], 'top')
        self.assertNotIn('.SUBCKT top', files['top.sp'])
        self.assertNotIn('.ENDS', files['top.sp'])
        no_ports_symbol = '(edif test (edifVersion 2 0 0) (library work (cell top (view symbol (interface (symbol (figure device (rectangle (pt 0 0) (pt 20 20)))))))))'
        files, report = convert('// Library name: work\n// Cell name: top\nR0 (A 0) resistor r=1k\n', marker_drawing(), ORDERS, PACKAGE, 'top', dialect='spectre', root_symbol=no_ports_symbol)
        self.assertEqual(report['root_kind'], 'top')
        self.assertIsNone(report['symbol_file'])
        self.assertNotIn('cell-symbol.rnl', files)
        self.assertNotIn('subckt top', files['top.scs'])
        self.assertNotIn('# @SET-SYMBOL', files['top.scs'])

    def test_veriloga_interface_and_includes_stay_outside_wrapper(self):
        model = 'module leaf(B, A); input [1:0] A; output [1:0] B; electrical [1:0] A, B; analog begin end endmodule'
        self.assertEqual(interfaces(model), {'leaf': ['B<1>', 'B<0>', 'A<1>', 'A<0>']})
        for bad in ('module x(A); input [N:0] A; endmodule', 'module x(A, A); input A; endmodule'):
            with self.assertRaises(ExportError):
                interfaces(bad)
        native = '// Library name: work\n// Cell name: top\nI0 (A B) leaf\nahdl_include "/model/leaf.va"\n'
        files, _ = convert(native, drawing(), {}, PACKAGE, 'top', dialect='spectre', behavior_models={'leaf': ['A', 'B']})
        self.assertIn('#"I0" @SET-SYMBOL %symbol_0', files['top.scs'])
        self.assertTrue(files['top.scs'].endswith('ends top\nahdl_include "/model/leaf.va"\n'))


    def test_ground_and_wire_end_no_connect(self):
        text = '// Library name: work\n// Cell name: top\nR0 (A 0) resistor r=1k\n'
        binary = Path(os.environ.get('RNL_BIN', str(ROOT/'target'/('rnl.exe' if os.name == 'nt' else 'rnl'))))
        for stub in (False, True):
            files, report = convert(text, marker_drawing(stub), ORDERS, PACKAGE, 'top', dialect='spectre')
            output = files['top.scs']
            self.assertEqual(report['statistics']['instances'], 1)
            self.assertIn('$GROUND @ATTACH &"0"', output)
            if stub:
                self.assertIn('$FIGURE @ATTACH %marker_2 => @SET-SYMBOL $NO_CONNECT_SYMBOL', output)
                self.assertIn('%marker_2 = (4, 0)', output)
                self.assertIn('{#"R0":1 %marker_2}', output)
            else:
                self.assertIn('$NO_CONNECT @ATTACH #"R0":1', output)
            if binary.is_file():
                with tempfile.TemporaryDirectory() as directory:
                    for name, content in files.items():
                        (Path(directory)/name).write_text(content, encoding='utf-8')
                    result = subprocess.run([str(binary), 'check', str(Path(directory)/'top.scs')], text=True, encoding='utf-8', capture_output=True)
                    self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
                    self.assertTrue(clean_check(result.stdout+result.stderr), result.stdout+result.stderr)

    def test_combined_preserves_every_electrical_line(self):
        files, report = self.export()
        plain = re.sub(r'\* @RNL\b.*?^\* }\n', '', files['top.sp'], flags=re.S | re.M)
        self.assertEqual(plain, netlist())
        self.assertEqual(report['statistics']['instances'], 2)
        self.assertIn('# @SET-SYMBOL %block_symbol', files['top.sp'])
        self.assertIn('#"RR0" @SET-SYMBOL $RESISTOR', files['top.sp'])

    def test_separate_and_current_level_keep_child_symbol(self):
        split, _ = self.export(split=True)
        self.assertNotIn('.include ', split['top.sp'])
        self.assertNotIn('#"XI0" @SET-SYMBOL', split['top.sp'])
        self.assertIn('# @SET-SYMBOL %block_symbol', split['leaf.sp'])
        current, _ = self.export(hierarchy=False)
        self.assertNotIn('.SUBCKT leaf', current['top.sp'])
        self.assertIn('#"XI0" @SET-SYMBOL %symbol_0', current['top.sp'])
        self.assertIn('XI0 A B / leaf', current['top.sp'])

    def test_array_keeps_members_and_one_bundle(self):
        files, report = self.export(array=True)
        self.assertEqual(report['statistics']['bundles'], 1)
        self.assertIn('{#"RR0<1>" #"RR0<0>"} @BUNDLE', files['top.sp'])
        self.assertIn('@PIN(terminal=["A<1>", "A<0>"])', files['top.sp'])
        self.assertIn('{&"A<1>" &"A<0>"} @WIRE', files['top.sp'])

    def test_terminal_order_is_required(self):
        with self.assertRaisesRegex(ExportError, 'termOrder'):
            convert(netlist(), drawing(), {}, PACKAGE, 'top')

    def test_bus_tap_reuses_the_bus_point(self):
        tap = '(net (rename tap "A<0>") (joined (portRef (member A 1))) (figure wire (path (pointList (pt -20 0) (pt -20 -30)))))'
        source = drawing(True).replace('(page one (instance I0', '(page one '+tap+' (instance I0')
        files, _ = convert(netlist(True), source, ORDERS, PACKAGE, 'top')
        text = files['top.sp'].split('.SUBCKT top', 1)[1]
        point = re.search(r'(%p\d+) = \(-2, 0\)', text)[1]
        wires = [line for line in text.splitlines() if '@WIRE' in line and point in line]
        self.assertEqual(len(wires), 2)
        self.assertIn('$JUNCTION @ATTACH '+point, text)

    def test_bad_connectivity_fails_instead_of_rewiring(self):
        with self.assertRaisesRegex(ExportError, 'connectivity disagrees'):
            convert(netlist().replace('RR0 A B', 'RR0 B A'), drawing(), ORDERS, PACKAGE, 'top')





    def test_bad_scale_and_array_width_are_rejected(self):
        for scale in (0, -1, float('nan')):
            with self.assertRaises(ExportError):
                self.export(scale=scale)

    def test_job_does_not_replace_existing_output(self):
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory)/'export'
            out.mkdir()
            sentinel = out/'precious.txt'
            sentinel.write_text('original')
            fields = ['rnl-export-job-v1', '', 'work', 'top', 'schematic', '', directory, str(out), 't', 'nil', '10', '']
            request = Path(directory)/'request.sx'
            request.write_text('('+' '.join(json.dumps(s) for s in fields)+')')
            with self.assertRaisesRegex(ExportError, 'already exists'):
                run_job(request)
            self.assertEqual(sentinel.read_text(), 'original')

    def test_all_modes_with_rnl_checker(self):
        binary = Path(os.environ.get('RNL_BIN', str(ROOT/'target'/('rnl.exe' if os.name == 'nt' else 'rnl'))))
        if not binary.is_file():
            self.skipTest('Build rnl-cli or set RNL_BIN to check generated RNL')
        for array in (False, True):
            for options in ({}, {'split': True}, {'hierarchy': False}, {'dialect': 'spectre'}, {'dialect': 'spectre', 'split': True}, {'dialect': 'spectre', 'hierarchy': False}):
                with self.subTest(array=array, options=options), tempfile.TemporaryDirectory() as directory:
                    files, _ = self.export(array=array, **options)
                    for name, content in files.items():
                        (Path(directory)/name).write_text(content, encoding='utf-8')
                    entry = 'top.scs' if options.get('dialect') == 'spectre' else 'top.sp'
                    result = subprocess.run([str(binary), 'check', str(Path(directory)/entry)], text=True, encoding='utf-8', capture_output=True)
                    self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
                    self.assertTrue(clean_check(result.stdout+result.stderr), result.stdout+result.stderr)


if __name__ == '__main__':
    unittest.main()
