"""FakeProvider：不依赖真实模型的固定回复实现，用于联通测试。"""

from __future__ import annotations

import logging
from collections.abc import Sequence

from ai.v1 import common_pb2

from .base import ChatProvider, ProviderResult

logger = logging.getLogger("flowmind.ai.provider.fake")

_REPLY_TEXT = "This is a fixed response from the FlowMind fake provider."


class FakeProvider(ChatProvider):
    name = "fake"

    async def complete(
        self,
        context: common_pb2.RequestContext,
        messages: Sequence[common_pb2.ChatMessage],
        max_output_tokens: int,
        temperature: float,
    ) -> ProviderResult:
        logger.info(
            "fake provider called model_profile=%s messages=%d max_output_tokens=%d temperature=%s",
            context.model_profile,
            len(messages),
            max_output_tokens,
            temperature,
        )
        return ProviderResult(message=_assistant_message(_REPLY_TEXT), model_name=self.name)


def _assistant_message(text: str) -> common_pb2.ChatMessage:
    return common_pb2.ChatMessage(
        role=common_pb2.MESSAGE_ROLE_ASSISTANT,
        status=common_pb2.MESSAGE_STATUS_COMPLETED,
        content=[common_pb2.ContentPart(text=common_pb2.TextPart(text=text))],
    )
