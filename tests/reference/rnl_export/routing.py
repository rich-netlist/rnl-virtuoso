"""Join EDIF route geometry using terminal references, independent of pin artwork."""
from . import ExportError
from .symbols import pt, ref, text_statement


def _point(value):
    # Match the precision written by pt(), including transformed source pins.
    return tuple(round(v, 6) for v in value)


def _on_segment(p, a, b):
    dx, dy = b[0]-a[0], b[1]-a[1]
    return abs((p[0]-a[0])*dy-(p[1]-a[1])*dx) < 1e-7 and min(a[0], b[0])-1e-7 <= p[0] <= max(a[0], b[0])+1e-7 and min(a[1], b[1])-1e-7 <= p[1] <= max(a[1], b[1])+1e-7


def _on_path(point, paths):
    return any(point in path or any(_on_segment(point, a, b) for a, b in zip(path, path[1:])) for path in paths)


def draw_routes(cell, endpoints, netmap, scale):
    statements, wire_keys, joins = [], set(), {}
    paths = [[[_point(tuple(v/scale for v in p)) for p in path] for path in route.paths] for route in cell.routes]
    bits = [frozenset(route.bits) for route in cell.routes]
    net_order = {n: i for i, n in enumerate(dict.fromkeys(n for route in cell.routes for n in route.bits))}
    terminals = {}
    for nets, source, terminal in endpoints:
        terminals.setdefault(_point(source), []).append((frozenset(nets), terminal))
    candidates = set(terminals) | {p for route in paths for path in route for p in path}
    points = [{p for p in candidates if _on_path(p, route)} for route in paths]
    coverage = {}
    for nets, route in zip(bits, points):
        for point in route:
            coverage.setdefault(point, []).append(nets)
    references, next_point = {}, 0

    def reference(point, nets):
        nonlocal next_point
        # Form components before choosing a reference: a bus and its scalar
        # taps must choose the same point regardless of which route is read first.
        component = set(nets)
        others = coverage.get(point, []) + [n for n, _ in terminals.get(point, [])]
        while True:
            expanded = component.union(*(n for n in others if component.intersection(n)))
            if expanded == component:
                break
            component = expanded
        key = (point, frozenset(component))
        if key not in references:
            eligible = [r for n, r in terminals.get(point, []) if n.issuperset(component)]
            if eligible:
                # This is a pin, not a saved coordinate of the old symbol's pin.
                references[key] = eligible[0]
            else:
                references[key] = '%%p%d' % next_point
                next_point += 1
                statements.append('%s = %s' % (references[key], pt(point)))
        return references[key]

    def wire(nets, vertices):
        vertices = [v for i, v in enumerate(vertices) if i == 0 or v != vertices[i-1]]
        if len(vertices) < 2:
            return
        actual = tuple(dict.fromkeys(netmap[n] for n in sorted(nets, key=lambda n: (net_order.get(n, len(net_order)), n))))
        key = (tuple(sorted(actual)), min(tuple(vertices), tuple(reversed(vertices))))
        if key in wire_keys:
            return
        wire_keys.add(key)
        names = [ref('&', n) for n in actual]
        host = names[0] if len(names) == 1 else '{'+' '.join(names)+'}'
        statements.append('%s @WIRE(mode="straight") {%s}' % (host, ' '.join(vertices)))
        for a, b in zip(vertices, vertices[1:]):
            joins.setdefault(a, set()).add(b)
            joins.setdefault(b, set()).add(a)

    # Preserve direct pin-to-pin contacts too, even when EDIF has no wire
    # figure for the contact. There is no connector to an old pin coordinate.
    for point, attached in terminals.items():
        for nets, terminal in attached:
            if any(n.intersection(nets) and r != terminal for n, r in attached):
                wire(nets, [terminal, reference(point, nets)])

    for index, route in enumerate(cell.routes):
        if any(n not in netmap for n in route.bits):
            raise ExportError('No native netlist correspondence for EDIF net %s/%s' % (cell.name, route.name))
        for point in sorted(points[index]):
            for nets, terminal in terminals.get(point, []):
                if nets == bits[index]:
                    wire(nets, [terminal, reference(point, nets)])
        for path in paths[index]:
            expanded = []
            for a, b in zip(path, path[1:]):
                if a == b:
                    continue
                segment = sorted((p for p in points[index] if _on_segment(p, a, b)), key=lambda p: (p[0]-a[0])**2+(p[1]-a[1])**2)
                expanded.extend(segment[:-1])
            if path:
                expanded.append(path[-1])
            wire(bits[index], [reference(p, bits[index]) for p in expanded])
        for display in route.labels:
            label = netmap[route.bits[0]] if len(route.bits) == 1 else route.name
            statements.append(text_statement(label, display, scale))
    for point, neighbours in joins.items():
        if len(neighbours) >= 3:
            statements.append('$JUNCTION @ATTACH '+point)
    return statements, len(wire_keys)
