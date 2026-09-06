"""ChatService：把 ChatProvider 暴露为 gRPC 服务并分类转换调用故障。"""

from __future__ import annotations

import logging
import time

import grpc
from ai.v1 import chat_pb2, chat_pb2_grpc, common_pb2

from providers.base import ChatProvider, ProviderError, ProviderErrorKind

from . import context as request_context

logger = logging.getLogger("flowmind.ai.chat")


class ChatService(chat_pb2_grpc.ChatServiceServicer):
    def __init__(self, provider: ChatProvider, model_profile: str = "default") -> None:
        self._provider = provider
        self._model_profile = model_profile

    async def Complete(self, request: chat_pb2.CompleteRequest, grpc_context):
        request_context.set_request_context(request.context.request_id, request.context.trace_id)
        started = time.perf_counter()
        try:
            _validate_request(request, self._model_profile)
            result = await self._provider.complete(
                request.context,
                request.messages,
                request.max_output_tokens,
                request.temperature,
            )
        except ProviderError as exc:
            latency_ms = _elapsed_ms(started)
            logger.exception(
                "provider call failed provider=%s kind=%s latency_ms=%d",
                self._provider.name,
                exc.kind,
                latency_ms,
            )
            await grpc_context.abort(
                _status_for_provider_error(exc.kind), _detail_for_provider_error(exc.kind)
            )
        except ValueError as exc:
            logger.warning("invalid chat completion request: %s", exc)
            await grpc_context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, "invalid chat completion request"
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
            message=result.message,
            model_name=result.model_name,
            usage=common_pb2.TokenUsage(
                prompt_tokens=result.prompt_tokens,
                completion_tokens=result.completion_tokens,
                total_tokens=result.prompt_tokens + result.completion_tokens,
            ),
            latency_ms=latency_ms,
        )

    async def CompleteStream(self, request: chat_pb2.CompleteStreamRequest, grpc_context):
        # 阶段 1.9 只实现最小 Chat 方法（Complete）；流式后续阶段再补。
        await grpc_context.abort(
            grpc.StatusCode.UNIMPLEMENTED, "CompleteStream not implemented yet"
        )


def _elapsed_ms(started: float) -> int:
    return int((time.perf_counter() - started) * 1000)


def _validate_request(request: chat_pb2.CompleteRequest, allowed_profile: str) -> None:
    if request.context.model_profile != allowed_profile:
        raise ValueError("unsupported model profile")
    if not request.messages:
        raise ValueError("messages must not be empty")
    if request.max_output_tokens <= 0:
        raise ValueError("max_output_tokens must be positive")
    if not 0 <= request.temperature <= 2:
        raise ValueError("temperature must be between 0 and 2")


def _status_for_provider_error(kind: ProviderErrorKind) -> grpc.StatusCode:
    return {
        ProviderErrorKind.CONFIGURATION: grpc.StatusCode.FAILED_PRECONDITION,
        ProviderErrorKind.UNAVAILABLE: grpc.StatusCode.UNAVAILABLE,
        ProviderErrorKind.TIMEOUT: grpc.StatusCode.DEADLINE_EXCEEDED,
        ProviderErrorKind.RESPONSE: grpc.StatusCode.INTERNAL,
    }[kind]


def _detail_for_provider_error(kind: ProviderErrorKind) -> str:
    return {
        ProviderErrorKind.CONFIGURATION: "AI provider configuration error",
        ProviderErrorKind.UNAVAILABLE: "AI provider unavailable",
        ProviderErrorKind.TIMEOUT: "AI provider timed out",
        ProviderErrorKind.RESPONSE: "AI provider error",
    }[kind]
