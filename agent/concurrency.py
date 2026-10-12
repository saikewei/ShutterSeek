"""Cancellation-safe FIFO capacity gates shared by all workspaces."""
import asyncio
import time
from collections import deque
from contextlib import asynccontextmanager
from observability import log_gate


class FairGate:
    def __init__(self, capacity, name=None):
        self.name = name if name in ("model", "browser") else None
        self.capacity = capacity
        self.active = 0
        self.peak = 0
        self.waiters = deque()

    def _wake(self):
        while self.active < self.capacity and self.waiters:
            future = self.waiters.popleft()
            if future.cancelled():
                continue
            self.active += 1
            self.peak = max(self.peak, self.active)
            future.set_result(None)

    @asynccontextmanager
    async def slot(self):
        started = time.monotonic()
        future = asyncio.get_running_loop().create_future()
        self.waiters.append(future)
        self._wake()
        if not future.done():
            log_gate(self.name, "resource_queued", self, 0)
        acquired = False
        try:
            await future
            acquired = True
            queued = time.monotonic() - started
            executing = time.monotonic()
            log_gate(self.name, "resource_started", self, 0, queued)
            yield queued
        finally:
            if acquired or (future.done() and not future.cancelled()):
                self.active -= 1
            else:
                future.cancel()
            self._wake()
            log_gate(self.name, "resource_released" if acquired else "resource_cancelled",
                     self, time.monotonic() - executing if acquired else time.monotonic() - started)
