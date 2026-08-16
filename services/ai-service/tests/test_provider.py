"""ChatProvider / FakeProvider 与 Provider 工厂的最小测试。"""

from __future__ import annotations

import pytest
from ai.v1 import common_pb2

from providers import build_provider
from providers.base import ProviderError
from providers.fake import FakeProvider


def _user_message(text: str) -> common_pb2.ChatMessage:
    return common_pb2.ChatMessage(
        role=common_pb2.MESSAGE_ROLE_USER,
        content=[common_pb2.ContentPart(text=common_pb2.TextPart(text=text))],
    )


async def test_fake_provider_returns_fixed_reply():
    provider = FakeProvider()
    message = await provider.complete(
        common_pb2.RequestContext(model_profile="default"),
        [_user_message("hello")],
        max_output_tokens=100,
        temperature=0.0,
    )
    assert message.role == common_pb2.MESSAGE_ROLE_ASSISTANT
    assert message.status == common_pb2.MESSAGE_STATUS_COMPLETED
    assert "fake provider" in message.content[0].text.text


def test_build_provider_fake():
    assert isinstance(build_provider("fake"), FakeProvider)


def test_build_provider_unknown_raises():
    with pytest.raises(ProviderError):
        build_provider("openai")
