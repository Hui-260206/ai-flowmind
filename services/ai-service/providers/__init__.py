"""Provider 抽象与实现。

阶段 1.9 提供 ChatProvider 接口与 FakeProvider 实现。模型 Provider 的
选择、URL 和密钥只进入本服务，不对移动端或 Go API 暴露。
"""

from app.config import Settings

from .base import ChatProvider, ProviderError, ProviderErrorKind
from .fake import FakeProvider
from .hy3 import HY3Provider

__all__ = [
    "ChatProvider",
    "FakeProvider",
    "HY3Provider",
    "ProviderError",
    "ProviderErrorKind",
    "build_provider",
]


def build_provider(settings: Settings) -> ChatProvider:
    """按已校验配置构造 Provider；密钥始终停留在 Python 服务。"""
    if settings.provider == FakeProvider.name:
        return FakeProvider()
    if settings.provider == HY3Provider.name:
        return HY3Provider(
            base_url=settings.provider_base_url,
            api_key=settings.provider_api_key,
            model_name=settings.model_name,
            timeout_seconds=settings.provider_timeout_seconds,
        )
    raise ProviderError(ProviderErrorKind.CONFIGURATION, "unsupported AI provider")
