"""结构化日志：单行 JSON 输出到 stdout，字段对齐 Go 的 ``log/slog``。

不引入 structlog / python-json-logger，保持依赖最小。由标准库 ``logging``
的 Filter 注入 ``request_id``/``trace_id``，Formatter 统一序列化为 JSON。
"""

from __future__ import annotations

import json
import logging
import sys
from datetime import UTC, datetime

from . import context

_LOG_LEVELS = {
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warning": logging.WARNING,
    "error": logging.ERROR,
}


class RequestContextFilter(logging.Filter):
    """把当前异步链上的 request_id/trace_id 注入每条日志。"""

    def filter(self, record: logging.LogRecord) -> bool:
        record.request_id = context.get_request_id()
        record.trace_id = context.get_trace_id()
        return True


class JsonFormatter(logging.Formatter):
    """把日志记录格式化为单行 JSON，字段命名与 Go 侧 slog 保持一致。"""

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, object] = {
            "time": datetime.now(UTC).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "msg": record.getMessage(),
        }
        request_id = getattr(record, "request_id", "") or ""
        trace_id = getattr(record, "trace_id", "") or ""
        if request_id:
            payload["request_id"] = request_id
        if trace_id:
            payload["trace_id"] = trace_id
        if record.exc_info:
            payload["error"] = self.formatException(record.exc_info)
        return json.dumps(payload, ensure_ascii=False)


def configure_logging(level: str) -> None:
    """按级别初始化根 logger，输出单行 JSON 到 stdout。"""
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JsonFormatter())
    handler.addFilter(RequestContextFilter())

    root = logging.getLogger()
    root.handlers = [handler]
    root.setLevel(_LOG_LEVELS.get(level, logging.INFO))

    # grpcio 内部日志较吵，默认压到 WARNING，避免淹没业务日志。
    logging.getLogger("grpc").setLevel(logging.WARNING)
