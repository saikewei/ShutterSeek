"""Readiness validates the authenticated RPC boundary without a model request."""
import sys
import unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import grpc
from google.protobuf.struct_pb2 import Struct
from health_check import check


class HealthCheckTests(unittest.IsolatedAsyncioTestCase):
    async def exercise(self, code):
        agent = grpc.aio.server()
        agent_port = agent.add_insecure_port("127.0.0.1:0")
        workspace = grpc.aio.server()
        async def call(request, context):
            await context.abort(code, "test response")
        workspace.add_generic_rpc_handlers([grpc.method_handlers_generic_handler(
            "shutterseek.creation.v1.Workspace", {
                "Call": grpc.unary_unary_rpc_method_handler(call,
                    request_deserializer=Struct.FromString, response_serializer=Struct.SerializeToString)
            })])
        workspace_port = workspace.add_insecure_port("127.0.0.1:0")
        await agent.start()
        await workspace.start()
        try:
            await check("127.0.0.1:" + str(agent_port), "127.0.0.1:" + str(workspace_port))
        finally:
            await agent.stop(0)
            await workspace.stop(0)

    async def test_healthy_rpc_pair_rejects_unauthenticated_workspace_call(self):
        await self.exercise(grpc.StatusCode.PERMISSION_DENIED)

    async def test_wrong_workspace_service_is_unhealthy(self):
        with self.assertRaises(grpc.aio.AioRpcError):
            await self.exercise(grpc.StatusCode.UNIMPLEMENTED)
