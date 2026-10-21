import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from fixtures import drawing, primitive_drawing, spectre_netlist
import test_symbols
from test_symbols import BINARY, PACKAGE
from go_backend import convert


@unittest.skipIf(os.environ.get('RNL_EXPORT_BACKEND') == 'python', 'Annotation fixes use the Go backend')
class AnnotationTests(unittest.TestCase):
    def export(self, source=None, **options):
        if source is None:
            source, _ = primitive_drawing('res', [(0, 3), (0, -3)], 'R0')
        return convert('// Library name: work\n// Cell name: top\nX0 (N1 N2) resistor r=1k\n',
                       source, {('analogLib', 'res'): ['P1', 'P2']}, PACKAGE, 'top', dialect='spectre', **options)[0]

    def problem(self, files, title='top'):
        if not BINARY.is_file():
            self.skipTest('Build rnl-cli or set RNL_BIN')
        with tempfile.TemporaryDirectory() as directory:
            for name, text in files.items():
                (Path(directory)/name).write_text(text, encoding='utf-8')
            result = subprocess.run([str(BINARY), 'problem', str(Path(directory)/'top.scs')],
                                    capture_output=True, text=True, encoding='utf-8')
            self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
            data = json.loads(result.stdout)
            return (next((level for level in data['levels'] if level['title'] == title), None)
                    or next(level for level in data['levels'] if level['title'] == title+'.scs'))

    def test_wire_labels_keep_placement_and_attach_to_nearest_deduplicated_path(self):
        source, _ = primitive_drawing('res', [(0, 3), (0, -3)], 'R0')
        display = lambda x: '(display (figureGroupOverride wire (textHeight 15)) (justify UPPERRIGHT) (orientation R90) (origin (pt %d -60)))' % x
        source = source.replace('(net N1 ', '(net (name N1 '+display(205)+display(405)+') ')
        # A long segment is closest at its middle, not at one of its endpoints.
        source = source.replace('(pt 200 -70) (pt 210 -60)', '(pt 200 -70) (pt 600 -70)')
        source = source.replace('(net N2 ', '(net (name N1 '+display(405)+') (joined) (figure wire (path (pointList (pt 600 -70) (pt 200 -70))))) (net N2 ')
        files = self.export(source)
        text = files['top.scs']
        owners = [line.split(' @TEXT')[0].strip('/ ;') for line in text.splitlines() if '@TEXT("N1"' in line]
        self.assertEqual(len(owners), 3)
        self.assertEqual(len(set(owners)), 1)
        self.assertTrue(owners[0].startswith('%wire_'))
        self.assertEqual(text.count('@WIRE('), 2)  # The reversed path is shared.
        level = test_symbols.SymbolTests().dump(files)
        labels = [t for t in level['texts'] if t['anchor'] in ([20.5, -6], [40.5, -6])]
        self.assertEqual(len(labels), 3)
        for label in labels:
            self.assertEqual((label['halign'], label['valign'], label['rotate']), ('end', 'top', 90))
            self.assertEqual(label['style']['size'], 1.5)
        problem = self.problem(files)
        attached = [o['text']['attach'] for o in problem['objects'] if o.get('text', {}).get('lines') == ['N1']]
        self.assertEqual(len(attached), 3)
        self.assertTrue(all('wire' in owner for owner in attached), attached)

    def test_labels_choose_separate_paths_on_the_same_net(self):
        source, _ = primitive_drawing('res', [(0, 3), (0, -3)], 'R0')
        source = source.replace('(net N1 ', '(net (name N1 (display wire (origin (pt 205 -60))) (display wire (origin (pt 405 -60)))) ')
        source = source.replace('(pt 200 -70) (pt 210 -60)))))', '(pt 200 -70) (pt 210 -60)))) (figure wire (path (pointList (pt 400 -70) (pt 410 -60)))))')
        files = self.export(source)
        owners = [line.split(' @TEXT')[0] for line in files['top.scs'].splitlines() if '@TEXT("N1"' in line]
        self.assertEqual(len(set(owners)), 2)
        test_symbols.SymbolTests().dump(files)

    def test_bus_range_label_is_one_wire_label(self):
        source = drawing(True).replace('(rename A "A<1:0>")', '(name (rename A "A<1:0>") (display wire (origin (pt -30 0))))')
        files, _ = convert(spectre_netlist(True), source, {('analogLib', 'res'): ['PLUS', 'MINUS']}, PACKAGE, 'top', dialect='spectre')
        test_symbols.SymbolTests().dump(files)
        for title in ('top', 'leaf'):
            labels = [o['text'] for o in self.problem(files, title)['objects'] if o.get('text', {}).get('lines') == ['A<1:0>'] and 'wire' in o['text']['attach']]
            self.assertEqual(len(labels), 1)
            self.assertIn('wire', labels[0]['attach'])

    def test_label_without_drawn_path_is_attached_to_net(self):
        source, _ = primitive_drawing('res', [(0, 3), (0, -3)], 'R0')
        source = source.replace('(net N1 ', '(net (name N1 (display wire (origin (pt 200 -60)))) ')
        source = source.replace('(figure wire (path (pointList (pt 200 -70) (pt 210 -60))))', '')
        files = self.export(source)
        labels = [o['text'] for o in self.problem(files)['objects'] if o.get('text', {}).get('lines') == ['N1']]
        self.assertEqual(len(labels), 1)
        self.assertEqual(labels[0]['attach'], {'net': 'N1'})
        test_symbols.SymbolTests().dump(files)

    def test_edif_note_figures_and_text_remain_annotations(self):
        source, _ = primitive_drawing('res', [(0, 3), (0, -3)], 'R0')
        notes = '''(figure text (path (pointList (pt -20 10) (pt -10 20))))
          (commentGraphics (figure text (rectangle (pt -30 -40) (pt 50 60)))
            (figure text (polygon (pointList (pt 0 0) (pt 10 20) (pt 20 0))))
            (figure text (circle (pt 30 0) (pt 50 0)))
            (figure text (openShape (curve (arc (pt 0 10) (pt 10 0) (pt 0 -10)))))
            (annotate (stringDisplay "Note" (display text (origin (pt -20 30))))))'''
        source = source.replace('(contents ', '(contents '+notes, 1)
        files = self.export(source)
        level = test_symbols.SymbolTests().dump(files)
        self.assertEqual(len(level['wires']), 2)
        self.assertEqual(len([s for s in level['shapes'] if s['layer'] == 'middle']), 5)
        self.assertIn([[-2, 1], [-1, 2]], [s['points'] for s in level['shapes']])
        label = next(t for t in level['texts'] if t['anchor'] == [-2, 3])
        self.assertEqual(label['layer'], 'middle')
        problem = self.problem(files)
        note = next(o['text'] for o in problem['objects'] if o.get('text', {}).get('lines') == ['Note'])
        point = next(o for o in problem['objects'] if o['id'] == note['attach']['object'])
        self.assertEqual(point['kind'], 'point')

    def test_database_note_sidecar_preserves_each_cells_scale_and_scope(self):
        notes = '''(("work" "top" 160
          (("line" ((-1 0) (1 0)) nil) ("rect" ((-2 -1) (2 1)) nil)
           ("polygon" ((0 0) (1 1) (2 0)) nil) ("ellipse" ((0 0) (4 2)) nil)
           ("path" ((3 0) (3 2) (4 2)) nil) ("arc" ((0 0) (2 2)) (0 3.141592653589793))
           ("arc" ((0 0) (4 2)) (0 3.141592653589793))))
          ("work" "not_exported" 100 (("rect" ((0 0) (1 1)) nil))))'''
        files = self.export(annotations=notes)
        level = test_symbols.SymbolTests().dump(files)
        self.assertEqual(len(level['wires']), 2)
        self.assertEqual(len([s for s in level['shapes'] if s['layer'] == 'middle']), 7)
        self.assertIn([[-16, 0], [16, 0]], [s['points'] for s in level['shapes']])
        self.assertIn([[-32, -16], [32, 16]], [s['points'] for s in level['shapes']])
        for forbidden in ('linecolor=', 'linewidth=', 'fillcolor=', '@SET-STYLE'):
            self.assertNotIn(forbidden, files['top.scs'])

    def test_hierarchy_notes_stay_in_their_own_cells(self):
        notes = '''(("work" "top" 160 (("rect" ((-1 -1) (1 1)) nil)))
                    ("work" "leaf" 200 (("rect" ((-1 -1) (1 1)) nil))))'''
        files, _ = convert(spectre_netlist(), drawing(), {('analogLib', 'res'): ['PLUS', 'MINUS']},
                           PACKAGE, 'top', dialect='spectre', annotations=notes)
        top = test_symbols.SymbolTests().dump(files, 'top')
        leaf = test_symbols.SymbolTests().dump(files, 'leaf')
        self.assertIn([[-16, -16], [16, 16]], [s['points'] for s in top['shapes']])
        self.assertNotIn([[-20, -20], [20, 20]], [s['points'] for s in top['shapes']])
        self.assertIn([[-20, -20], [20, 20]], [s['points'] for s in leaf['shapes']])
        self.assertNotIn([[-16, -16], [16, 16]], [s['points'] for s in leaf['shapes']])

    def test_dashed_note_shapes_preserve_strokes_and_legacy_snapshots(self):
        figures = [
            '("line" ((0 0) (1 1)) nil)', '("path" ((1 0) (2 1)) nil)',
            '("polygon" ((2 0) (3 1) (4 0)) nil)', '("rect" ((0 2) (1 3)) nil)',
            '("ellipse" ((2 2) (4 3)) nil)', '("arc" ((0 0) (2 2)) (0 3.141592653589793))',
            '("arc" ((0 0) (4 2)) (0 3.141592653589793))']
        for style in ('dashed', 'solid', None):
            with self.subTest(style=style):
                shapes = ' '.join(f[:-1]+' "'+style+'")' if style else f for f in figures)
                files = self.export(annotations='(("work" "top" 160 ('+shapes+')))')
                level = test_symbols.SymbolTests().dump(files)
                notes = [s for s in level['shapes'] if s['layer'] == 'middle']
                self.assertEqual(len(notes), len(figures))
                self.assertTrue(all(s['style'].get('linestyle', 'solid') == (style or 'solid') for s in notes))
