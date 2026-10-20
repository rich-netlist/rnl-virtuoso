"""Cadence native netlist + EDIF to RNL. Offline conversion needs no Virtuoso."""

__version__ = "0.5.1"


class ExportError(ValueError):
    """An input cannot be translated without guessing or losing circuit data."""
