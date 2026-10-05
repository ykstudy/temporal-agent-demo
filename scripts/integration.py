#!/usr/bin/env python3
"""Compatibility entry point for the independent public-HTTP validation caller.

Start an Agent separately with validation/start.py, then supply --base-url.
"""
from pathlib import Path
import runpy
import sys

root = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(root / "validation"))
runpy.run_path(str(root / "validation" / "scenarios.py"), run_name="__main__")
