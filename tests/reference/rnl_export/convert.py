"""Join a native netlist to its EDIF drawing; add only RNL comments."""
import math
import re
from pathlib import Path

from . import ExportError
from . import cdl, edif, spectre
from . import orientation, resources
from .routing import draw_routes
from .symbols import (STANDARD, package_pins, quote, ref, pt, symbol_definition,
                      text_statement, literal)


def load_orders(text):
    if not text.strip():
        return {}
    rows = edif.parse(text, skill=True)
    return {(row[0], row[1]): row[2] if isinstance(row[2], list) else [] for row in rows}


def _port(cell, spelling, dialect="cdl", names=None):
    normalize = (lambda s: s) if dialect == "spectre" else str.lower
    names = names or {}
    matches = [key for key, value in cell.ports.items() if normalize(names.get(value, value)) == normalize(spelling)]
    if not matches and dialect == "cdl":
        matches = [key for key, value in cell.ports.items() if cdl.mapped_name(value) == cdl.mapped_name(spelling)]
    if len(matches) != 1:
        raise ExportError("Cannot identify native netlist port %s in %s/%s" % (spelling, cell.library, cell.name))
    return matches[0]


def _header(carrier="*"):
    return ('* @RNL(type="header") {\n'
            '*   @VERSION("0.3");\n'
            '*   @USE-PACKAGE(name="eda-symbol", version="0.1", path="eda-symbol.rnl");\n'
            '*   @USE-PACKAGE(name="eda-style", version="0.1", path="eda-style.rnl")\n'
            '* }\n').replace('*', carrier)


def _record(statements, carrier="*"):
    body = ';\n'.join(statements)
    return carrier+' @RNL(author="rnl-virtuoso") {\n' + ''.join(carrier+'   '+line+'\n' for line in body.splitlines()) + carrier+' }\n'


def _nets(bits, mapping):
    refs = list(dict.fromkeys(ref('&', mapping[b]) for b in bits))
    return refs[0] if len(refs) == 1 else '{'+' '.join(refs)+'}'


def interface_order(cell, metadata):
    """Expand CDF bus groups against the EDIF interface, never alphabetize bits."""
    order = []
    for name in metadata:
        group = [k for k, label in cell.port_labels.items() if label == name]
        if len(group) == 1:
            order.extend(cell.port_groups[group[0]])
        else:
            matches = [k for k, label in cell.ports.items() if label == name]
            if len(matches) != 1:
                return None
            order.extend(matches)
    return order if len(order) == len(set(order)) and set(order) == set(cell.ports) else None


def _netlist_filename(name, extension):
    if not name or any(c in '/\\\":<>|?*' or ord(c) < 32 or ord(c) == 127 for c in name):
        raise ExportError('Unsafe netlist filename for '+name)
    return name+'.'+extension


