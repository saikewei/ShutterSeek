"""Async gRPC server; no database credentials or photo mounts."""
import asyncio
import os
import logging
import traceback
from pathlib import Path
from uuid import UUID
import grpc
from google.protobuf.struct_pb2 import Struct
from concurrency import FairGate
from rpc import Workspace, encode, decode
from model import CreationModel
from browser import Browser
from workflow import Workflow, CandidateError

LOGGER = logging.getLogger("shutterseek.agent")


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
    reason = error.code if isinstance(error, CandidateError) else "agent_execution_failed"
    LOGGER.error("creation_run_failed run=%s error_type=%s rpc_code=%s error_code=%s stack=%s",
                 reference, type(error).__name__, code, reason, locations)


class Agent:
    def __init__(self, model, browser, channel):
        self.model,self.browser,self.channel=model,browser,channel
        self.tasks={}

    async def execute(self, request, context):
        try:
            async for item in self._execute(request, context):
                yield item
        except grpc.aio.AbortError:
            raise
        except Exception as error:
            log_failure(decode(request).get("run_id"), error)
            yield encode({"stage":"failed", "error_code":
                          error.code if isinstance(error, CandidateError) else "agent_execution_failed"})
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
            await events.put(value)
        async def work():
            await Workflow(workspace,self.model,self.browser,emit).execute()
        task=asyncio.create_task(work())
        # A task cancelled before its first step never enters coroutine finally.
        task.add_done_callback(lambda completed: events.put_nowait(None))
        self.tasks[run_id]=(credential,task)
        try:
            while True:
                item=await events.get()
                if item is None:
                    await task
                    break
                yield encode(item)
        finally:
            task.cancel()
            try:
                await asyncio.gather(task,return_exceptions=True)
            finally:
                self.tasks.pop(run_id,None)

    async def cancel(self,request,context):
        value=decode(request)
        current=self.tasks.get(value.get("run_id"))
        if not current or current[0]!=value.get("credential"):
            await context.abort(grpc.StatusCode.PERMISSION_DENIED,"invalid run credential")
        current[1].cancel()
        return encode({"cancelled":True})


async def main():
    logging.basicConfig(level=logging.WARNING,
                        format="%(asctime)s %(levelname)s %(name)s %(message)s")
    channel=grpc.aio.insecure_channel(
        os.environ.get("SHUTTERSEEK_CREATIONS_WORKSPACE_ADDRESS","shutterseek:8081"),
        options=[("grpc.max_receive_message_length",24<<20),
                 ("grpc.max_send_message_length",24<<20)])
    model=CreationModel(FairGate(5))
    agent=Agent(model,Browser(FairGate(1)),channel)
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
    await server.start()
    try:
        await server.wait_for_termination()
    finally:
        await server.stop(5)
        await channel.close()


if __name__=="__main__":
    asyncio.run(main())
