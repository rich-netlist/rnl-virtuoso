"""Original shared-library fixtures; no foundry or Cadence library data."""
import copy
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent/'reference'))
from fixtures import drawing, spectre_netlist, netlist, marker_drawing
from go_backend import call, cell_json
from rnl_export import ExportError, edif, resources


def with_dependencies(files, entry):
    """Load ported fragments through a caller without changing exported includes."""
    ported = re.compile(r'^\s*(?:subckt|\.subckt)\s+', re.I | re.M)
    if not ported.search(files[entry]):
        return files, entry
    extension = Path(entry).suffix
    spectre = extension == '.scs'
    caller = '__test_caller'+extension
    text = '// Fixture caller\nsimulator lang=spectre\n' if spectre else '* Fixture caller\n'
    for name in sorted(files):
        if name.endswith(extension) and ported.search(files[name]):
            text += ('include ' if spectre else '.include ')+json.dumps(name)+'\n'
    return dict(files, **{caller: text}), caller


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
        return [top, leaf, bench, self.external_fixture(dialect, None, 'leaf', 'bench_top', portless=True), only_symbol]

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
                context, actual = with_dependencies(files, entry)
                if actual != entry:
                    (root/actual).write_text(context[actual], encoding='utf-8')
                result = subprocess.run([binary, 'check', str(root/actual)], capture_output=True, text=True, encoding='utf-8')
                self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
                self.assertIn('0 errors, 0 warnings', result.stdout+result.stderr)

    def external_inputs(self, dialect, library='other', native_name='Leaf_native_2', top='top', portless=False):
        tree = edif.parse(drawing())
        work = next(n for n in tree if edif.tag(n) == 'library' and n[1] == 'work')
        leaf = next(n for n in work if edif.tag(n) == 'cell' and n[1] == 'leaf')
        if library:
            work.remove(leaf)
            tree.append(['library', library, leaf])
        def update(n):
            if isinstance(n, list):
                if library and edif.tag(n) == 'cellref' and n[1] == 'leaf':
                    n.append(['libraryRef', library])
                if edif.tag(n) == 'cell' and n[1] == 'top':
                    n[1] = top
                for child in n:
                    update(child)
        update(tree)
        if portless:
            root = next(n for n in work if edif.tag(n) == 'cell' and n[1] == top)
            view = next(n for n in root if edif.tag(n) == 'view')
            interface = next(n for n in view if edif.tag(n) == 'interface')
            interface[:] = ['interface']
            def remove_ports(n):
                if isinstance(n, list):
                    n[:] = [child for child in n if not (isinstance(child, list) and
                        (edif.tag(child) == 'portimplementation' or edif.tag(child) == 'portref' and len(child) == 2))]
                    for child in n:
                        remove_ports(child)
            remove_ports(view)
        def emit(n):
            return '('+' '.join(emit(x) for x in n)+')' if isinstance(n, list) else json.dumps(n)
        if dialect == 'spectre':
            native = spectre_netlist().replace('// Library name: work\n// Cell name: leaf',
                                                '// Library name: '+(library or 'work')+'\n// Cell name: leaf')
            native = native.replace('subckt leaf ', 'subckt '+native_name+' ').replace('ends leaf', 'ends '+native_name)
            native = native.replace(') leaf\n', ') '+native_name+'\n').replace('// Cell name: top', '// Cell name: '+top)
        else:
            native = netlist().replace('* Library Name: work\n* Cell Name: leaf',
                                       '* Library Name: '+(library or 'work')+'\n* Cell Name: leaf')
            native = native.replace('.SUBCKT leaf ', '.SUBCKT '+native_name+' ').replace('/ leaf\n', '/ '+native_name+'\n')
            native = native.replace('* Cell Name: top', '* Cell Name: '+top).replace('.SUBCKT top ', '.SUBCKT '+top+' ')
            if portless:
                native = native.replace('.SUBCKT '+top+' A B\n', '.SUBCKT '+top+'\n')
        return native, emit(tree)

    def external_fixture(self, dialect, library='other', native_name='Leaf_native_2', top='top', portless=False):
        native, source = self.external_inputs(dialect, library, native_name, top, portless)
        return self.capture(native, source, top, dialect)

    def test_external_definitions_are_grouped_by_source_library_and_deduplicated(self):
        for dialect in ('spectre', 'cdl'):
            captures = [self.external_fixture(dialect), self.external_fixture(dialect, top='second'),
                        self.external_fixture(dialect, 'ThirdLib', 'Leaf_native_3', 'third')]+self.fixtures(dialect)[2:]
            extension = 'scs' if dialect == 'spectre' else 'sp'
            prefix = '' if dialect == 'spectre' else '.'
            for split in (False, True):
                with self.subTest(dialect=dialect, split=split):
                    result = call(op='library', library='work', dialect=dialect, split=split, captures=captures, export_symbols=True)
                    files, report = result['files'], result['report']
                    self.assertEqual(len(report['definitions']), 6)
                    self.assertEqual(len(report['entries']), 5)
                    self.assertEqual({n.split('/')[0] for n in files if '/' in n}, {'other', 'ThirdLib'})
                    for definition in report['definitions']:
                        directory = '' if definition['library'] == 'work' else definition['library']+'/'
                        expected = directory+definition['cell']+'.'+extension
                        self.assertEqual(definition['file'], expected)
                        self.assertIn(expected, files)
                    foreign = 'other/leaf.'+extension
                    body = next(d['Body'] for d in captures[0]['Definitions'] if d['Library'] == 'other')
                    self.assertEqual(files[foreign].count(body), 1)
                    self.assertNotIn('netlists/Leaf_native_2.'+extension, files)
                    for entry in report['entries']:
                        text = files[entry['file']]
                        required = ['leaf.'+extension] if entry['cell'] == 'bench_top' else []
                        for path in required:
                            self.assertEqual(text.count(prefix+'include '+json.dumps(path)+'\n'), 1)
                        if entry['cell'] in ('top', 'second', 'third'):
                            self.assertNotIn(prefix+'include ', text)
                    self.check(files, [e['file'] for e in report['entries']]+['symbols.rnl'])

    def test_unsafe_or_colliding_library_directories_are_rejected(self):
        capture = self.external_fixture('spectre')
        for directory in ('..', '.', '../escape', 'bad\\dir', 'trailing.', 'trailing ', 'symbols.rnl', 'top.scs'):
            bad = copy.deepcopy(capture)
            next(d for d in bad['Definitions'] if d['Library'] == 'other')['Library'] = directory
            with self.subTest(directory=directory), self.assertRaisesRegex(ExportError, 'Unsafe|conflicts|Conflicting'):
                call(op='library', library='work', dialect='spectre', split=True, captures=[bad])
        other = self.external_fixture('spectre', 'OTHER', 'Leaf_native_3', 'second')
        with self.assertRaisesRegex(ExportError, 'Conflicting library directory'):
            call(op='library', library='work', dialect='spectre', split=True, captures=[capture, other])

    def test_shared_definitions_symbols_and_portless_entries_in_both_formats_and_modes(self):
        for dialect in ('spectre', 'cdl'):
            captures = self.fixtures(dialect)
            for split in (False, True):
                with self.subTest(dialect=dialect, split=split):
                    result = call(op='library', library='work', dialect=dialect, split=split, captures=captures, export_symbols=True)
                    files, report = result['files'], result['report']
                    self.assertEqual(len(report['definitions']), 2)
                    self.assertEqual(len(report['entries']), 4)
                    self.assertFalse([n for n in files if n.startswith('netlists/')])
                    self.assertEqual(files['symbols.rnl'].count('$"work/OnlySymbol" ='), 1)
                    extension = 'scs' if dialect == 'spectre' else 'sp'
                    host = '#"I0"' if dialect == 'spectre' else '#"XI0"'
                    self.assertNotIn(host+' @SET-SYMBOL', files['top.'+extension])
                    self.assertIn('# @SET-SYMBOL %block_symbol', files['leaf.'+extension])
                    self.assertNotIn(('subckt ' if dialect == 'spectre' else '.SUBCKT ')+'bench', files['bench.'+extension])
                    self.check(files, [e['file'] for e in report['entries']]+['symbols.rnl'])
            only = call(op='library', library='work', dialect=dialect, split=False, captures=[captures[-1]], export_symbols=True)
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
            self.assertIn('subckt leaf A B\n', result['files']['leaf.scs'])
            self.assertIn(child['Body'], result['files']['leaf.scs'])
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
        self.assertIn('#"I0" @SET-SYMBOL', result['files']['top.scs'])
        self.check(result['files'], ['top.scs'])
