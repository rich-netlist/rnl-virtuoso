"""Original per-cell file-layout and shared-descendant regressions."""
import copy
import os
import unittest

import test_library
from go_backend import call
from rnl_export import ExportError
from rnl_export import resources
from fixtures import drawing, netlist, spectre_netlist


def diamond_captures(dialect='spectre'):
    spectre = dialect == 'spectre'
    def definition(library, cell, name, body, root=False):
        text = ('subckt '+name+' p n\n'+body+'ends '+name+'\n' if spectre else
                '.SUBCKT '+name+' p n\n'+body+'.ENDS '+name+'\n')
        return dict(Library=library, Cell=cell, Name=name, Body=text, Root=root)
    leaf = definition('foreign', 'shared', 'Shared_native',
                      'R0 (p n) resistor r=1k\n' if spectre else 'R0 p n 1k\n')
    a = definition('work', 'A', 'A_native',
                   'X0 (p n) Shared_native\n' if spectre else 'X0 p n / Shared_native\n', True)
    b = definition('work', 'B', 'B_native',
                   'X0 (p n) Shared_native\n' if spectre else 'X0 p n / Shared_native\n', True)
    prefix, extension, carrier = ('', 'scs', '//') if spectre else ('.', 'sp', '*')
    def capture(cell, definitions, devices=''):
        template = carrier+' original fixture\n'
        if spectre:
            template += 'simulator lang=spectre\nglobal 0\n'
        template += ''.join(prefix+'include "netlists/'+d['Name']+'.'+extension+'"\n' for d in definitions)+devices
        return dict(Library='work', Cell=cell, Dialect=dialect, Entry=cell+'.'+extension,
                    Template=template, Definitions=definitions)
    devices = ('V0 (vin 0) vsource dc=1\nXA (vin out) A_native\nXB (vin out) B_native\n'
               'Rload (out 0) resistor r=1k\ndcOp dc\nsave out\n' if spectre else
               'V0 vin 0 1\nXA vin out / A_native\nXB vin out / B_native\nRload out 0 1k\n')
    children = [copy.deepcopy(a), copy.deepcopy(b), copy.deepcopy(leaf)]
    for child in children:
        child['Root'] = False
    return [capture('A', [a, leaf]), capture('B', [b, leaf]), capture('bench', children, devices)]


