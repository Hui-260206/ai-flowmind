"""Provider 抽象与实现。

阶段 1.9 提供 ChatProvider 接口与 FakeProvider 实现。模型 Provider 的
选择、URL 和密钥只进入本服务，不对移动端或 Go API 暴露。
"""

from .base import ChatProvider, ProviderError
from .fake import FakeProvider

__all__ = ["ChatProvider", "FakeProvider", "ProviderError", "build_provider"]


def build_provider(name: str) -> ChatProvider:
    """按名称构造 Provider。未知名称抛 ProviderError，由启动流程清晰报错。"""
    if name == FakeProvider.name:
        return FakeProvider()
    raise ProviderError(f"unsupported provider {name!r}")
