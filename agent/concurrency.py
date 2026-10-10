"""Cancellation-safe FIFO capacity gates shared by all workspaces."""
import asyncio
import time
from collections import deque
from contextlib import asynccontextmanager


class FairGate:
    def __init__(self, capacity):
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
        acquired = False
        try:
            await future
            acquired = True
            yield time.monotonic() - started
        finally:
            if acquired or (future.done() and not future.cancelled()):
                self.active -= 1
            else:
                future.cancel()
            self._wake()
