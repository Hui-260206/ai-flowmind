"""ChatProvider / FakeProvider 与 Provider 工厂的最小测试。"""

from __future__ import annotations

import asyncio

import pytest
from ai.v1 import common_pb2

from app.config import Settings
from providers import build_provider
from providers.base import ProviderError, ProviderErrorKind
from providers.fake import FakeProvider
from providers.hy3 import HY3Provider, _completion_url, _parse_completion


def _user_message(text: str) -> common_pb2.ChatMessage:
    return common_pb2.ChatMessage(
        role=common_pb2.MESSAGE_ROLE_USER,
        content=[common_pb2.ContentPart(text=common_pb2.TextPart(text=text))],
    )


async def test_fake_provider_returns_fixed_reply():
    provider = FakeProvider()
    result = await provider.complete(
        common_pb2.RequestContext(model_profile="default"),
        [_user_message("hello")],
        max_output_tokens=100,
        temperature=0.0,
    )
    assert result.model_name == "fake"
    assert result.message.role == common_pb2.MESSAGE_ROLE_ASSISTANT
    assert result.message.status == common_pb2.MESSAGE_STATUS_COMPLETED
    assert "fake provider" in result.message.content[0].text.text


def test_build_provider_fake():
    assert isinstance(build_provider(Settings()), FakeProvider)


def test_provider_error_carries_stable_kind():
    with pytest.raises(ProviderError) as exc:
        raise ProviderError(ProviderErrorKind.CONFIGURATION, "unsupported AI provider")
    assert exc.value.kind == ProviderErrorKind.CONFIGURATION


def test_hy3_provider_requires_all_sensitive_configuration():
    with pytest.raises(ProviderError) as exc:
        HY3Provider(base_url="", api_key="", model_name="", timeout_seconds=1)
    assert exc.value.kind == ProviderErrorKind.CONFIGURATION


def test_hy3_url_and_completion_response_are_openai_compatible():
    assert _completion_url("https://example.test") == "https://example.test/v1/chat/completions"
    assert _completion_url("https://example.test/v1") == "https://example.test/v1/chat/completions"
    result = _parse_completion(
        {
            "model": "hy3",
            "choices": [{"message": {"content": "真实模型回复"}}],
            "usage": {"prompt_tokens": 3, "completion_tokens": 5},
        },
        "configured-model",
    )
    assert result.message.content[0].text.text == "真实模型回复"
    assert result.model_name == "hy3"
    assert result.prompt_tokens == 3
    assert result.completion_tokens == 5


async def test_hy3_request_is_cancelable(monkeypatch):
    started = asyncio.Event()
    cancelled = asyncio.Event()

    class _BlockingClient:
        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

        async def post(self, *args, **kwargs):
            started.set()
            try:
                await asyncio.Event().wait()
            except asyncio.CancelledError:
                cancelled.set()
                raise

    monkeypatch.setattr("providers.hy3.httpx.AsyncClient", lambda **kwargs: _BlockingClient())
    provider = HY3Provider(
        base_url="https://example.test/v1", api_key="test-key", model_name="hy3", timeout_seconds=1
    )
    task = asyncio.create_task(
        provider.complete(
            common_pb2.RequestContext(),
            [_user_message("hello")],
            max_output_tokens=1,
            temperature=0,
        )
    )
    await started.wait()
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task
    assert cancelled.is_set()
