"""Original shared-library fixtures; no foundry or Cadence library data."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent/'reference'))
from fixtures import drawing, spectre_netlist, netlist, marker_drawing
from go_backend import call, cell_json
from rnl_export import ExportError, edif, resources


@unittest.skipIf(os.environ.get('RNL_EXPORT_BACKEND') == 'python', 'Shared-library export is a Go backend extension')
class LibraryTests(unittest.TestCase):
    def capture(self, native, source, top, dialect='spectre', **options):
        return call(op='capture', library='work', netlist=native, drawing=source, top=top,
                    orders=[dict(library='analogLib', cell='res', ports=['PLUS', 'MINUS'])],
                    package=resources.read('eda-symbol.rnl'), hierarchy=True, split=True, dialect=dialect, **options)['capture']

    def fixtures(self, dialect):
        native = spectre_netlist() if dialect == 'spectre' else netlist()
        top = self.capture(native, drawing(), 'top', dialect)
        if dialect == 'spectre':
            leaf_native = '// Library name: work\n// Cell name: leaf\nR0 (A B) resistor r=(Rbase + 1k)\n'
            bench_native = '// Library name: work\n// Cell name: bench\nR0 (A 0) resistor r=1k\n'
        else:
            leaf_native = native.split('* Library Name: work\n* Cell Name: top')[0]
            bench_native = '* Bench\n* Library Name: work\n* Cell Name: bench\n.SUBCKT bench\nR0 A 0 1k\n.ENDS\n'
        leaf = self.capture(leaf_native, drawing(), 'leaf', dialect)
        bench_tree = edif.parse(marker_drawing().replace('(cell top ', '(cell bench '))
        canonical_res = next(n for lib in edif.parse(drawing()) if edif.tag(lib) == 'library' and lib[1] == 'analogLib' for n in lib if edif.tag(n) == 'cell')
        analog = next(lib for lib in bench_tree if edif.tag(lib) == 'library' and lib[1] == 'analogLib')
        analog[2] = canonical_res
        def emit(n):
            return '('+' '.join(emit(x) for x in n)+')' if isinstance(n, list) else json.dumps(n)
        bench = self.capture(bench_native, emit(bench_tree), 'bench', dialect)
        cell = edif.read(drawing())['work', 'leaf']
        definition = call(op='symbol', cell=cell_json(cell), symbol_id='$"work/OnlySymbol"', terminals={'a': '"A"', 'b': '"B"'})['text']
        only_symbol = dict(Library='work', Cell='OnlySymbol', Dialect=dialect,
                           Symbols=[dict(Library='work', Cell='OnlySymbol', Definition=definition)])
        return [top, leaf, bench, only_symbol]

    def check(self, files, entries):
        binary = os.environ.get('RNL_BIN')
        if not binary or not Path(binary).is_file():
            self.skipTest('Set RNL_BIN to validate the shared library')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name, text in files.items():
                (root/name).parent.mkdir(parents=True, exist_ok=True)
                (root/name).write_text(text, encoding='utf-8')
            for entry in entries:
                result = subprocess.run([binary, 'check', str(root/entry)], capture_output=True, text=True, encoding='utf-8')
                self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
                self.assertIn('0 errors, 0 warnings', result.stdout+result.stderr)

    def test_shared_definitions_symbols_and_portless_entries_in_both_formats_and_modes(self):
        for dialect in ('spectre', 'cdl'):
            captures = self.fixtures(dialect)
            for split in (False, True):
                with self.subTest(dialect=dialect, split=split):
                    result = call(op='library', library='work', dialect=dialect, split=split, captures=captures)
                    files, report = result['files'], result['report']
                    self.assertEqual(len(report['definitions']), 2)
                    self.assertEqual(len(report['entries']), 3)
                    self.assertEqual(len([n for n in files if n.startswith('netlists/')]), 2 if split else 1)
                    self.assertEqual(files['symbols.rnl'].count('$"work/OnlySymbol" ='), 1)
                    extension = 'scs' if dialect == 'spectre' else 'sp'
                    self.assertNotIn(('subckt ' if dialect == 'spectre' else '.SUBCKT ')+'bench', files['bench.'+extension])
                    self.check(files, [e['file'] for e in report['entries']]+['symbols.rnl'])
            only = call(op='library', library='work', dialect=dialect, split=False, captures=[captures[-1]])
            self.assertEqual(only['report']['entries'], [])
            self.assertEqual(only['report']['definitions'], [])
            self.assertFalse([name for name in only['files'] if name.startswith('netlists/')])
            self.check(only['files'], ['symbols.rnl'])

    def test_conflicting_native_names_and_electrical_definitions_are_not_swallowed(self):
        captures = self.fixtures('spectre')[:2]
        for change in ('library', 'body'):
            bad = copy.deepcopy(captures)
            if change == 'library':
                bad[1]['Definitions'][0]['Library'] = 'other'
            else:
                bad[1]['Definitions'][0]['Body'] = bad[1]['Definitions'][0]['Body'].replace('Rbase + 1k', 'Rbase + 2k')
            with self.subTest(change=change), self.assertRaisesRegex(ExportError, 'Conflicting native subcircuit'):
                call(op='library', library='work', dialect='spectre', split=True, captures=bad)

    def test_spectre_root_wrapper_does_not_change_native_child_port_order(self):
        captures = self.fixtures('spectre')[:2]
        child = next(d for d in captures[0]['Definitions'] if d['Cell'] == 'leaf')
        root = next(d for d in captures[1]['Definitions'] if d['Cell'] == 'leaf')
        root['Body'] = root['Body'].replace('subckt leaf A B', 'subckt leaf B A')
        root['Body'] = root['Body'].replace('resistor r=', 'resistor  \\\n    r=')
        for parts in (captures, list(reversed(captures))):
            result = call(op='library', library='work', dialect='spectre', split=True, captures=parts)
            self.assertIn('subckt leaf A B\n', result['files']['netlists/leaf.scs'])
            self.assertIn(child['Body'], result['files']['netlists/leaf.scs'])
            self.check(result['files'], ['top.scs', 'leaf.scs'])
        # Two native definitions with different positional interfaces are unsafe.
        captures[1]['Definitions'][0]['Root'] = False
        with self.assertRaisesRegex(ExportError, 'Conflicting native subcircuit'):
            call(op='library', library='work', dialect='spectre', split=True, captures=captures)

    def test_external_scope_keeps_the_symbol_but_omits_the_definition(self):
        tree = edif.parse(drawing())
        work = next(n for n in tree if edif.tag(n) == 'library' and n[1] == 'work')
        leaf = next(n for n in work if edif.tag(n) == 'cell' and n[1] == 'leaf')
        work.remove(leaf)
        tree.append(['library', 'other', leaf])
        def update(n):
            if isinstance(n, list):
                if edif.tag(n) == 'cellref' and n[1] == 'leaf':
                    n.append(['libraryRef', 'other'])
                for child in n:
                    update(child)
        update(tree)
        def emit(n):
            return '('+' '.join(emit(x) for x in n)+')' if isinstance(n, list) else json.dumps(n)
        native = spectre_netlist().replace('// Library name: work\n// Cell name: leaf', '// Library name: other\n// Cell name: leaf')
        capture = self.capture(native, emit(tree), 'top', library_scope='work')
        self.assertEqual([d['Name'] for d in capture['Definitions']], ['top'])
        self.assertIn('other/leaf', [s['Library']+'/'+s['Cell'] for s in capture['Symbols']])
        result = call(op='library', library='work', dialect='spectre', split=True, captures=[capture])
        self.assertNotIn('netlists/leaf.scs', result['files'])
        self.assertIn('#"I0" @SET-SYMBOL', result['files']['netlists/top.scs'])
        self.check(result['files'], ['top.scs'])
