"""Keep PR and release scans comparable without reducing scanner coverage."""

import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]


def workflow(name):
    """Load GitHub's string-valued workflow keys without YAML boolean coercion."""
    return yaml.load(  # nosec B506 - BaseLoader only constructs strings and containers
        (ROOT / ".github/workflows" / name).read_text(), Loader=yaml.BaseLoader
    )


class TrivyReportingTests(unittest.TestCase):
    def test_pr_and_release_use_the_existing_main_analysis_category(self):
        quality = workflow("quality-gate.yml")
        release = workflow("release-pipeline.yml")
        self.assertIn("pull_request", quality["on"])
        self.assertEqual(
            release["jobs"]["checks"]["uses"],
            "./.github/workflows/quality-gate.yml",
        )
        self.assertEqual(
            quality["jobs"]["check-3"]["uses"],
            "./.github/workflows/trivy-fs.yml",
        )
        uploads = [
            step
            for step in workflow("trivy-fs.yml")["jobs"]["trivy-scan"]["steps"]
            if step.get("uses", "").startswith("github/codeql-action/upload-sarif@")
        ]
        self.assertEqual(len(uploads), 1)
        # This is an existing main-branch category, not a new empty baseline.
        self.assertEqual(
            uploads[0]["with"].get("category"),
            ".github/workflows/release-pipeline.yml:trivy-scan",
        )
        self.assertEqual(uploads[0]["if"], "always()")
        self.assertEqual(uploads[0]["with"]["sarif_file"], "trivy-results.sarif")

    def test_shared_scan_still_blocks_high_and_critical_findings(self):
        job = workflow("trivy-fs.yml")["jobs"]["trivy-scan"]
        scans = [
            step for step in job["steps"]
            if step.get("uses", "").startswith("aquasecurity/trivy-action@")
        ]
        self.assertEqual(len(scans), 1)
        self.assertNotIn("continue-on-error", job)
        self.assertNotIn("continue-on-error", scans[0])
        self.assertNotIn("if", scans[0])
        self.assertEqual(scans[0]["with"]["scan-type"], "fs")
        self.assertEqual(scans[0]["with"]["scan-ref"], ".")
        self.assertEqual(scans[0]["with"]["exit-code"], "1")
        self.assertEqual(scans[0]["with"]["severity"], "HIGH,CRITICAL")
        self.assertEqual(scans[0]["with"]["output"], "trivy-results.sarif")
