"""AI 服务统一启动入口。

职责：加载配置 → 初始化结构化日志 → 启动 gRPC 生命周期。

本服务不提供 FastAPI / Uvicorn / HTTP 健康检查入口；gRPC 只监听内部网络
地址，不对公网开放。
"""

from __future__ import annotations

import asyncio
import logging
import sys
from pathlib import Path

from pydantic import ValidationError

# 在导入依赖 ai.v1 生成代码的模块之前，先把生成目录加入 sys.path。路径基于
# 本文件定位，与工作目录无关；测试场景由 pytest 的 pythonpath 配置覆盖。
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "gen" / "python"))

from . import grpc_server, logging_config
from .config import load_settings

SERVICE_NAME = "flowmind-ai-service"
SERVICE_VERSION = "0.1.0"


def main() -> None:
    try:
        settings = load_settings()
    except ValidationError as exc:
        print(f"configuration error: {exc}", file=sys.stderr)
        sys.exit(1)

    logging_config.configure_logging(settings.log_level)
    logger = logging.getLogger("flowmind.ai")

    logger.info(
        "starting %s v%s env=%s grpc_addr=%s provider=%s model_profile=%s",
        SERVICE_NAME,
        SERVICE_VERSION,
        settings.environment,
        settings.grpc_listen_addr,
        settings.provider,
        settings.model_profile,
    )

    try:
        asyncio.run(grpc_server.serve(settings))
    except KeyboardInterrupt:
        pass
    except Exception:
        logger.exception("service failed to run")
        sys.exit(1)

    logger.info("service stopped")


if __name__ == "__main__":
    main()
