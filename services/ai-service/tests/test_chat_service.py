"""ChatService gRPC 适配层的最小测试：组装响应与错误转换。"""

from __future__ import annotations

import grpc
import pytest
from ai.v1 import chat_pb2, common_pb2

from app.chat_service import ChatService
from providers.base import ChatProvider, ProviderError
from providers.fake import FakeProvider


class _Aborted(Exception):
    """模拟 context.abort 抛出，用于中断 servicer 方法。"""


class _FakeGrpcContext:
    def __init__(self) -> None:
        self.aborted: tuple[grpc.StatusCode, str] | None = None

    async def abort(self, code: grpc.StatusCode, details: str = "") -> None:
        self.aborted = (code, details)
        raise _Aborted()


class _FailingProvider(ChatProvider):
    name = "failing"

    async def complete(self, *args, **kwargs):
        raise ProviderError("boom")


def _complete_request(text: str) -> chat_pb2.CompleteRequest:
    return chat_pb2.CompleteRequest(
        context=common_pb2.RequestContext(request_id="req-123", model_profile="default"),
        messages=[
            common_pb2.ChatMessage(
                role=common_pb2.MESSAGE_ROLE_USER,
                content=[common_pb2.ContentPart(text=common_pb2.TextPart(text=text))],
            )
        ],
        max_output_tokens=100,
        temperature=0.0,
    )


async def test_complete_returns_response():
    service = ChatService(FakeProvider())
    response = await service.Complete(_complete_request("hello"), _FakeGrpcContext())
    assert response.model_name == "fake"
    assert response.message.role == common_pb2.MESSAGE_ROLE_ASSISTANT
    assert response.latency_ms >= 0


async def test_complete_converts_provider_error():
    service = ChatService(_FailingProvider())
    grpc_context = _FakeGrpcContext()
    with pytest.raises(_Aborted):
        await service.Complete(_complete_request("hello"), grpc_context)
    assert grpc_context.aborted == (grpc.StatusCode.INTERNAL, "AI provider error")


async def test_complete_stream_unimplemented():
    service = ChatService(FakeProvider())
    grpc_context = _FakeGrpcContext()
    with pytest.raises(_Aborted):
        await service.CompleteStream(chat_pb2.CompleteStreamRequest(), grpc_context)
    assert grpc_context.aborted[0] == grpc.StatusCode.UNIMPLEMENTED
