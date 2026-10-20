"""Pinned symbol/style resources shared by source and packaged installations."""
from pathlib import Path


DIRECTORY = Path(__file__).resolve().parents[3] / "resources"


def read(name):
    return (DIRECTORY / name).read_text(encoding="utf-8")
