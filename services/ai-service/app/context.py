"""请求级上下文：跨异步调用透传 ``request_id`` / ``trace_id``。

约定（与 Go API 的 ``X-Request-ID`` 语义保持一致）：

- ``request_id`` 来自 gRPC 请求 ``RequestContext.request_id``（见
  ``services/proto/ai/v1/common.proto``）。
- 调用方未提供时，由服务端在入口生成 ``uuid4().hex``。
- 这些 ``contextvars`` 由阶段 1.9 的 gRPC interceptor 在每次 RPC 前设置、
  RPC 结束后清理；本阶段只定义载体和读写约定，供日志过滤器读取。
"""

from __future__ import annotations

import contextvars
import uuid

request_id_var: contextvars.ContextVar[str] = contextvars.ContextVar("request_id", default="")
trace_id_var: contextvars.ContextVar[str] = contextvars.ContextVar("trace_id", default="")


def new_request_id() -> str:
    """生成服务端 request_id（无横线的 32 位十六进制）。"""
    return uuid.uuid4().hex


def set_request_context(request_id: str = "", trace_id: str = "") -> None:
    """写入当前异步链的请求上下文，缺失 request_id 时自动生成。"""
    request_id_var.set(request_id or new_request_id())
    trace_id_var.set(trace_id)


def clear_request_context() -> None:
    """清空当前异步链的请求上下文，避免跨请求串用。"""
    request_id_var.set("")
    trace_id_var.set("")


def get_request_id() -> str:
    return request_id_var.get()


def get_trace_id() -> str:
    return trace_id_var.get()
