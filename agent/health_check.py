"""Readiness probes without model requests, database access, or credentials."""
import asyncio
import os
import sys
import grpc
from google.protobuf.struct_pb2 import Struct


async def check(address, workspace_address):
    async with grpc.aio.insecure_channel(address) as agent:
        await asyncio.wait_for(agent.channel_ready(), 4)
    async with grpc.aio.insecure_channel(workspace_address) as workspace:
        call = workspace.unary_unary(
            "/shutterseek.creation.v1.Workspace/Call",
            request_serializer=Struct.SerializeToString,
            response_deserializer=Struct.FromString,
        )
        try:
            await call(Struct(), timeout=4)
        except grpc.aio.AioRpcError as error:
            if error.code() == grpc.StatusCode.PERMISSION_DENIED:
                return
            raise
        raise RuntimeError("workspace authentication boundary unavailable")


async def main():
    try:
        await check("127.0.0.1:50051", os.environ.get(
            "SHUTTERSEEK_CREATIONS_WORKSPACE_ADDRESS", "shutterseek:8081"))
    except Exception:
        print("Agent or authenticated workspace not ready", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
