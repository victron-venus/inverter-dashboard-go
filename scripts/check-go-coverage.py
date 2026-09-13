#!/usr/bin/env python3
"""Enforce the same total coverage threshold as reusable Go CI."""

import re
import subprocess
import sys

result = subprocess.check_output(
    ["go", "tool", "cover", "-func=coverage.out"], text=True
)
match = re.search(r"^total:.*?([0-9.]+)%$", result, re.MULTILINE)
if not match or float(match.group(1)) < float(sys.argv[1]):
    raise SystemExit(f"Coverage below {sys.argv[1]}% or report is missing")
print(f"Coverage: {match.group(1)}%")
