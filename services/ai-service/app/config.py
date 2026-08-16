"""配置模块：从进程环境变量读取 AI 服务配置。

与 Go API 保持一致，本服务只读取进程环境变量，不自动搜索或解析 ``.env``：

- 本地开发由 ``make dev-ai`` 显式加载 ``services/.env`` 后注入；
- 生产环境由进程管理器或容器注入环境变量。

模型 Provider 的 URL 与密钥只允许出现在本服务，不进入 Go API。
"""

from __future__ import annotations

from pydantic import Field, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

_LOG_LEVELS = ("debug", "info", "warning", "error")


class Settings(BaseSettings):
    """AI 服务配置，字段与 ``services/.env.example`` 中的环境变量一一对应。"""

    model_config = SettingsConfigDict(extra="ignore", populate_by_name=True)

    environment: str = Field(default="local", validation_alias="AI_ENV")
    log_level: str = Field(default="info", validation_alias="AI_LOG_LEVEL")
    grpc_listen_addr: str = Field(
        default="127.0.0.1:50051", validation_alias="AI_GRPC_LISTEN_ADDR"
    )
    provider: str = Field(default="fake", validation_alias="AI_PROVIDER")
    provider_base_url: str = Field(default="", validation_alias="AI_PROVIDER_BASE_URL")
    provider_api_key: str = Field(default="", validation_alias="AI_PROVIDER_API_KEY")
    model_profile: str = Field(default="default", validation_alias="MODEL_PROFILE")

    @field_validator("log_level")
    @classmethod
    def _normalize_log_level(cls, value: str) -> str:
        level = value.strip().lower()
        if level not in _LOG_LEVELS:
            raise ValueError(f"log level must be one of {_LOG_LEVELS}, got {value!r}")
        return level

    @field_validator("grpc_listen_addr")
    @classmethod
    def _validate_grpc_listen_addr(cls, value: str) -> str:
        address = value.strip()
        if not address:
            raise ValueError("gRPC listen address must not be empty")
        _validate_port(address)
        return address

    @field_validator("provider")
    @classmethod
    def _normalize_provider(cls, value: str) -> str:
        provider = value.strip().lower()
        if not provider:
            raise ValueError("provider must not be empty")
        return provider


def _validate_port(address: str) -> None:
    """校验监听地址包含合法端口（1..65535），host 部分交由 grpc 在绑定时校验。"""
    if any(char.isspace() for char in address):
        raise ValueError("gRPC listen address must not contain whitespace")
    last_colon = address.rfind(":")
    if last_colon < 0:
        raise ValueError("gRPC listen address must include a port, e.g. 127.0.0.1:50051")
    port_text = address[last_colon + 1 :]
    try:
        port = int(port_text)
    except ValueError as exc:
        raise ValueError("gRPC listen address port must be an integer from 1 to 65535") from exc
    if not 1 <= port <= 65535:
        raise ValueError("gRPC listen address port must be an integer from 1 to 65535")


def load_settings() -> Settings:
    """从进程环境变量加载并校验配置。非法配置抛 ``pydantic.ValidationError``。"""
    return Settings()
