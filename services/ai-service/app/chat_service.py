"""ChatService：把 ChatProvider 暴露为 gRPC 服务。

职责：提取 request_id → 设置请求上下文 → 调用 Provider → 记录耗时 →
组装响应；Provider 异常统一记录并转换为 gRPC INTERNAL。
"""

from __future__ import annotations

import logging
import time

import grpc
from ai.v1 import chat_pb2, chat_pb2_grpc, common_pb2

from providers.base import ChatProvider

from . import context as request_context

logger = logging.getLogger("flowmind.ai.chat")


class ChatService(chat_pb2_grpc.ChatServiceServicer):
    def __init__(self, provider: ChatProvider) -> None:
        self._provider = provider

    async def Complete(self, request: chat_pb2.CompleteRequest, grpc_context):
        request_context.set_request_context(
            request.context.request_id, request.context.trace_id
        )
        started = time.perf_counter()
        try:
            message = await self._provider.complete(
                request.context,
                request.messages,
                request.max_output_tokens,
                request.temperature,
            )
        except Exception:
            latency_ms = _elapsed_ms(started)
            logger.exception(
                "provider call failed provider=%s latency_ms=%d",
                self._provider.name,
                latency_ms,
            )
            await grpc_context.abort(grpc.StatusCode.INTERNAL, "AI provider error")

        latency_ms = _elapsed_ms(started)
        logger.info(
            "chat completed provider=%s latency_ms=%d messages=%d",
            self._provider.name,
            latency_ms,
            len(request.messages),
        )
        return chat_pb2.CompleteResponse(
            message=message,
            model_name=self._provider.name,
            usage=common_pb2.TokenUsage(),
            latency_ms=latency_ms,
        )

    async def CompleteStream(self, request: chat_pb2.CompleteStreamRequest, grpc_context):
        # 阶段 1.9 只实现最小 Chat 方法（Complete）；流式后续阶段再补。
        await grpc_context.abort(
            grpc.StatusCode.UNIMPLEMENTED, "CompleteStream not implemented yet"
        )


def _elapsed_ms(started: float) -> int:
    return int((time.perf_counter() - started) * 1000)
