"""Async gRPC server; no database credentials or photo mounts."""
import asyncio
import os
import traceback
import time
import signal
import resource
from pathlib import Path
from uuid import UUID
import grpc
from google.protobuf.struct_pb2 import Struct
from concurrency import FairGate
from rpc import Workspace, encode, decode
from model import CreationModel
from browser import Browser
from workflow import Workflow, CandidateError
from checkpoint import CheckpointMaterialError
from observability import LOGGER, RUN_REFERENCE, configure_logging, log_progress, run_reference, stage_name


def log_failure(run_id, error):
    # Never log exception text, request payloads, credentials, or model content.
    try:
        reference = str(UUID(str(run_id)))
    except (ValueError, TypeError):
        reference = "invalid"
    frames = traceback.extract_tb(error.__traceback__)
    locations = " > ".join(f"{Path(frame.filename).name}:{frame.lineno}:{frame.name}"
                           for frame in frames)
    code = error.code().name if isinstance(error, grpc.aio.AioRpcError) else "INTERNAL"
    reason = error.code if isinstance(error, (CandidateError, CheckpointMaterialError)) else "agent_execution_failed"
    LOGGER.error("creation_run_failed run=%s error_type=%s rpc_code=%s error_code=%s stack=%s",
                 reference, type(error).__name__, code, reason, locations)


class Agent:
    def __init__(self, model, browser, channel):
        self.model,self.browser,self.channel=model,browser,channel
        self.tasks={}
        self.status={}

    async def execute(self, request, context):
        try:
            async for item in self._execute(request, context):
                yield item
        except grpc.aio.AbortError:
            raise
        except Exception as error:
            log_failure(decode(request).get("run_id"), error)
            yield encode({"stage":"failed", "error_code":
                          error.code if isinstance(error, (CandidateError, CheckpointMaterialError)) else "agent_execution_failed"})
            code = error.code() if isinstance(error, grpc.aio.AioRpcError) else grpc.StatusCode.INTERNAL
            await context.abort(code, "Agent execution failed; see server diagnostics.")

    async def _execute(self, request, context):
        value=decode(request)
        run_id=value.get("run_id")
        credential=value.get("credential")
        if not run_id or not credential:
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT,"run credential required")
        if run_id in self.tasks:
            await context.abort(grpc.StatusCode.ALREADY_EXISTS,"run already executing")
        workspace=Workspace(self.channel,run_id,credential)
        # Authenticate before exposing the cancellation capability or doing work.
        await workspace.call("context")
        if run_id in self.tasks:
            await context.abort(grpc.StatusCode.ALREADY_EXISTS,"run already executing")
        events=asyncio.Queue()
        async def emit(value):
            log_progress(run_id, value)
            if run_id in self.status:
                self.status[run_id]["stage"] = stage_name(value.get("stage"))
            await events.put(value)
        async def work():
            token = RUN_REFERENCE.set(run_reference(run_id))
            try:
                await Workflow(workspace,self.model,self.browser,emit).execute()
            finally:
                RUN_REFERENCE.reset(token)
        task=asyncio.create_task(work())
        # A task cancelled before its first step never enters coroutine finally.
        task.add_done_callback(lambda completed: events.put_nowait(None))
        self.tasks[run_id]=(credential,task)
        started = time.monotonic()
        self.status[run_id] = {"started": started, "stage": "starting"}
        outcome = "failed"
        LOGGER.info("开始创作 event=run_started run=%s active_runs=%d", run_reference(run_id), len(self.tasks))
        try:
            while True:
                item=await events.get()
                if item is None:
                    await task
                    final_stage = self.status[run_id]["stage"]
                    outcome = final_stage if final_stage in ("waiting", "completed") else "completed"
                    break
                yield encode(item)
        except asyncio.CancelledError:
            outcome = "cancelled"
            raise
        finally:
            task.cancel()
            try:
                await asyncio.gather(task,return_exceptions=True)
            finally:
                self.tasks.pop(run_id,None)
                self.status.pop(run_id,None)
                LOGGER.info("创作结束 event=run_finished run=%s outcome=%s elapsed_seconds=%.3f active_runs=%d",
                            run_reference(run_id), outcome, time.monotonic() - started, len(self.tasks))

    async def cancel(self,request,context):
        value=decode(request)
        current=self.tasks.get(value.get("run_id"))
        if not current or current[0]!=value.get("credential"):
            await context.abort(grpc.StatusCode.PERMISSION_DENIED,"invalid run credential")
        LOGGER.info("取消创作 event=cancel_requested run=%s", run_reference(value.get("run_id")))
        current[1].cancel()
        return encode({"cancelled":True})


