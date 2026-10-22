"""Release tests use a fake Docker CLI and never touch production services."""
import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
REVISION = "a" * 40


class DeploymentTests(unittest.TestCase):
    def exercise(self, enabled=True, key=True, failure=False, logs=True):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            release = root / "release"
            (release / "scripts").mkdir(parents=True)
            shutil.copy(ROOT / "scripts/deploy.sh", release / "scripts/deploy.sh")
            shutil.copy(ROOT / "docker-compose.yml", release / "docker-compose.yml")
            (root / ".env").write_text("")
            docker = root / "docker"
            docker.write_text("""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with Path(os.environ["FAKE_LOG"]).open("a") as output:
    output.write(json.dumps({"args": args, "tag": os.environ.get("IMAGE_TAG")}) + "\\n")
if "--help" in args:
    print("--wait-timeout")
elif "config" in args and "--quiet" not in args:
    print('services:\\n  shutterseek:\\n    environment:\\n      SHUTTERSEEK_CREATIONS_ENABLED: "' + os.environ["FAKE_ENABLED"] + '"')
    if os.environ["FAKE_KEY"] == "true":
        print("      SHUTTERSEEK_CREATIONS_MODEL_KEY: test-only-model-key")
    else:
        print('      SHUTTERSEEK_CREATIONS_MODEL_KEY: ""')
elif "logs" in args and os.environ["FAKE_VISIBLE_LOGS"] == "true":
    print("event=agent_ready")
    print("event=heartbeat active_runs=0")
elif "up" in args and os.environ["FAKE_FAILURE"] == "true":
    sys.exit(42)
""")
            docker.chmod(0o700)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       DEPLOY_SHA=REVISION, SHUTTERSEEK_DEPLOY_PROJECT_DIR=str(root),
                       FAKE_ENABLED="true" if enabled else "false",
                       FAKE_KEY="true" if key else "false",
                       FAKE_FAILURE="true" if failure else "false",
                       FAKE_VISIBLE_LOGS="true" if logs else "false",
                       FAKE_LOG=str(root / "calls"))
            result = subprocess.run(["sh", str(release / "scripts/deploy.sh")], env=env, capture_output=True, text=True)
            if not (root / "calls").exists():
                raise AssertionError(result.stderr)
            calls = [json.loads(line) for line in (root / "calls").read_text().splitlines()]
            marker = (release / "deployed-revision").exists()
            return result, calls, marker

    def test_enabled_release_includes_agent_and_waits_for_health(self):
        result, calls, marker = self.exercise()
        self.assertEqual(result.returncode, 0, result.stderr)
        up = next(call for call in calls if "up" in call["args"] and "--help" not in call["args"])
        self.assertIn("agent", up["args"])
        self.assertIn("nginx", up["args"])
        self.assertIn("--wait", up["args"])
        self.assertIn("--wait-timeout", up["args"])
        self.assertEqual(up["tag"], REVISION)
        self.assertTrue(marker)

    def test_disabled_feature_deploys_app_embedding_and_nginx(self):
        result, calls, marker = self.exercise(enabled=False, key=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        up = next(call for call in calls if "up" in call["args"] and "--help" not in call["args"])
        self.assertNotIn("agent", up["args"])
        self.assertIn("nginx", up["args"])
        self.assertTrue(marker)

    def test_missing_configuration_stops_before_any_pull_or_restart(self):
        result, calls, marker = self.exercise(key=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any("pull" in call["args"] for call in calls))
        self.assertFalse(any("up" in call["args"] and "--help" not in call["args"] for call in calls))
        self.assertFalse(marker)

    def test_failed_health_is_not_recorded_as_successful_deployment(self):
        result, calls, marker = self.exercise(failure=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(marker)
        self.assertNotIn("healthy", result.stdout)

    def test_missing_stdout_logs_prevent_success_marker(self):
        result, calls, marker = self.exercise(logs=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Agent startup log unavailable", result.stderr)
        self.assertFalse(marker)
