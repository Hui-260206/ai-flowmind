"""HY3 的 OpenAI Chat Completions 兼容 Provider。"""

from __future__ import annotations

import json
from collections.abc import Sequence

import httpx
from ai.v1 import common_pb2

from .base import ChatProvider, ProviderError, ProviderErrorKind, ProviderResult


class HY3Provider(ChatProvider):
    """通过标准 OpenAI-compatible HTTP 协议调用 HY3。"""

    name = "hy3"

    def __init__(
        self, base_url: str, api_key: str, model_name: str, timeout_seconds: float
    ) -> None:
        self._base_url = _completion_url(base_url)
        self._api_key = api_key.strip()
        self._model_name = model_name.strip()
        self._timeout_seconds = timeout_seconds
        if not self._base_url or not self._api_key or not self._model_name:
            raise ProviderError(
                ProviderErrorKind.CONFIGURATION,
                "HY3 provider requires base URL, API key, and model name",
            )

    async def complete(
        self,
        context: common_pb2.RequestContext,
        messages: Sequence[common_pb2.ChatMessage],
        max_output_tokens: int,
        temperature: float,
    ) -> ProviderResult:
        del context  # 请求标识由 ChatService 日志上下文处理，不发送给外部 Provider。
        payload = {
            "model": self._model_name,
            "messages": [_to_openai_message(message) for message in messages],
            "max_tokens": max_output_tokens,
            "temperature": temperature,
        }
        response = await self._post(payload)
        return _parse_completion(response, self._model_name)

    async def _post(self, payload: dict[str, object]) -> dict[str, object]:
        try:
            async with httpx.AsyncClient(timeout=self._timeout_seconds) as client:
                response = await client.post(
                    self._base_url,
                    json=payload,
                    headers={
                        "Authorization": f"Bearer {self._api_key}",
                        "Accept": "application/json",
                    },
                )
        except httpx.TimeoutException as exc:
            raise ProviderError(ProviderErrorKind.TIMEOUT, "HY3 provider timed out") from exc
        except httpx.RequestError as exc:
            raise ProviderError(
                ProviderErrorKind.UNAVAILABLE, "HY3 provider is unavailable"
            ) from exc

        if response.status_code >= 400:
            if response.status_code in {408, 429, 500, 502, 503, 504}:
                raise ProviderError(ProviderErrorKind.UNAVAILABLE, "HY3 provider is unavailable")
            raise ProviderError(ProviderErrorKind.RESPONSE, "HY3 provider rejected the request")

        try:
            decoded = response.json()
        except json.JSONDecodeError as exc:
            raise ProviderError(
                ProviderErrorKind.RESPONSE, "HY3 provider returned invalid JSON"
            ) from exc
        if not isinstance(decoded, dict):
            raise ProviderError(
                ProviderErrorKind.RESPONSE, "HY3 provider returned an invalid response"
            )
        return decoded


def _completion_url(base_url: str) -> str:
    normalized = base_url.strip().rstrip("/")
    if not normalized:
        return ""
    if normalized.endswith("/chat/completions"):
        return normalized
    if normalized.endswith("/v1"):
        return f"{normalized}/chat/completions"
    return f"{normalized}/v1/chat/completions"


def _to_openai_message(message: common_pb2.ChatMessage) -> dict[str, str]:
    role = common_pb2.MessageRole.Name(message.role).removeprefix("MESSAGE_ROLE_").lower()
    if role not in {"system", "user", "assistant", "tool"}:
        raise ProviderError(ProviderErrorKind.RESPONSE, "unsupported message role")
    text = "".join(part.text.text for part in message.content if part.HasField("text"))
    if not text:
        raise ProviderError(ProviderErrorKind.RESPONSE, "message must contain text content")
    return {"role": role, "content": text}


def _parse_completion(payload: dict[str, object], configured_model_name: str) -> ProviderResult:
    try:
        choices = payload["choices"]
        choice = choices[0]  # type: ignore[index]
        text = choice["message"]["content"]  # type: ignore[index]
    except (KeyError, IndexError, TypeError) as exc:
        raise ProviderError(
            ProviderErrorKind.RESPONSE, "HY3 provider returned no assistant message"
        ) from exc
    if not isinstance(text, str) or not text.strip():
        raise ProviderError(
            ProviderErrorKind.RESPONSE, "HY3 provider returned empty assistant content"
        )
    usage = payload.get("usage", {})
    if not isinstance(usage, dict):
        usage = {}
    prompt_tokens = _non_negative_int(usage.get("prompt_tokens"))
    completion_tokens = _non_negative_int(usage.get("completion_tokens"))
    model_name = payload.get("model")
    if not isinstance(model_name, str) or not model_name.strip():
        model_name = configured_model_name
    return ProviderResult(
        message=common_pb2.ChatMessage(
            role=common_pb2.MESSAGE_ROLE_ASSISTANT,
            status=common_pb2.MESSAGE_STATUS_COMPLETED,
            content=[common_pb2.ContentPart(text=common_pb2.TextPart(text=text))],
        ),
        model_name=model_name,
        prompt_tokens=prompt_tokens,
        completion_tokens=completion_tokens,
    )


def _non_negative_int(value: object) -> int:
    return value if isinstance(value, int) and value >= 0 else 0
