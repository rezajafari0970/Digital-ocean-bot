#!/usr/bin/env python3
"""Repository entry point for the standalone publisher's canonical release gate."""
from pathlib import Path
import runpy

GATE = Path(__file__).resolve().parents[2] / 'tools/public-release/transport-regression-gate.py'

if __name__ == '__main__':
    runpy.run_path(str(GATE), run_name='__main__')
else:
    _implementation = runpy.run_path(str(GATE))
    REQUIRED = _implementation['REQUIRED']
    verify_events = _implementation['verify_events']
    main = _implementation['main']
