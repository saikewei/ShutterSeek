"""Persist graph checkpoints and pending writes through Go, never locally."""
import json
import asyncio
from langgraph.checkpoint.base import BaseCheckpointSaver, CheckpointTuple


def clean(value):
    # Graph channels contain only text, observations, and asset references.
    def safe_error(item):
        if isinstance(item, BaseException):
            return {"checkpoint_error": "node execution failed"}
        raise TypeError("unsupported checkpoint value")
    text = json.dumps(value, ensure_ascii=False, default=safe_error)
    if "data:image/" in text or "credential" in text:
        raise ValueError("checkpoint contains forbidden material")
    return json.loads(text)


class GoCheckpointer(BaseCheckpointSaver):
    def __init__(self, workspace):
        super().__init__()
        self.workspace = workspace
        self.lock = asyncio.Lock()

    async def aget_tuple(self, config):
        saved = (await self.workspace.call("checkpoint_get")).get("checkpoint")
        if not saved or "checkpoint" not in saved:
            return None
        return CheckpointTuple(saved["config"], saved["checkpoint"],
                               saved["metadata"], saved.get("parent_config"),
                               [tuple(x) for x in saved.get("writes", [])])

    async def aput(self, config, checkpoint, metadata, new_versions):
        next_config = {"configurable": {
            "thread_id": config["configurable"]["thread_id"],
            "checkpoint_ns": config["configurable"].get("checkpoint_ns", ""),
            "checkpoint_id": checkpoint["id"],
        }}
        saved = clean({"config": next_config, "checkpoint": checkpoint,
                       "metadata": metadata, "parent_config": None, "writes": []})
        async with self.lock:
            await self.workspace.call("checkpoint_put", saved)
        return next_config

    async def aput_writes(self, config, writes, task_id, task_path=""):
        async with self.lock:
            saved = (await self.workspace.call("checkpoint_get")).get("checkpoint")
            if not saved:
                raise RuntimeError("checkpoint missing for pending writes")
            existing = saved.get("writes", [])
            for index, (channel, value) in enumerate(writes):
                entry = clean([task_id, channel, value])
                if not any(x[0] == task_id and x[1] == channel for x in existing):
                    existing.append(entry)
            saved["writes"] = existing
            await self.workspace.call("checkpoint_put", saved)
