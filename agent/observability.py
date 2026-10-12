"""Allowlisted operational logs never serialize model or workspace payloads."""
import logging
import math
import sys
from contextvars import ContextVar
from uuid import UUID

LOGGER = logging.getLogger("shutterseek.agent")
RUN_REFERENCE = ContextVar("creation_log_run", default="-")
STAGES = {
    "localizing": "整理中文内容", "observing": "理解照片", "planning": "规划方案",
    "generating": "生成网页", "designing": "使用设计工具", "rendering": "渲染检查",
    "rendered": "截图完成", "reviewing": "视觉审查", "reviewed": "审查完成",
    "repairing": "修复页面", "saving": "保存作品", "waiting": "等待用户回复",
    "completed": "创作完成", "starting": "准备创作",
}


def configure_logging():
    logging.basicConfig(level=logging.WARNING, stream=sys.stdout,
                        format="%(asctime)s %(levelname)s %(name)s %(message)s", force=True)
    LOGGER.setLevel(logging.INFO)


def run_reference(value):
    try:
        return str(UUID(str(value)))
    except (ValueError, TypeError, AttributeError):
        return "invalid"


def stage_name(value):
    return value if isinstance(value, str) and value in STAGES else "unknown"


def number(value):
    return (isinstance(value, (int, float)) and not isinstance(value, bool)
            and 0 <= value <= 1e12 and math.isfinite(value))


def log_progress(run_id, event):
    stage = stage_name(event.get("stage"))
    status = event.get("status")
    status = status if isinstance(status, str) and status in ("started", "finished") else "update"
    fields = []
    for key in ("round", "attempt", "completed", "total", "model_seconds",
                "model_queue_seconds", "browser_seconds", "browser_queue_seconds",
                "tool_calls", "tool_seconds"):
        value = event.get(key)
        if number(value):
            fields.append(f"{key}={value:.3f}" if isinstance(value, float) else f"{key}={value}")
    usage = event.get("usage")
    if isinstance(usage, dict):
        for key in ("input_tokens", "output_tokens", "total_tokens"):
            value = usage.get(key)
            if number(value):
                fields.append(f"{key}={value}")
    for key in ("screenshots", "issues"):
        value = event.get(key)
        if isinstance(value, list):
            fields.append(f"{key}_count={len(value)}")
    if isinstance(event.get("verified"), bool):
        fields.append("verified=" + str(event["verified"]).lower())
    LOGGER.info("创作进度 event=stage run=%s stage=%s label=%s status=%s %s",
                run_reference(run_id), stage, STAGES.get(stage, "未知阶段"), status, " ".join(fields))


def log_gate(name, event, gate, started, queue_seconds=None):
    if name not in ("model", "browser"):
        return
    fields = ""
    if queue_seconds is not None:
        fields += f" queue_seconds={queue_seconds:.3f}"
    LOGGER.info("资源调度 event=%s resource=%s run=%s active=%d queued=%d capacity=%d elapsed_seconds=%.3f%s",
                event, name, RUN_REFERENCE.get(), gate.active,
                sum(not item.cancelled() for item in gate.waiters), gate.capacity,
                started, fields)
