"""Keep RC qualification exhaustive without charging every beta for the full matrix."""
import json
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]


def workflow(name):
    return yaml.safe_load((ROOT / ".github/workflows" / name).read_text())


class BrowserReleasePolicyTests(unittest.TestCase):
    def test_full_matrix_and_required_baseline_remain_complete(self):
        matrix = json.loads((ROOT / ".github/browser-compatibility.json").read_text())
        self.assertEqual(len(matrix), 35)
        self.assertEqual(len({row["id"] for row in matrix}), 35)
        self.assertEqual(sum(row["tier"] == "required" for row in matrix), 5)
        required = {(row["os"], row["engine"]) for row in matrix if row["tier"] == "required"}
        self.assertEqual(required, {("ubuntu-latest", "chromium"), ("ubuntu-latest", "firefox"),
                                   ("ubuntu-latest", "webkit"), ("windows-2022", "chromium"),
                                   ("macos-15-intel", "webkit")})

    def test_intel_browser_routing_preserves_generations_and_native_targets(self):
        matrix = json.loads((ROOT / ".github/browser-compatibility.json").read_text())
        mac = [row for row in matrix if row["os"].startswith("macos-")]
        self.assertEqual(len(mac), 11)
        self.assertEqual({row["os"] for row in mac}, {"macos-15-intel"})
        self.assertTrue(all(row["id"].startswith("macos-15-intel-") for row in mac))
        bundled = [row for row in mac if not row.get("channel")]
        generations = {row["playwright"] for row in matrix if not row.get("channel")}
        self.assertEqual(len(generations), 3)
        self.assertEqual({(row["engine"], row["playwright"]) for row in bundled},
                         {(engine, version) for engine in ("chromium", "firefox", "webkit")
                          for version in generations})
        self.assertEqual({row["channel"] for row in mac if row.get("channel")},
                         {"chrome", "msedge"})
        for row in mac:
            lock = json.loads((ROOT / row["toolchain"] / "package-lock.json").read_text())
            self.assertEqual(lock["packages"]["node_modules/playwright-core"]["version"],
                             row["playwright"])
        # Browser routing must not remove either separately built Darwin artifact.
        targets = workflow("release-build.yml")["jobs"]["binaries"]["strategy"]["matrix"]["include"]
        self.assertEqual({(row["goarch"], row["name"]) for row in targets if row["goos"] == "darwin"},
                         {("arm64", "macos-silicon"), ("amd64", "macos-intel")})
        policy = json.loads((ROOT / ".release-policy.json").read_text())
        self.assertTrue({"release-inputs-macos-silicon.json", "release-inputs-macos-intel.json"}
                        <= set(policy["versioning"]["required_build_receipts"]))

    def test_browser_route_remains_required_and_channel_selection_is_explicit(self):
        browser = workflow("ci.yml")["jobs"]["browser-compatibility"]
        self.assertEqual(browser["with"]["browser-extended"], "${{ github.event_name == 'schedule' || (github.event_name == 'workflow_dispatch' && (github.event.inputs.channel == '' || github.event.inputs.channel == 'rc')) }}")
        self.assertEqual(browser["with"]["browser-release-channel"], "${{ github.event_name == 'workflow_dispatch' && github.event.inputs.channel || '' }}")
        self.assertFalse(browser.get("continue-on-error", False))
        quality = workflow("quality-gate.yml")["jobs"]
        self.assertEqual(quality["check-0"]["uses"], "./.github/workflows/ci.yml")
        self.assertIn("check-0", quality["gate"]["needs"])


if __name__ == "__main__":
    unittest.main()
