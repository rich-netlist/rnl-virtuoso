"""Optional reusable symbol packages and primary symbol-only library cells."""
import copy
import json
import os
import unittest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent/'reference'))

from fixtures import drawing, netlist, spectre_netlist, marker_drawing
from go_backend import call
from rnl_export import edif, resources, ExportError
import test_library


@unittest.skipIf(os.environ.get('RNL_EXPORT_BACKEND') == 'python', 'Optional symbol packages use the Go backend')
class SymbolPackageTests(unittest.TestCase):
    def root_symbol(self):
        tree = edif.parse(drawing())
        lib = next(n for n in tree if edif.tag(n) == 'library' and n[1] == 'work')
        cell = copy.deepcopy(next(n for n in lib if edif.tag(n) == 'cell' and n[1] == 'leaf'))
        cell[1] = 'top'
        view = next(n for n in cell if edif.tag(n) == 'view')
        view[:] = [n for n in view if edif.tag(n) != 'contents']
        def emit(n):
            return '('+' '.join(emit(x) for x in n)+')' if isinstance(n, list) else json.dumps(n)
        return emit(['edif', 'test', ['edifVersion', '2', '0', '0'], ['library', 'work', cell]])

    def test_cell_package_is_opt_in_without_changing_any_circuit_file(self):
        for dialect in ('cdl', 'spectre'):
            for split in (False, True):
                for hierarchy in (False, True):
                    with self.subTest(dialect=dialect, split=split, hierarchy=hierarchy):
                        request = dict(op='convert', top='top', dialect=dialect,
                            netlist=spectre_netlist() if dialect == 'spectre' else netlist(), drawing=drawing(),
                            orders=[dict(library='analogLib', cell='res', ports=['PLUS', 'MINUS'])],
                            package=resources.read('eda-symbol.rnl'), split=split, hierarchy=hierarchy,
                            root_symbol=self.root_symbol())
                        default = call(**request)
                        off = call(**request, export_symbols=False)
                        on = call(**request, export_symbols=True)
                        self.assertEqual(default['files'], off['files'])
                        self.assertIsNone(default['report']['symbol_file'])
                        self.assertFalse(default['report']['export_symbols'])
                        self.assertNotIn('symbols.rnl', off['files'])
                        self.assertNotIn('cell-symbol.rnl', on['files'])
                        self.assertEqual(set(on['files'])-set(off['files']), {'symbols.rnl'})
                        self.assertEqual({k:v for k,v in on['files'].items() if k!='symbols.rnl'}, off['files'])
                        self.assertEqual(on['report']['symbol_file'], 'symbols.rnl')
                        self.assertTrue(on['report']['export_symbols'])
                        self.assertIn('@PACKAGE(name="virtuoso-library/work"', on['files']['symbols.rnl'])
                        self.assertIn('$"work/top" = @DEFINE-SYMBOL', on['files']['symbols.rnl'])
                        self.assertEqual(on['files']['symbols.rnl'].count('$"work/top" ='), 1)
                        self.assertIn('# @SET-SYMBOL %block_symbol', off['files'][off['report']['entry']])

    def test_portless_top_symbol_is_available_only_in_optional_collection(self):
        request = dict(op='convert', top='top', dialect='spectre',
            netlist='// Library name: work\n// Cell name: top\nR0 (A 0) resistor r=1k\n',
            drawing=marker_drawing(), orders=[dict(library='analogLib',cell='res',ports=['PLUS','MINUS'])],
            package=resources.read('eda-symbol.rnl'),
            root_symbol='(edif test (edifVersion 2 0 0) (library work (cell top (view symbol (interface (symbol (figure device (rectangle (pt 0 0) (pt 20 20)))))))))')
        off = call(**request)
        on = call(**request, export_symbols=True)
        self.assertEqual(off['files']['top.scs'], on['files']['top.scs'])
        self.assertNotIn('symbols.rnl', off['files'])
        self.assertNotIn('cell-symbol.rnl', on['files'])
        self.assertIn('$"work/top" = @DEFINE-SYMBOL', on['files']['symbols.rnl'])

    def test_symbol_only_cells_always_get_their_own_package(self):
        helper = test_library.LibraryTests(methodName='runTest')
        for dialect in ('spectre','cdl'):
            captures = helper.fixtures(dialect)
            results = [call(op='library', library='work', dialect=dialect, captures=captures, export_symbols=v) for v in (False,True)]
            off, on = results
            self.assertNotIn('symbols.rnl', off['files'])
            self.assertNotIn('cell-symbol.rnl', on['files'])
            self.assertEqual({k:v for k,v in on['files'].items() if k!='symbols.rnl'}, off['files'])
            standalone = off['files']['OnlySymbol.rnl']
            self.assertIn('@PACKAGE(name="virtuoso/work/OnlySymbol"', standalone)
            self.assertEqual(standalone.count('$"work/OnlySymbol" ='),1)
            self.assertEqual(off['report']['symbol_only_files'],[dict(library='work',cell='OnlySymbol',file='OnlySymbol.rnl',package='virtuoso/work/OnlySymbol')])
            self.assertIsNone(off['report']['symbol_file'])
            only = call(op='library',library='work',dialect=dialect,captures=[captures[-1]])
            self.assertIn('OnlySymbol.rnl',only['files'])
            self.assertNotIn('symbols.rnl',only['files'])
            self.assertEqual(only['report']['definitions'],[])
            if os.environ.get('RNL_BIN'):
                helper.check(off['files'],[e['file'] for e in off['report']['entries']]+['OnlySymbol.rnl'])
                helper.check(on['files'],['symbols.rnl','OnlySymbol.rnl'])

    def test_symbol_only_filenames_cannot_collide_or_escape(self):
        definition='$"work/only" = @DEFINE-SYMBOL { (0,0) @PIN(terminal="A") }'
        def capture(cell):
            return dict(Library='work',Cell=cell,Dialect='spectre',Symbols=[dict(Library='work',Cell=cell,Definition=definition.replace('work/only','work/'+cell))])
        foreign = capture('only')
        foreign['Library'] = foreign['Symbols'][0]['Library'] = 'symbols.rnl'
        for parts in ([capture('only'),capture('ONLY')],[capture('symbols')],[capture('../escape')],[foreign]):
            with self.subTest(parts=parts), self.assertRaisesRegex(ExportError,'Conflicting|Reserved|Unsafe|conflicts'):
                call(op='library',library='work',dialect='spectre',captures=parts)
