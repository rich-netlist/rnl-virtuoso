"""Test support for the Go exporter: EDIF fixture parsing, resources and the symbol table.

The Python reference backend is retired; only the Go backend converts.
"""


class ExportError(ValueError):
    """An input cannot be translated without guessing or losing circuit data."""
