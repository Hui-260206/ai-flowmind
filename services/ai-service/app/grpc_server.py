"""gRPC 服务生命周期。

阶段 1.8 搭 create / start / graceful-stop 骨架；阶段 1.9 注册 ChatService。
"""

from __future__ import annotations

import asyncio
import logging
import signal

import grpc
from ai.v1 import chat_pb2_grpc

from providers import build_provider

from .chat_service import ChatService
from .config import Settings

logger = logging.getLogger("flowmind.ai.grpc")


def build_server(settings: Settings) -> grpc.aio.Server:
    """创建 aio server，注册 ChatService 并绑定监听地址。"""
    server = grpc.aio.server()
    chat_service = ChatService(provider=build_provider(settings.provider))
    chat_pb2_grpc.add_ChatServiceServicer_to_server(chat_service, server)
    port = server.add_insecure_port(settings.grpc_listen_addr)
    if port == 0:
        raise RuntimeError(f"failed to bind gRPC address {settings.grpc_listen_addr!r}")
    return server


async def serve(settings: Settings) -> None:
    """启动 server，等待退出信号，随后优雅停止。"""
    server = build_server(settings)
    await server.start()
    logger.info(
        "gRPC server started on %s provider=%s",
        settings.grpc_listen_addr,
        settings.provider,
    )

    stop_event = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, stop_event.set)
        except (NotImplementedError, RuntimeError):
            # 非主线程或不支持 signal 的平台回退为键盘中断。
            pass

    try:
        await stop_event.wait()
        logger.info("shutdown signal received")
    finally:
        await server.stop(grace=5)
        logger.info("gRPC server stopped")
