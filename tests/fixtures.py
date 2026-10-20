"""Small, original fixtures. No Cadence library drawings are redistributed."""


def drawing(array=False):
    def named(base):
        return '(array (rename %s "%s<1:0>") 2)' % (base, base) if array else base

    ports = ' '.join('(port %s)' % named(p) for p in ('A', 'B'))
    implementation = '''(portImplementation A (connectLocation (figure pin (dot (pt 0 0)))))
        (portImplementation B (connectLocation (figure pin (dot (pt 60 0)))))'''
    refs = lambda p: '(portList (portRef %s (instanceRef (member R0 0))) (portRef %s (instanceRef (member R0 1))))' % (p, p) if array else '(portRef %s (instanceRef R0))' % p
    leaf_nets = ' '.join('''(net %s (joined (portRef %s) %s)
        (figure wire (path (pointList (pt %d 0) (pt %d 0)))))''' %
        (named(p), p, refs('PLUS' if p == 'A' else 'MINUS'), -40 if p == 'A' else 60, 0 if p == 'A' else 100) for p in ('A', 'B'))
    top_nets = ' '.join('''(net %s (joined (portRef %s) (portRef %s (instanceRef I0)))
        (figure wire (path (pointList (pt %d 0) (pt %d 0)))))''' %
        (named(p), p, p, -40 if p == 'A' else 60, 0 if p == 'A' else 100) for p in ('A', 'B'))
    return '''(edif test (edifVersion 2 0 0)
      (library analogLib
        (cell res (view symbol (interface (port PLUS) (port MINUS)
          (symbol (figure device (path (pointList (pt 0 0) (pt 0 -60))))
            (portImplementation PLUS (connectLocation (figure pin (dot (pt 0 0)))))
            (portImplementation MINUS (connectLocation (figure pin (dot (pt 0 -60))))))))))
      (library work
        (cell leaf (view schematic (interface %s
          (symbol (boundingBox (rectangle (pt 0 -20) (pt 60 20)))
            (figure device (rectangle (pt 0 -20) (pt 60 20))) %s))
          (contents (page one (instance %s (viewRef symbol (cellRef res (libraryRef analogLib)))
              (transform (orientation R90))) %s %s))))
        (cell top (view schematic (interface %s)
          (contents (page one (instance I0 (viewRef schematic (cellRef leaf)) (transform)) %s %s))))))''' % (ports, implementation, named('R0'), implementation, leaf_nets, ports, implementation, top_nets)


def netlist(array=False):
    ports = 'A<1> A<0> B<1> B<0>' if array else 'A B'
    resistors = 'RR0<1> A<1> B<1> 1k $[RP]\nRR0<0> A<0> B<0> 1k $[RP]\n' if array else 'RR0 A B 1k $[RP]\n'
    return ('* Fixture\n* Library Name: work\n* Cell Name: leaf\n.SUBCKT leaf '+ports+'\n'+resistors+
            '.ENDS\n* Library Name: work\n* Cell Name: top\n.SUBCKT top '+ports+'\nXI0 '+ports+' / leaf\n.ENDS\n')


def spectre_netlist(array=False):
    ports = r'A\<1\> A\<0\> B\<1\> B\<0\>' if array else 'A B'
    resistors = ('R0\\<1\\> (A\\<1\\> B\\<1\\>) resistor r=1k\n'
                 'R0\\<0\\> (A\\<0\\> B\\<0\\>) resistor r=1k\n') if array else 'R0 (A B) resistor r=(Rbase + 1k)\n'
    return ('// Fixture\nsimulator lang=spectre\nglobal 0\n'
            '// Library name: work\n// Cell name: leaf\nsubckt leaf '+ports+'\n'+resistors+
            'ends leaf\n// Library name: work\n// Cell name: top\nI0 ('+ports+') leaf\n')


def marker_drawing(stub=True):
    """Original resistor/marker shapes; no vendor geometry."""
    def pin(name, y=0):
        return '(portImplementation %s (connectLocation (figure pin (dot (pt 0 %d)))))' % (name, y)

    resistor = '(cell res (view symbol (interface (port PLUS) (port MINUS) (symbol %s %s))))' % (pin('PLUS'), pin('MINUS', -60))
    marker = lambda name: '(cell %s (view symbol (interface (port P) (symbol %s))))' % (name, pin('P'))
    x = 40 if stub else 0
    content = ('(instance R0 (viewRef symbol (cellRef res (libraryRef analogLib))) (transform))'
               '(instance G0 (viewRef symbol (cellRef gnd (libraryRef analogLib))) (transform (origin (pt 0 -60))))'
               '(instance NC0 (viewRef symbol (cellRef noConn (libraryRef basic))) (transform (origin (pt %d 0))))'
               '(net A (joined (portRef PLUS (instanceRef R0)) (portRef P (instanceRef NC0)))'
               ' (figure wire (path (pointList (pt 0 0) (pt %d 0)))))'
               '(net GND (joined (portRef MINUS (instanceRef R0)) (portRef P (instanceRef G0))))') % (x, x)
    return ('(edif markers (edifVersion 2 0 0)'
            '(library analogLib %s %s)(library basic %s)'
            '(library work (cell top (view schematic (interface) (contents %s)))))') % (resistor, marker('gnd'), marker('noConn'), content)


def primitive_drawing(cell, pins, orientation):
    """Original pin-only master and wire stubs, in units ten times RNL's."""
    interface = ' '.join('(port P%d)' % i for i in range(1, len(pins)+1))
    pin_impl = ' '.join('(portImplementation P%d (connectLocation (figure pin (dot (pt %g %g)))))' % (i, x*10, y*10) for i, (x, y) in enumerate(pins, 1))
    # Transform stub coordinates independently using integer fixture matrices.
    matrices = {'R0': (1, 0, 0, 1), 'R90': (0, -1, 1, 0), 'R180': (-1, 0, 0, -1), 'R270': (0, 1, -1, 0),
                'MX': (1, 0, 0, -1), 'MY': (-1, 0, 0, 1), 'MXR90': (0, 1, 1, 0), 'MYR90': (0, -1, -1, 0)}
    a, b, c, d = matrices[orientation]
    nets = []
    world_pins = []
    for i, (x, y) in enumerate(pins, 1):
        px, py = a*x+b*y+20, c*x+d*y-10
        world_pins.append((px, py))
        nets.append('(net N%d (joined (portRef P%d (instanceRef X0))) (figure wire (path (pointList (pt %g %g) (pt %g %g)))))' % (i, i, px*10, py*10, px*10+10, py*10+10))
    text = ('(edif example (edifVersion 2 0 0) '
            '(library analogLib (cell %s (view symbol (interface %s (symbol %s))))) '
            '(library work (cell top (view schematic (interface) (contents '
            '(instance X0 (viewRef symbol (cellRef %s (libraryRef analogLib))) (transform (orientation %s) (origin (pt 200 -100)))) %s)))))') % (cell, interface, pin_impl, cell, orientation, ' '.join(nets))
    return text, world_pins