@unittest.skipIf(os.environ.get('RNL_EXPORT_BACKEND') == 'python', 'Per-cell layout is a Go backend extension')
class PerCellTests(unittest.TestCase):
    def check(self, files, entries):
        test_library.LibraryTests(methodName='runTest').check(files, entries)

    def test_diamond_has_one_foreign_file_and_one_flat_include_per_definition(self):
        for dialect, extension, prefix in [('spectre', 'scs', ''), ('cdl', 'sp', '.')]:
            captures = diamond_captures(dialect)
            result = call(op='library', library='work', dialect=dialect, split=True, captures=captures)
            files = result['files']
            expected = {'A.'+extension, 'B.'+extension, 'foreign/shared.'+extension, 'bench.'+extension}
            self.assertEqual({n for n in files if n.endswith('.'+extension)}, expected)
            self.assertFalse(any(n.startswith('netlists/') for n in files))
            for filename in expected-{'bench.'+extension}:
                self.assertNotIn(prefix+'include ', files[filename])
                self.assertIn('subckt' if dialect == 'spectre' else '.SUBCKT', files[filename])
            for filename in sorted(expected-{'bench.'+extension}):
                self.assertEqual(files['bench.'+extension].count(prefix+'include "'+filename+'"\n'), 1)
            self.assertIn('path="../eda-symbol.rnl"', files['foreign/shared.'+extension])
            self.check(files, sorted(expected))

    def test_cell_native_variants_share_one_cell_file_and_keep_their_dependencies(self):
        captures = diamond_captures()
        other = dict(Library='another', Cell='extra', Name='Extra_native', Root=False,
                     Body='subckt Extra_native p n\nR0 (p n) resistor r=2k\nends Extra_native\n')
        variant = copy.deepcopy(captures[1]['Definitions'][1])
        variant['Name'] = 'Shared_variant'
        variant['Body'] = 'subckt Shared_variant p n\nX0 (p n) Extra_native\nends Shared_variant\n'
        captures[1]['Definitions'][0]['Body'] = captures[1]['Definitions'][0]['Body'].replace('Shared_native', 'Shared_variant')
        captures[1]['Definitions'][1] = variant
        captures[1]['Definitions'].append(other)
        # The bench's original capture reaches Shared_native through A only;
        # B supplied another native variant stored in that same physical file.
        captures[2]['Definitions'] = [copy.deepcopy(captures[0]['Definitions'][0]), copy.deepcopy(captures[0]['Definitions'][1])]
        captures[2]['Definitions'][0]['Root'] = False
        captures[2]['Template'] = captures[2]['Template'].replace('include "netlists/B_native.scs"\n', '').replace('XB (vin out) B_native\n', '')
        result = call(op='library', library='work', dialect='spectre', split=True, captures=captures)
        files = result['files']
        self.assertIn('subckt Shared_native p n', files['foreign/shared.scs'])
        self.assertIn('subckt Shared_variant p n', files['foreign/shared.scs'])
        self.assertIn('include "another/extra.scs"', files['bench.scs'])
        self.assertNotIn('include "B.scs"', files['bench.scs'])
        self.check(files, ['bench.scs', 'A.scs', 'B.scs', 'foreign/shared.scs', 'another/extra.scs'])

    def test_actual_cell_names_are_validated_and_native_model_includes_belong_to_top(self):
        captures = diamond_captures()
        captures[0]['Template'] += 'ahdl_include "/models/behavior.va"\n'
        captures[2]['Template'] += 'ahdl_include "/models/behavior.va"\n'
        result = call(op='library', library='work', dialect='spectre', split=True, captures=captures)
        self.assertNotIn('ahdl_include', result['files']['A.scs'])
        self.assertIn('ahdl_include "/models/behavior.va"', result['files']['bench.scs'])
        for cell in ('../escape', 'A'):
            bad = diamond_captures()
            bad[0]['Definitions'][1]['Cell'] = cell
            if cell == 'A':
                bad[0]['Definitions'][1]['Library'] = 'work'
                # Actual different source cells are rejected on portable filename collisions.
                bad[0]['Definitions'][1]['Cell'] = 'a'
            with self.subTest(cell=cell), self.assertRaisesRegex(ExportError, 'Unsafe|Conflicting'):
                call(op='library', library='work', dialect='spectre', split=True, captures=bad)

    def test_single_cell_export_uses_the_same_paths_and_respects_external_scope(self):
        helper = test_library.LibraryTests(methodName='runTest')
        for dialect, extension in [('spectre', 'scs'), ('cdl', 'sp')]:
            for portless in (False, True):
                native, source = helper.external_inputs(dialect, portless=portless)
                for external in (False, True):
                    with self.subTest(dialect=dialect, portless=portless, external=external):
                        result = call(op='convert', netlist=native, drawing=source, top='top',
                            orders=[dict(library='analogLib', cell='res', ports=['PLUS', 'MINUS'])],
                            package=resources.read('eda-symbol.rnl'), hierarchy=True, split=True,
                            library_scope='' if external else 'work', dialect=dialect)
                        files = result['files']
                        self.assertEqual('other/leaf.'+extension in files, external)
                        includes = 'include "other/leaf.'+extension+'"'
                        self.assertEqual(includes in files['top.'+extension], portless and external)
                        self.assertIn('#"I0" @SET-SYMBOL' if dialect == 'spectre' else '#"XI0" @SET-SYMBOL', files['top.'+extension])
                        self.check(files, [name for name in files if name.endswith('.'+extension)])

    def test_separate_bus_and_array_cells_keep_standalone_child_symbols(self):
        for dialect, native, extension in [('spectre', spectre_netlist(True), 'scs'), ('cdl', netlist(True), 'sp')]:
            result = call(op='convert', netlist=native, drawing=drawing(True), top='top',
                orders=[dict(library='analogLib', cell='res', ports=['PLUS', 'MINUS'])],
                package=resources.read('eda-symbol.rnl'), hierarchy=True, split=True, dialect=dialect)
            files = result['files']
            self.assertIn('I0" @SET-SYMBOL %symbol_', files['top.'+extension])
            self.assertIn('@PIN(terminal=[1, 2])', files['top.'+extension])
            self.assertIn('@BUNDLE', files['leaf.'+extension])
            self.assertNotIn('include ', files['top.'+extension])
            self.check(files, ['top.'+extension, 'leaf.'+extension])