def convert(netlist, drawing, orders, package, top, hierarchy=True, split=False, scale=10.0, dialect="cdl", name_maps=None, root_symbol=None, behavior_models=None, style_package=None, orientation_rules=None):
    if scale <= 0 or not math.isfinite(scale):
        raise ExportError("EDIF units per RNL unit must be positive and finite")
    if "@RNL" in netlist:
        raise ExportError("Input already contains RNL records; export a fresh native netlist")
    if dialect not in {"cdl", "spectre"}:
        raise ExportError("Unsupported native netlist format: " + dialect)
    name_maps = name_maps or {}
    behavior_models = behavior_models or {}
    style_package = resources.read("eda-style.rnl") if style_package is None else style_package
    orientation_rules = orientation.load_rules() if orientation_rules is None else orientation_rules
    normalize = (lambda s: s) if dialect == "spectre" else str.lower
    carrier = "//" if dialect == "spectre" else "*"
    extension = "scs" if dialect == "spectre" else "sp"
    lines, blocks = spectre.read(netlist, top) if dialect == "spectre" else cdl.read(netlist)
    cells = edif.read(drawing)
    standards = package_pins(package)
    topkey = normalize(top)
    if topkey not in blocks:
        matches = [key for key, block in blocks.items() if block.cell == top]
        if len(matches) != 1:
            raise ExportError("Top cell %s is not uniquely identified in native netlist" % top)
        topkey = matches[0]
    source, block_for_cell = {}, {}
    for key, block in blocks.items():
        matches = [c for c in cells.values() if c.name == (block.cell or block.name) and (not block.library or c.library == block.library)]
        if len(matches) != 1:
            raise ExportError("Cannot uniquely match native circuit %s to EDIF" % block.name)
        source[key] = matches[0]
        block_for_cell[(matches[0].library, matches[0].name)] = block

    root, root_cell = blocks[topkey], source[topkey]
    root_is_subcircuit = bool(root_cell.ports)
    exported_symbol = root_cell if root_cell.symbol else None
    if root_symbol is not None:
        exported_symbol = edif.read(root_symbol).get((root_cell.library, root_cell.name))
        if not exported_symbol or not exported_symbol.symbol:
            raise ExportError("Native EDIF omitted the current cell's symbol view")
        if set(exported_symbol.ports.values()) != set(root_cell.ports.values()):
            raise ExportError("Current schematic and symbol terminal interfaces differ")
    if dialect == "spectre" and root_is_subcircuit:
        order = interface_order(root_cell, orders.get((root_cell.library, root_cell.name), []))
        order = order if order is not None else list(root_cell.ports)
        names = name_maps.get(root.name, {}).get("net", {})
        root.ports = [names.get(root_cell.ports[p], root_cell.ports[p]) for p in order]
        if len(set(root.ports)) != len(root.ports):
            raise ExportError("Native root port names are not unique")

    def port_id(cell, spelling):
        block = block_for_cell.get((cell.library, cell.name))
        names = name_maps.get(block.name, {}).get("net", {}) if block else {}
        return _port(cell, spelling, dialect, names)

    selected = set(blocks) if hierarchy else {topkey}
    records, notes = {}, []
    stats = {"subcircuits": len(selected), "instances": 0, "bundles": 0, "wires": 0, "custom_symbols": 0, "standard_symbols": 0}
    for key in blocks:
        if key not in selected:
            continue
        block, cell = blocks[key], source[key]
        statements = []
        native_names = name_maps.get(block.name, {})
        bindings = spectre.match_instances(block, cell, native_names) if dialect == "spectre" else cdl.match_instances(block, cell)
        connections = {(iid.lower() if iid else None, port): net.name
                       for net in cell.nets for iid, port in net.terminals}
        netmap = {}

        def bind_net(original, actual):
            expected = native_names.get("net", {}).get(original)
            if expected is not None and expected != actual:
                raise ExportError("Native name map and EDIF connectivity disagree on %s/%s" % (block.name, original))
            if original in netmap and normalize(netmap[original]) != normalize(actual):
                raise ExportError("Netlist/EDIF connectivity disagrees on %s/%s: %s vs %s" % (block.name, original, netmap[original], actual))
            netmap[original] = actual

        for actual in block.ports:
            port = port_id(cell, actual)
            original = connections.get((None, port))
            if original is not None:
                bind_net(original, actual)
        # Routing uses source topology and symbolic terminals, never package pin coordinates.
        endpoints = []
        local_symbols = {}
        symbol_cell = exported_symbol if key == topkey else cell if cell.symbol else None
        if symbol_cell and (key != topkey or root_is_subcircuit):
            mapping = {port_id(symbol_cell, p): p for p in block.ports}
            statements.append(symbol_definition(symbol_cell, "%block_symbol", mapping, scale, notes))
            statements.append("# @SET-SYMBOL %block_symbol")
            stats["custom_symbols"] += 1
        done_bundles = set()
        for ins in cell.instances:
            if ins.bundle and ins.bundle in done_bundles:
                continue
            master = cells[ins.master]
            words = bindings.get(ins.id.lower())
            if words is None:
                continue  # supply markers are handled after net names are established
            child = block_for_cell.get(ins.master)
            if child:
                order = [port_id(master, p) for p in child.ports]
            else:
                metadata = behavior_models.get(words[-1], orders.get(ins.master, []))
                if not metadata:
                    raise ExportError("Missing native simulator termOrder for %s/%s; refusing to guess positional pins" % ins.master)
                order = [port_id(master, p) for p in metadata]
            if set(order) != set(master.ports) or any(p not in master.pins for p in order):
                raise ExportError("EDIF symbol pins and native terminal interface differ for %s/%s" % ins.master)
            members = [i for i in cell.instances if i.bundle == ins.bundle] if ins.bundle else [ins]
            for member in members:
                values = bindings[member.id.lower()]
                if dialect == "spectre" and len(values) != len(order) + 2:
                    raise ExportError("Spectre instance %s node count differs from its CDF/ports" % values[0])
                if len(values) <= len(order):
                    raise ExportError("Native instance %s has fewer nodes than its CDF/ports" % values[0])
                if child:
                    tail = values[1+len(order):]
                    if tail and tail[0] == "/":
                        tail = tail[1:]
                    if not tail or normalize(tail[0]) != normalize(child.name):
                        raise ExportError("Netlist/EDIF subcircuit reference disagrees for %s/%s" % (block.name, member.name))
                for port, actual in zip(order, values[1:]):
                    original = connections.get((member.id.lower(), port))
                    if original is None:
                        raise ExportError("No EDIF connection for %s/%s:%s" % (block.name, member.name, port))
                    bind_net(original, actual)
            host = ref("#", words[0])
            if ins.bundle:
                host = '%%bundle_%d' % len(done_bundles)
                statements.append('%s = {%s} @BUNDLE' % (host, ' '.join(ref('#', bindings[m.id.lower()][0]) for m in members)))
                done_bundles.add(ins.bundle)
                stats['bundles'] += 1
            xy = tuple(v/scale for v in ins.xy)
            output_orient = ins.orient
            destination_pins = None
            standard = STANDARD.get(master.name.lower()) if master.library == "analogLib" and not child else None
            # Standard symbols are used only when their positional interface is complete.
            if standard and standard in standards:
                pin_set = standards[standard]
                aliases = {"d": "1", "g": "2", "s": "3", "b": "4", "c": "1", "e": "3", "p": "1", "n": "2", "a": "1", "k": "2"}
                numeric = {aliases.get(p, p): pos for p, pos in pin_set.items()}
                if all(str(i+1) in numeric for i in range(len(order))):
                    destination_pins = [numeric[str(i+1)] for i in range(len(order))]
            if destination_pins is not None:
                source_pins = [tuple(v/scale for v in master.pins[p]) for p in order]
                xy, output_orient = orientation.place(standard, source_pins, destination_pins, ins.orient, xy, orientation_rules)
                statements.append("%s @SET-SYMBOL $%s" % (host, standard))
                stats["standard_symbols"] += 1
            else:
                if not child or normalize(child.name) not in selected:
                    signature = (ins.master, tuple(order))
                    if signature not in local_symbols:
                        sid = "%%symbol_%d" % len(local_symbols)
                        local_symbols[signature] = sid
                        statements.append(symbol_definition(master, sid, {p: i+1 for i, p in enumerate(order)}, scale, notes))
                        stats["custom_symbols"] += 1
                    statements.append("%s @SET-SYMBOL %s" % (host, local_symbols[signature]))
            statements.append("%s @PLACE%s => @ORIENT(%s)" % (host, pt(xy), quote(output_orient)))
            if ins.bundle:
                statements.append('%s @TEXT(%s, key="name")' % (host, quote(literal(ins.bundle_name))))
            for ports in master.port_groups.values():
                port = ports[0]
                i = order.index(port)
                original = edif.transform(tuple(v/scale for v in master.pins[port]), ins.orient, tuple(v/scale for v in ins.xy))
                bits = tuple(dict.fromkeys(connections[(m.id.lower(), p)] for m in members for p in ports))
                endpoints.append((bits, original, "%s:%d" % (host, i+1)))
            stats["instances"] += len(members)
        for index, ins in enumerate(cell.instances):
            if ins.id.lower() in bindings:
                continue
            master = cells[ins.master]
            if len(master.pins) != 1:
                raise ExportError("Unsupported non-netlisted marker %s/%s" % (block.name, ins.name))
            port, loc = next(iter(master.pins.items()))
            original = connections.get((ins.id.lower(), port))
            if original not in netmap:
                raise ExportError("Cannot map the net of marker %s" % ins.name)
            at = edif.transform(tuple(v/scale for v in loc), ins.orient, tuple(v/scale for v in ins.xy))
            sid = "%%marker_%d" % index
            if master.name.lower() == "noconn":
                targets = [r for n, a, r in endpoints if original in n and a == at]
                if len(targets) == 1:
                    statements.append("$NO_CONNECT @ATTACH %s => @ORIENT(%s)" % (targets[0], quote(ins.orient)))
                else:
                    # A terminal-bound NO_CONNECT cannot have its own position.
                    # Keep a wire-end marker as a figure on the wire's point.
                    if len([r for n, _, r in endpoints if original in n]) != 1:
                        raise ExportError("No-connect marker %s does not identify exactly one device terminal" % ins.name)
                    statements.append("%s = %s" % (sid, pt(at)))
                    statements.append("$FIGURE @ATTACH %s => @SET-SYMBOL $NO_CONNECT_SYMBOL => @ORIENT(%s)" % (sid, quote(ins.orient)))
                    endpoints.append(((original,), at, sid))
                continue
            kind = "GROUND" if master.name.lower().startswith("gnd") else "SUPPLY"
            statements.append("%s = $%s @ATTACH %s => @PLACE%s => @ORIENT(%s)" % (sid, kind, ref("&", netmap[original]), pt(at), quote(ins.orient)))
            endpoints.append(((original,), at, sid+":p"))
        for index, (port, pos) in enumerate(cell.port_positions.items()):
            bits = tuple(connections.get((None, p)) for p in cell.port_groups[port])
            if any(b not in netmap for b in bits):
                continue
            at = tuple(v/scale for v in pos)
            sid = "%%port_%d" % index
            definition, args, terminal = '$IO_PORT', _nets(bits, netmap), ':p'
            if len(bits) > 1:
                # One drawn port with one bus pin; scalar roles retain every net.
                definition = '%%bus_port_type_%d' % index
                roles = ', '.join('["%s", "net"]' % ('p' if i == 0 else 'p%d' % i) for i in range(len(bits)))
                statements.append('%s = @DEFINE(roles=[%s]) {}' % (definition, roles))
                statements.append('%s_symbol = @DEFINE-SYMBOL {$IO_PORT_SYMBOL; (0, 0) @PIN(terminal=[%s])}' % (definition, ', '.join(str(i+1) for i in range(len(bits)))))
                statements.append('%s @REMOVE-FROM $PORT_FILL_SET' % _nets(bits, netmap))
                args = ' '.join(ref('&', netmap[b]) for b in bits)
                terminal = ':1'
            statements.append("%s = %s @ATTACH %s => @PLACE%s => @ORIENT(%s)" % (sid, definition, args, pt(at), quote(cell.port_orientations.get(port, "R0"))))
            if len(bits) > 1:
                statements.append('%s @SET-SYMBOL %s_symbol' % (sid, definition))
                statements.append('%s @TEXT(%s, key="name")' % (sid, quote(literal(cell.port_labels[port]))))
            endpoints.append((bits, at, sid+terminal))
        wires, wire_count = draw_routes(cell, endpoints, netmap, scale)
        statements.extend(wires)
        stats["wires"] += wire_count
        for cg in cell.annotations:
            for sd in edif.walk(cg, "stringdisplay"):
                statements.append(text_statement(edif.decoded(sd[1]), edif.first(sd, "display", []), scale))
        records[key] = _record(statements, carrier)
    rendered = {key: ''.join(lines[b.start:b.close]) + records[key] + ''.join(lines[b.close:b.end]) for key, b in blocks.items() if key in selected}
    if dialect == "spectre" and root_is_subcircuit:
        escape = lambda s: re.sub(r'^[0-9]|[^a-zA-Z0-9_]', lambda m: '\\'+m[0], s)
        rendered[topkey] = 'subckt '+escape(root.name)+' '+ ' '.join(escape(p) for p in root.ports)+'\n'+rendered[topkey]+'ends '+escape(root.name)+'\n'
    elif dialect == "cdl" and not root_is_subcircuit:
        body_start = next(end for start, end, _ in cdl.statements(lines) if start == root.start)
        rendered[topkey] = ''.join(lines[body_start:root.close])+records[topkey]
    files = {}
    entry = _netlist_filename(top, extension)
    if split and hierarchy:
        includes = []
        filenames = {entry.lower()}
        for key in (k for k in blocks if k != topkey):
            filename = _netlist_filename(blocks[key].name, extension)
            if filename.lower() in filenames:
                raise ExportError('Subcircuit filename '+filename+' conflicts with the entry file or another subcircuit')
            filenames.add(filename.lower())
            files[filename] = carrier+" RNL subcircuit " + blocks[key].name + "\n" + _header(carrier) + rendered[key]
            includes.append(('%sinclude "%s"\n' % ('' if dialect == 'spectre' else '.', filename)))
        replacement = {key: rendered[key] if key == topkey else "" for key in blocks}
    else:
        includes = []
        replacement = {key: rendered.get(key, "") for key in blocks}
    out, cursor = [], 0
    for key, block in sorted(blocks.items(), key=lambda item: item[1].start):
        out.extend(lines[cursor:block.start])
        out.append(replacement[key])
        cursor = block.end
    out.extend(lines[cursor:])
    native = ''.join(out)
    first_line, sep, rest = native.partition('\n')
    files[entry] = first_line + '\n' + _header(carrier) + ''.join(includes) + rest
    files["eda-symbol.rnl"] = package
    files["eda-style.rnl"] = style_package
    if exported_symbol:
        public_name = root_cell.library+'/'+root_cell.name
        mapping = {port_id(exported_symbol, p): p for p in root.ports}
        definition = symbol_definition(exported_symbol, ref('$', public_name), mapping, scale, notes)
        files['cell-symbol.rnl'] = '@RNL(type="header") { @VERSION("0.3"); @PACKAGE(name='+quote('virtuoso/'+public_name)+', version="0.1") }\n@RNL {\n'+definition+'\n}\n'
    return files, {"entry": entry, "top": root.name, "root_kind": "subcircuit" if root_is_subcircuit else "top", "symbol_file": "cell-symbol.rnl" if exported_symbol else None, "statistics": stats, "notes": sorted(set(notes))}
