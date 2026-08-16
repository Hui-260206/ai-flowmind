"""ChatProvider 抽象：模型调用边界的统一接口。

真实 Provider（1.9 之后接入）只负责「输入消息 → 输出助手消息」，不感知
gRPC、会话业务或数据库；错误以异常向上抛出，由 ChatService 统一记录并
转换为 gRPC 状态码。
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Sequence

from ai.v1 import common_pb2


class ProviderError(Exception):
    """Provider 调用失败。ChatService 会将其转换为 gRPC INTERNAL。"""


class ChatProvider(ABC):
    """把一组对话消息转换为助手回复消息。"""

    name: str = ""

    @abstractmethod
    async def complete(
        self,
        context: common_pb2.RequestContext,
        messages: Sequence[common_pb2.ChatMessage],
        max_output_tokens: int,
        temperature: float,
    ) -> common_pb2.ChatMessage:
        """返回助手回复消息。"""
