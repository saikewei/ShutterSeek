"""Operational logs must be useful, isolated and free of private payloads."""
import asyncio
import os
import signal
import socket
import sys
import unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from concurrency import FairGate
from observability import RUN_REFERENCE, log_progress
from server import report_status

FIRST = "11111111-1111-4111-8111-111111111111"
SECOND = "22222222-2222-4222-8222-222222222222"


class ObservabilityTests(unittest.IsolatedAsyncioTestCase):
    def test_progress_reports_only_allowlisted_status_and_metrics(self):
        payload = {"stage": "reviewed", "round": 2, "summary": "private-user-story",
                   "html": "private-html", "credential": "private-capability",
                   "screenshots": [{"data": "data:image/png;base64,private-image"}],
                   "issues": [{"description": "private-fact"}], "verified": False,
                   "model_seconds": 1.25, "usage": {"input_tokens": 8, "output_tokens": 2,
                   "total_tokens": 10, "private": "private-model-key"}}
        with self.assertLogs("shutterseek.agent", level="INFO") as logs:
            log_progress(FIRST, payload)
            log_progress("private-user-id\nforged=log", {"stage": "private-stage", "status": "private-status",
                          "model_seconds": float("nan"), "round": "private-model-key", "total": 10 ** 400})
        output = "\n".join(logs.output)
        for expected in ("stage=reviewed", "round=2", "screenshots_count=1", "issues_count=1",
                         "verified=false", "input_tokens=8", "model_seconds=1.250", "run=invalid", "stage=unknown"):
            self.assertIn(expected, output)
        for forbidden in ("private-", "data:image", "forged=log", "nan"):
            self.assertNotIn(forbidden, output)

    async def test_queued_resources_keep_each_run_reference_and_release_slots(self):
        gate = FairGate(1, "model")
        entered = asyncio.Event()
        release = asyncio.Event()
        async def first():
            token = RUN_REFERENCE.set(FIRST)
            try:
                async with gate.slot():
                    entered.set()
                    await release.wait()
            finally:
                RUN_REFERENCE.reset(token)
        async def second():
            token = RUN_REFERENCE.set(SECOND)
            try:
                async with gate.slot():
                    pass
            finally:
                RUN_REFERENCE.reset(token)
        with self.assertLogs("shutterseek.agent", level="INFO") as logs:
            one = asyncio.create_task(first())
            await entered.wait()
            two = asyncio.create_task(second())
            await asyncio.sleep(0)
            release.set()
            await asyncio.gather(one, two)
        output = "\n".join(logs.output)
        self.assertTrue(any("event=resource_queued" in line and "run=" + SECOND in line for line in logs.output))
        self.assertTrue(any("event=resource_started" in line and "run=" + FIRST in line for line in logs.output))
        self.assertTrue(any("event=resource_started" in line and "run=" + SECOND in line for line in logs.output))
        self.assertIn("event=resource_released", output)
        self.assertEqual(gate.active, 0)
        self.assertFalse(gate.waiters)

    async def test_idle_and_active_heartbeat_show_current_status(self):
        class Engine:
            gate = FairGate(1)
        class Agent:
            model = Engine()
            browser = Engine()
            tasks = {}
            status = {}
        agent = Agent()
        with self.assertLogs("shutterseek.agent", level="INFO") as logs:
            heartbeat = asyncio.create_task(report_status(agent, .01))
            await asyncio.sleep(.005)
            agent.tasks[FIRST] = object()
            agent.status[FIRST] = {"started": 0, "stage": "generating"}
            await asyncio.sleep(.015)
            heartbeat.cancel()
            await asyncio.gather(heartbeat, return_exceptions=True)
        self.assertTrue(any("event=heartbeat active_runs=0" in line for line in logs.output))
        self.assertTrue(any("event=heartbeat active_runs=1" in line for line in logs.output))
        self.assertTrue(any("event=run_status run=" + FIRST + " stage=generating" in line for line in logs.output))

    async def test_process_flushes_startup_to_stdout_and_logs_graceful_shutdown(self):
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        env = dict(os.environ, SHUTTERSEEK_CREATIONS_MODEL_KEY="private-test-startup-key",
                   SHUTTERSEEK_CREATIONS_AGENT_LISTEN="127.0.0.1:" + str(port))
        process = await asyncio.create_subprocess_exec(sys.executable, "-u", "server.py",
            cwd=Path(__file__).resolve().parents[1], env=env,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE)
        lines = []
        try:
            async with asyncio.timeout(20):
                while True:
                    line = await process.stdout.readline()
                    if not line:
                        raise AssertionError("server stopped before readiness")
                    lines.append(line.decode())
                    if "event=agent_ready" in lines[-1]:
                        break
            process.send_signal(signal.SIGTERM)
            stdout, stderr = await asyncio.wait_for(process.communicate(), 10)
            output = "".join(lines) + stdout.decode() + stderr.decode()
            self.assertEqual(process.returncode, 0, output)
            self.assertIn("event=agent_starting", output)
            self.assertIn("event=agent_stopping", output)
            self.assertIn("event=agent_stopped", output)
            self.assertNotIn("private-test-startup-key", output)
        finally:
            if process.returncode is None:
                process.kill()
                await process.wait()
