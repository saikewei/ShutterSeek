import asyncio
import sys
import unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import grpc
from google.protobuf.struct_pb2 import Struct
from rpc import encode,decode
from server import Agent
from workflow import CandidateError
from concurrency import FairGate
from test_workflow import MemoryWorkspace,FakeModel,FakeBrowser


class RPCTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.workspaces={str(x):MemoryWorkspace(x) for x in range(1,6)}
        self.workspace_server=grpc.aio.server()
        async def callback(request,context):
            value=decode(request)
            run=value.get("run_id")
            if run not in self.workspaces or value.get("credential")!="key-"+run:
                await context.abort(grpc.StatusCode.PERMISSION_DENIED,"invalid capability")
            return encode(await self.workspaces[run].call(value["op"],value.get("payload")))
        self.workspace_server.add_generic_rpc_handlers([
            grpc.method_handlers_generic_handler("shutterseek.creation.v1.Workspace",{
                "Call":grpc.unary_unary_rpc_method_handler(callback,
                    request_deserializer=Struct.FromString,
                    response_serializer=Struct.SerializeToString)})])
        port=self.workspace_server.add_insecure_port("127.0.0.1:0")
        await self.workspace_server.start()
        self.workspace_channel=grpc.aio.insecure_channel("127.0.0.1:"+str(port))
        self.model_gate=FairGate(5);self.browser_gate=FairGate(1)
        self.agent=Agent(FakeModel(self.model_gate),FakeBrowser(self.browser_gate),
                         self.workspace_channel)
        self.server=grpc.aio.server()
        self.server.add_generic_rpc_handlers([
            grpc.method_handlers_generic_handler("shutterseek.creation.v1.Agent",{
                "Execute":grpc.unary_stream_rpc_method_handler(self.agent.execute,
                    request_deserializer=Struct.FromString,
                    response_serializer=Struct.SerializeToString),
                "Cancel":grpc.unary_unary_rpc_method_handler(self.agent.cancel,
                    request_deserializer=Struct.FromString,
                    response_serializer=Struct.SerializeToString)})])
        port=self.server.add_insecure_port("127.0.0.1:0")
        await self.server.start()
        self.channel=grpc.aio.insecure_channel("127.0.0.1:"+str(port))
        self.execute=self.channel.unary_stream("/shutterseek.creation.v1.Agent/Execute",
            request_serializer=Struct.SerializeToString,response_deserializer=Struct.FromString)
        self.cancel=self.channel.unary_unary("/shutterseek.creation.v1.Agent/Cancel",
            request_serializer=Struct.SerializeToString,response_deserializer=Struct.FromString)

    async def asyncTearDown(self):
        await self.channel.close()
        await self.server.stop(0)
        await self.workspace_channel.close()
        await self.workspace_server.stop(0)

    async def test_five_authenticated_streams_keep_workspaces_isolated(self):
        async def run(number):
            events=[]
            async for item in self.execute(encode({"run_id":str(number),"credential":"key-"+str(number)})):
                events.append(decode(item))
            self.assertEqual(events[-1]["stage"],"completed")
        await asyncio.gather(*(run(x) for x in range(1,6)))
        for number,workspace in self.workspaces.items():
            self.assertTrue(workspace.finished["verified"])
            self.assertEqual(workspace.finished["used_photo_ids"],[int(number)])
        self.assertLessEqual(self.model_gate.peak,5)
        self.assertEqual(self.browser_gate.peak,1)

    async def test_invalid_run_capability_rejected(self):
        with self.assertRaises(grpc.aio.AioRpcError):
            async for item in self.execute(encode({"run_id":"1","credential":"other"})):
                pass
        self.assertIsNone(self.workspaces["1"].finished)

    async def test_execution_failure_logs_locations_without_payloads(self):
        async def fail(*args, **kwargs):
            raise RuntimeError("private-model-key data:image/png;base64,private-image")
        self.agent.model.invoke = fail
        run_id = "1"
        with self.assertLogs("shutterseek.agent", level="ERROR") as logs:
            with self.assertRaises(grpc.aio.AioRpcError) as error:
                async for item in self.execute(encode({"run_id": run_id, "credential": "key-1"})):
                    pass
        self.assertEqual(error.exception.code(), grpc.StatusCode.INTERNAL)
        self.assertIn("server diagnostics", error.exception.details())
        output = "\n".join(logs.output) + error.exception.details()
        self.assertIn("error_type=RuntimeError", output)
        self.assertIn("workflow.py:", output)
        for forbidden in ("private-model-key", "data:image", "private-image", "key-1"):
            self.assertNotIn(forbidden, output)
        self.assertNotIn(run_id, self.agent.tasks)
        self.assertIsNone(self.workspaces[run_id].finished)

    async def test_known_candidate_failure_streams_safe_reason_before_abort(self):
        async def invalid(*args, **kwargs):
            raise CandidateError("confirmed_photos_changed")
        self.agent.model.invoke = invalid
        events = []
        with self.assertLogs("shutterseek.agent", level="ERROR") as logs:
            with self.assertRaises(grpc.aio.AioRpcError) as error:
                async for item in self.execute(encode({"run_id":"1", "credential":"key-1"})):
                    events.append(decode(item))
        self.assertEqual(error.exception.code(), grpc.StatusCode.INTERNAL)
        self.assertEqual(events[-1], {"stage":"failed", "error_code":"confirmed_photos_changed"})
        self.assertIn("error_code=confirmed_photos_changed", "\n".join(logs.output))

    async def test_callback_authentication_failure_keeps_status_and_safe_log(self):
        with self.assertLogs("shutterseek.agent", level="ERROR") as logs:
            with self.assertRaises(grpc.aio.AioRpcError) as error:
                async for item in self.execute(encode({"run_id": "1", "credential": "private-invalid-capability"})):
                    pass
        self.assertEqual(error.exception.code(), grpc.StatusCode.PERMISSION_DENIED)
        self.assertIn("rpc_code=PERMISSION_DENIED", "\n".join(logs.output))
        self.assertNotIn("private-invalid-capability", "\n".join(logs.output))
        self.assertEqual(self.agent.tasks, {})

    async def test_cancel_capability_and_disconnected_stream(self):
        original=self.agent.model.invoke
        async def stalled(*args,**kwargs):
            await asyncio.sleep(10)
            return await original(*args,**kwargs)
        self.agent.model.invoke=stalled
        stream=self.execute(encode({"run_id":"1","credential":"key-1"}))
        # Start consumption, otherwise gRPC never executes the lazy stream.
        reader=asyncio.create_task(stream.read())
        for _ in range(100):
            if "1" in self.agent.tasks:break
            await asyncio.sleep(.005)
        with self.assertRaises(grpc.aio.AioRpcError) as error:
            await self.cancel(encode({"run_id":"1","credential":"other"}))
        self.assertEqual(error.exception.code(),grpc.StatusCode.PERMISSION_DENIED)
        await self.cancel(encode({"run_id":"1","credential":"key-1"}))
        stream.cancel()
        await asyncio.gather(reader,return_exceptions=True)
        for _ in range(100):
            if "1" not in self.agent.tasks:break
            await asyncio.sleep(.005)
        self.assertNotIn("1",self.agent.tasks)
        self.assertIsNone(self.workspaces["1"].finished)