async def report_status(agent, interval=60):
    while True:
        model_gate = agent.model.gate
        browser_gate = agent.browser.gate
        LOGGER.info("运行状态 event=heartbeat active_runs=%d model_active=%d model_queued=%d browser_active=%d browser_queued=%d memory_peak_mib=%.1f",
                    len(agent.tasks), model_gate.active, sum(not item.cancelled() for item in model_gate.waiters),
                    browser_gate.active, sum(not item.cancelled() for item in browser_gate.waiters),
                    resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024)
        for run_id, current in list(agent.status.items()):
            LOGGER.info("任务状态 event=run_status run=%s stage=%s elapsed_seconds=%.3f",
                        run_reference(run_id), current["stage"], time.monotonic() - current["started"])
        await asyncio.sleep(interval)


async def main():
    configure_logging()
    LOGGER.info("准备启动 event=agent_starting model_capacity=5 browser_capacity=1")
    channel=grpc.aio.insecure_channel(
        os.environ.get("SHUTTERSEEK_CREATIONS_WORKSPACE_ADDRESS","shutterseek:8081"),
        options=[("grpc.max_receive_message_length",24<<20),
                 ("grpc.max_send_message_length",24<<20)])
    model=CreationModel(FairGate(5, "model"))
    agent=Agent(model,Browser(FairGate(1, "browser")),channel)
    server=grpc.aio.server(options=[("grpc.max_receive_message_length",24<<20),
                                    ("grpc.max_send_message_length",24<<20)])
    server.add_generic_rpc_handlers([grpc.method_handlers_generic_handler(
        "shutterseek.creation.v1.Agent",{
            "Execute":grpc.unary_stream_rpc_method_handler(agent.execute,
                request_deserializer=Struct.FromString,response_serializer=Struct.SerializeToString),
            "Cancel":grpc.unary_unary_rpc_method_handler(agent.cancel,
                request_deserializer=Struct.FromString,response_serializer=Struct.SerializeToString),
        })])
    server.add_insecure_port(os.environ.get("SHUTTERSEEK_CREATIONS_AGENT_LISTEN","0.0.0.0:50051"))
    stopping = asyncio.Event()
    loop = asyncio.get_running_loop()
    for stop_signal in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(stop_signal, stopping.set)
    await server.start()
    LOGGER.info("Agent 已就绪 event=agent_ready transport=grpc model_capacity=5 browser_capacity=1")
    heartbeat = asyncio.create_task(report_status(agent))
    try:
        await stopping.wait()
    finally:
        LOGGER.info("正在停止 event=agent_stopping active_runs=%d", len(agent.tasks))
        heartbeat.cancel()
        await asyncio.gather(heartbeat, return_exceptions=True)
        await server.stop(5)
        await channel.close()
        for stop_signal in (signal.SIGTERM, signal.SIGINT):
            loop.remove_signal_handler(stop_signal)
        LOGGER.info("Agent 已停止 event=agent_stopped")


if __name__=="__main__":
    try:
        asyncio.run(main())
    except Exception as error:
        log_failure(None, error)
        raise SystemExit(1) from None
