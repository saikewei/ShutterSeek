"""The protobuf Struct envelope contract lives in rpc/creation.proto."""
import grpc
from google.protobuf.struct_pb2 import Struct
from google.protobuf.json_format import MessageToDict


def encode(value):
    out = Struct()
    out.update(value)
    return out


def decode(value):
    return MessageToDict(value)


class Workspace:
    def __init__(self, channel, run_id, credential):
        self.run_id = run_id
        self.credential = credential
        self.call_rpc = channel.unary_unary(
            "/shutterseek.creation.v1.Workspace/Call",
            request_serializer=Struct.SerializeToString,
            response_deserializer=Struct.FromString,
        )

    async def call(self, op, payload=None):
        value = await self.call_rpc(
            encode({"run_id": self.run_id, "credential": self.credential,
                    "op": op, "payload": payload or {}}), timeout=30
        )
        return decode(value)
