"""Use the reviewed application toolchain for its security and race checks."""

from pathlib import Path
import re
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[2]


class GoCIToolchainTests(unittest.TestCase):
    def test_ci_uses_the_toolchain_selected_by_go_mod(self):
        selected = re.search(r"^toolchain go([0-9]+\.[0-9]+\.[0-9]+)$", (ROOT / "go.mod").read_text(), re.M)
        self.assertIsNotNone(selected, "Keep an explicit reviewed Go patch toolchain")
        workflow = yaml.safe_load((ROOT / ".github/workflows/ci.yml").read_text())
        self.assertEqual(workflow["jobs"]["ci"]["with"]["go-version"], selected.group(1))
