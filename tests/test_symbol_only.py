"""Library cells with a symbol and no schematic: empty subcircuits or one package."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent/'support'))

from fixtures import drawing, netlist, spectre_netlist, clean_check
from go_backend import call, cell_json, convert
from rnl_export import edif, resources, ExportError
import test_library

EXTENSION = {'spectre': 'scs', 'cdl': 'sp'}


def symbol_only(library, cell, ports, **fields):
    """The library part a symbol job writes for a cell drawn like analogLib/res."""
    res = edif.read(drawing())['analogLib', 'res']
    terminals = {key: '"'+name+'"' for key, name in zip(res.ports, ports)}
    definition = call(op='symbol', cell=cell_json(res), symbol_id='$"%s/%s"' % (library, cell), terminals=terminals)['text']
    return dict(Library=library, Cell=cell, Dialect=fields.pop('Dialect', 'spectre'),
                Symbols=[dict(Library=library, Cell=cell, Definition=definition)], **fields)


class SymbolOnlyTests(unittest.TestCase):
    helper = test_library.LibraryTests(methodName='runTest')

    def check(self, files, entries):
        binary = os.environ.get('RNL_BIN')
        if not binary or not Path(binary).is_file():
            self.skipTest('Set RNL_BIN to validate the output')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name, text in files.items():
                (root/name).parent.mkdir(parents=True, exist_ok=True)
                (root/name).write_text(text, encoding='utf-8')
            for entry in entries:
                result = subprocess.run([binary, 'check', str(root/entry)], capture_output=True, text=True, encoding='utf-8')
                self.assertEqual(result.returncode, 0, entry+'\n'+result.stdout+result.stderr)
                self.assertTrue(clean_check(result.stdout+result.stderr), entry+'\n'+result.stdout+result.stderr)

    def calling(self, dialect, captures, callee='OnlySymbol'):
        """The portless bench of the fixtures, with an instance of callee."""
        captures = copy.deepcopy(captures)
        bench = next(c for c in captures if c.get('Cell') == 'bench')
        bench['Template'] += 'I9 (A 0) %s\n' % callee if dialect == 'spectre' else 'XI9 A 0 %s\n' % callee
        bench['Calls'] = sorted(set(bench.get('Calls') or []) | {callee})
        return captures

    def test_cell_exports_write_no_separate_symbol_package(self):
        for dialect in ('cdl', 'spectre'):
            for split in (False, True):
                with self.subTest(dialect=dialect, split=split):
                    files, report = convert(spectre_netlist() if dialect == 'spectre' else netlist(), drawing(),
                                            {('analogLib', 'res'): ['PLUS', 'MINUS']}, resources.read('eda-symbol.rnl'),
                                            'top', split=split, dialect=dialect)
                    self.assertFalse({'symbols.rnl', 'cell-symbol.rnl'} & set(files))
                    self.assertFalse({'export_symbols', 'symbol_file'} & set(report))

    def test_by_default_each_becomes_an_empty_subcircuit_with_its_symbol(self):
        for dialect in ('spectre', 'cdl'):
            with self.subTest(dialect=dialect):
                captures = self.helper.fixtures(dialect)
                result = call(op='library', library='work', dialect=dialect, captures=captures)
                files, report = result['files'], result['report']
                name = 'OnlySymbol.'+EXTENSION[dialect]
                text = files[name]
                if dialect == 'spectre':
                    self.assertIn('\nsubckt OnlySymbol A B\n', text)
                    self.assertTrue(text.endswith('ends OnlySymbol\n'))
                else:
                    self.assertIn('\n.SUBCKT OnlySymbol A B\n', text)
                    self.assertTrue(text.endswith('.ENDS\n'))
                self.assertEqual(text.count('%block_symbol = @DEFINE-SYMBOL'), 1)
                self.assertIn('# @SET-SYMBOL %block_symbol', text)
                self.assertNotIn('OnlySymbol.rnl', files)
                self.assertEqual(report['symbol_only_cells'], 'subcircuit')
                self.assertEqual(report['symbol_only_files'], [dict(library='work', cell='OnlySymbol', file=name, subcircuit='OnlySymbol')])
                self.assertIsNone(report['symbol_package'])
                # Nothing calls it, so no file includes it.
                self.assertFalse([n for n, t in files.items() if 'include "'+name+'"' in t])
                self.check(files, [name])
                only = call(op='library', library='work', dialect=dialect, captures=[captures[-1]])
                self.assertEqual(set(only['files']), {name, 'eda-style.rnl', 'eda-symbol.rnl'})

    def test_portless_tops_include_the_empty_subcircuits_they_call(self):
        for dialect in ('spectre', 'cdl'):
            with self.subTest(dialect=dialect):
                prefix, name = '' if dialect == 'spectre' else '.', 'OnlySymbol.'+EXTENSION[dialect]
                captures = self.calling(dialect, self.helper.fixtures(dialect))
                files = call(op='library', library='work', dialect=dialect, captures=captures)['files']
                bench = files['bench.'+EXTENSION[dialect]]
                self.assertEqual(bench.count(prefix+'include '+json.dumps(name)+'\n'), 1)
                for other in ('top', 'leaf'):
                    self.assertNotIn(name, files[other+'.'+EXTENSION[dialect]])
                # A cell defined by a view the netlister stops at gets no file.
                stopped = copy.deepcopy(captures)
                stopped[-1]['StopView'] = 'veriloga' if dialect == 'spectre' else 'auCdl'
                result = call(op='library', library='work', dialect=dialect, captures=stopped)
                self.assertNotIn(name, result['files'])
                self.assertNotIn(name, result['files']['bench.'+EXTENSION[dialect]])
                self.assertEqual(result['report']['symbol_only_files'],
                                 [dict(library='work', cell='OnlySymbol', file=None, defined_by_view=stopped[-1]['StopView'])])

    def test_cdf_terminal_order_orders_the_subcircuit_ports(self):
        for order, header in ((['MINUS', 'PLUS'], 'subckt only MINUS PLUS'), (['MINUS', 'X'], 'subckt only PLUS MINUS')):
            with self.subTest(order=order):
                part = symbol_only('work', 'only', ['PLUS', 'MINUS'], TermOrder=order)
                files = call(op='library', library='work', dialect='spectre', captures=[part])['files']
                self.assertIn('\n'+header+'\n', files['only.scs'])
        part = symbol_only('work', '2stage', ['in+', 'out'])
        text = call(op='library', library='work', dialect='spectre', captures=[part])['files']['2stage.scs']
        self.assertIn('\nsubckt \\2stage in\\+ out\n', text)
        self.assertTrue(text.endswith('ends \\2stage\n'))

    def test_package_mode_writes_one_package_and_its_mapping(self):
        parts = [symbol_only('myLib', 'va_amp', ['inp', 'out'], StopView='veriloga', TermOrder=['out', 'inp'], Model='va_amp'),
                 symbol_only('myLib', '2nd', ['p', 'n']),
                 symbol_only('myLib', 'pad', ['p', 'n'], Model='iPar("model")')]
        result = call(op='library', library='myLib', dialect='spectre', captures=parts, symbol_only_cells='package')
        files, report = result['files'], result['report']
        self.assertEqual(set(files), {'mylib.rnl', 'mylib.virtuoso.json'})
        text = files['mylib.rnl']
        self.assertIn('@PACKAGE(name="mylib", version="0.1")', text)
        self.assertIn('@VERSION("0.4")', text)
        for symbol in ('$MYLIB_VA_AMP', '$MYLIB_2ND', '$MYLIB_PAD'):
            self.assertEqual(text.count(symbol+' = @DEFINE-SYMBOL'), 1)
        # A fixed model name gets terminal names and a rule; an expression does not.
        self.assertIn('@DEVICE(model="va_amp", terminals=["out", "inp"])', text)
        self.assertIn('# @SELECT-INSTANCE(model="va_amp", recursive=true) => @SET-SYMBOL $MYLIB_VA_AMP', text)
        self.assertEqual(text.count('@DEVICE('), 1)
        mapping = json.loads(files['mylib.virtuoso.json'])
        self.assertEqual(mapping['package'], dict(name='mylib', version='0.1'))
        self.assertEqual(mapping['cells'][0], dict(library='myLib', cell='2nd', symbol='MYLIB_2ND', pins=[['p', 'p'], ['n', 'n']]))
        self.assertEqual(report['symbol_only_cells'], 'package')
        self.assertEqual(report['symbol_only_files'], [])
        self.assertEqual(report['symbol_package']['file'], 'mylib.rnl')
        self.assertEqual([s['symbol'] for s in report['symbol_package']['symbols']], ['MYLIB_2ND', 'MYLIB_PAD', 'MYLIB_VA_AMP'])
        self.check(files, ['mylib.rnl'])
        named = call(op='library', library='myLib', dialect='spectre', captures=parts, symbol_only_cells='package', symbol_package='pdk-mylib')
        self.assertEqual(set(named['files']), {'pdk-mylib.rnl', 'pdk-mylib.virtuoso.json'})
        for mode, name in (('package', 'My Lib'), ('package', 'eda-style'), ('files', '')):
            with self.subTest(mode=mode, name=name), self.assertRaisesRegex(ExportError, 'symbol package name|subcircuit or package'):
                call(op='library', library='myLib', dialect='spectre', captures=parts, symbol_only_cells=mode, symbol_package=name)

    def test_package_mode_beside_converted_cells(self):
        for dialect in ('spectre', 'cdl'):
            with self.subTest(dialect=dialect):
                captures = self.calling(dialect, self.helper.fixtures(dialect))
                result = call(op='library', library='work', dialect=dialect, captures=captures, symbol_only_cells='package')
                files = result['files']
                self.assertIn('work.rnl', files)
                self.assertNotIn('OnlySymbol.'+EXTENSION[dialect], files)
                self.assertNotIn('OnlySymbol.', files['bench.'+EXTENSION[dialect]])
                self.assertIn('$WORK_ONLYSYMBOL = @DEFINE-SYMBOL', files['work.rnl'])


if __name__ == '__main__':
    unittest.main()
