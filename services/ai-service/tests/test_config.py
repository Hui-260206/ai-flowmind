"""config 模块的最小测试：环境变量读取与非法配置报错。"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from app.config import Settings

_ALL_ENV_VARS = (
    "AI_ENV",
    "AI_LOG_LEVEL",
    "AI_GRPC_LISTEN_ADDR",
    "AI_PROVIDER",
    "AI_PROVIDER_BASE_URL",
    "AI_PROVIDER_API_KEY",
    "AI_MODEL",
    "AI_PROVIDER_TIMEOUT",
    "MODEL_PROFILE",
)


@pytest.fixture(autouse=True)
def _clean_env(monkeypatch):
    for name in _ALL_ENV_VARS:
        monkeypatch.delenv(name, raising=False)


def test_defaults_when_no_environment():
    settings = Settings()
    assert settings.environment == "local"
    assert settings.log_level == "info"
    assert settings.grpc_listen_addr == "127.0.0.1:50051"
    assert settings.provider == "fake"
    assert settings.provider_base_url == ""
    assert settings.provider_api_key == ""
    assert settings.model_name == "hy3"
    assert settings.provider_timeout_seconds == 12.0
    assert settings.model_profile == "default"


def test_reads_environment_variables(monkeypatch):
    monkeypatch.setenv("AI_GRPC_LISTEN_ADDR", "0.0.0.0:50051")
    monkeypatch.setenv("AI_LOG_LEVEL", "debug")
    monkeypatch.setenv("AI_PROVIDER", "fake")
    settings = Settings()
    assert settings.grpc_listen_addr == "0.0.0.0:50051"
    assert settings.log_level == "debug"


def test_reads_hy3_settings(monkeypatch):
    monkeypatch.setenv("AI_PROVIDER", " HY3 ")
    monkeypatch.setenv("AI_PROVIDER_BASE_URL", "https://example.test/v1")
    monkeypatch.setenv("AI_PROVIDER_API_KEY", "test-key")
    monkeypatch.setenv("AI_MODEL", "hy3")
    monkeypatch.setenv("AI_PROVIDER_TIMEOUT", "3.5")
    settings = Settings()
    assert settings.provider == "hy3"
    assert settings.model_name == "hy3"
    assert settings.provider_timeout_seconds == 3.5


def test_normalizes_model_profile(monkeypatch):
    monkeypatch.setenv("MODEL_PROFILE", " default ")
    assert Settings().model_profile == "default"


def test_ignores_unrelated_environment_variables(monkeypatch):
    monkeypatch.setenv("MYSQL_HOST", "127.0.0.1")
    monkeypatch.setenv("REDIS_ADDR", "127.0.0.1:6379")
    settings = Settings()
    assert settings.environment == "local"


def test_rejects_empty_grpc_addr(monkeypatch):
    monkeypatch.setenv("AI_GRPC_LISTEN_ADDR", "")
    with pytest.raises(ValidationError):
        Settings()


def test_rejects_addr_without_port(monkeypatch):
    monkeypatch.setenv("AI_GRPC_LISTEN_ADDR", "127.0.0.1")
    with pytest.raises(ValidationError):
        Settings()


def test_rejects_invalid_log_level(monkeypatch):
    monkeypatch.setenv("AI_LOG_LEVEL", "verbose")
    with pytest.raises(ValidationError):
        Settings()


def test_rejects_provider_timeout_outside_http_budget(monkeypatch):
    monkeypatch.setenv("AI_PROVIDER_TIMEOUT", "15")
    with pytest.raises(ValidationError):
        Settings()
