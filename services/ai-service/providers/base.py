"""ChatProvider 抽象：模型调用边界的统一接口。

真实 Provider（1.9 之后接入）只负责「输入消息 → 输出助手消息」，不感知
gRPC、会话业务或数据库；错误以异常向上抛出，由 ChatService 统一记录并
转换为 gRPC 状态码。
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Sequence
from dataclasses import dataclass
from enum import StrEnum

from ai.v1 import common_pb2


class ProviderErrorKind(StrEnum):
    """Provider 故障的稳定分类，不携带上游服务的敏感细节。"""

    CONFIGURATION = "configuration"
    UNAVAILABLE = "unavailable"
    TIMEOUT = "timeout"
    RESPONSE = "response"


class ProviderError(Exception):
    """Provider 调用失败；ChatService 根据 ``kind`` 映射 gRPC 状态。"""

    def __init__(self, kind: ProviderErrorKind, message: str) -> None:
        super().__init__(message)
        self.kind = kind


@dataclass(frozen=True)
class ProviderResult:
    """一次模型补全的、与具体 Provider 无关的结果。"""

    message: common_pb2.ChatMessage
    model_name: str
    prompt_tokens: int = 0
    completion_tokens: int = 0


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
    ) -> ProviderResult:
        """返回助手回复消息。"""
