#!/usr/bin/env python3
"""
Upload exported CodeBuddy markdown to ChatSpark via Supabase Edge Function.

Flow (default):
1) call functions/v1/upload-session-relay
2) function verifies API key, uploads full markdown to bucket, and writes chat_sessions metadata

No third-party dependency (urllib only).
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import re
import socket
import ssl
import sys
from pathlib import Path
from typing import Any
from urllib import error, request

_CA_BUNDLE_PATHS = [
    "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",  # RHEL/CentOS 7+
    "/etc/ssl/certs/ca-certificates.crt",                  # Debian/Ubuntu
    "/etc/pki/tls/certs/ca-bundle.crt",                    # RHEL 6
]


def find_working_ca(host: str = "baasapi.qq.com", port: int = 443) -> ssl.SSLContext | None:
    for p in _CA_BUNDLE_PATHS:
        if not Path(p).is_file():
            continue
        ctx = ssl.create_default_context(cafile=p)
        try:
            with socket.create_connection((host, port), timeout=5) as sock:
                with ctx.wrap_socket(sock, server_hostname=host):
                    return ctx
        except Exception:
            continue
    return None


def make_ssl_context() -> ssl.SSLContext | None:
    ca = (
        os.environ.get("CHATS_CA_BUNDLE", "").strip()
        or os.environ.get("SSL_CERT_FILE", "").strip()
    )
    if ca and Path(ca).exists():
        return ssl.create_default_context(cafile=ca)
    return None

DEFAULT_RELAY_URL = "https://baasapi.qq.com/p/lrs3knwhh33tksuymgjc/functions/v1/upload-session-relay"

TOKEN_FILENAME = "chats_token"
DEFAULT_TOKEN_DIR = Path.home() / ".chats"
DEFAULT_TOKEN_FILE = DEFAULT_TOKEN_DIR / TOKEN_FILENAME

ALLOWED_VISIBILITIES = ("public", "private")
DEFAULT_VISIBILITY = "public"
PLATFORM_TAGS = ("前端", "后端", "客户端", "通用", "搞笑", "其他")
DEFAULT_PLATFORM_TAG = "通用"
TAG_MAX_COUNT = 4


def normalize_visibility(raw: str) -> str:
    value = (raw or "").strip().lower()
    if value in ALLOWED_VISIBILITIES:
        return value
    return DEFAULT_VISIBILITY


def eprint(*args: Any) -> None:
    print(*args, file=sys.stderr)


def resolve_relay_url(cli_relay_url: str, disable_relay: bool) -> str:
    if disable_relay:
        return ""
    if cli_relay_url.strip():
        return cli_relay_url.strip()

    env_url = os.environ.get("CHATS_UPLOAD_RELAY_URL", "").strip()
    if env_url:
        return env_url

    return DEFAULT_RELAY_URL


def resolve_token_file(cli_token_file: str | None) -> Path:
    """按优先级解析 token 文件路径（不读取内容）：
    1) CLI --token-file（最高）
    2) 环境变量 CHATS_HOME → $CHATS_HOME/chats_token
    3) 默认路径 ~/.chats/chats_token
    """
    if cli_token_file:
        return Path(cli_token_file).expanduser()

    home_env = os.environ.get("CHATS_HOME", "").strip()
    if home_env:
        return Path(home_env).expanduser() / TOKEN_FILENAME

    return DEFAULT_TOKEN_FILE


def load_token(token_file: Path) -> str:
    token = os.environ.get("CHATS_TOKEN", "").strip()
    if token:
        return token

    if not token_file.exists():
        raise RuntimeError(f"token 文件不存在: {token_file}")

    token = token_file.read_text(encoding="utf-8").strip()
    if not token:
        raise RuntimeError(f"token 文件为空: {token_file}")

    return token


def post_json(
    url: str,
    payload: dict[str, Any],
    apikey: str = "",
    timeout: int = 30,
    ssl_context: ssl.SSLContext | None = None,
) -> tuple[int, Any, str]:
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    headers: dict[str, str] = {
        "Content-Type": "application/json",
        "Accept": "application/json",
    }
    if apikey:
        headers["apikey"] = apikey
        headers["Authorization"] = f"Bearer {apikey}"

    req = request.Request(url=url, data=body, headers=headers, method="POST")

    try:
        with request.urlopen(req, timeout=timeout, context=ssl_context) as resp:
            status = int(resp.getcode())
            raw = resp.read().decode("utf-8", errors="replace")
    except error.HTTPError as ex:
        status = int(ex.code)
        raw = ex.read().decode("utf-8", errors="replace")
    except ssl.SSLError as ex:
        return 0, None, f"[SSL_ERROR] {ex}"
    except Exception as ex:
        return 0, None, str(ex)

    try:
        parsed = json.loads(raw) if raw else {}
    except Exception:
        parsed = None

    return status, parsed, raw


def infer_title(md_text: str, md_file: Path) -> str:
    for line in md_text.splitlines():
        line = line.strip()
        if line.startswith("# "):
            title = line[2:].strip()
            if title:
                return title[:200]
    return md_file.stem[:200] if md_file.stem else "未命名对话"


def infer_summary(md_text: str) -> str:
    lines = [ln.strip() for ln in md_text.splitlines()]
    chunks: list[str] = []
    for ln in lines:
        if not ln:
            continue
        if ln.startswith("#") or ln.startswith(">") or ln.startswith("<!--") or ln.startswith("```"):
            continue
        if ln == "## User" or ln.startswith("## Assistant"):
            continue

        chunks.append(ln)
        if len(" ".join(chunks)) >= 220:
            break

    summary = " ".join(chunks).strip()
    if not summary:
        summary = "由 CodeBuddy 自动上传的历史会话"
    return summary[:500]


def infer_platform_tag_by_rules(title: str, summary: str, md_text: str) -> str:
    corpus = f"{title} {summary} {_build_tagging_context(md_text, max_chars=1200)}".lower()

    mapping: list[tuple[tuple[str, ...], str]] = [
        (("搞笑", "段子", "玩笑", "meme", "梗", "joke"), "搞笑"),
        (("ios", "android", "移动端", "小程序", "客户端", "app", "flutter", "react native", "rn", "uniapp"), "客户端"),
        (("后端", "backend", "api", "server", "服务端", "数据库", "mysql", "postgres", "sql", "go", "java", "django", "flask", "spring"), "后端"),
        (("前端", "frontend", "react", "vue", "next", "html", "css", "javascript", "typescript", "js", "ts", "web"), "前端"),
    ]

    for keys, platform_tag in mapping:
        if any(k in corpus for k in keys):
            return platform_tag

    if len(corpus.strip()) < 20:
        return "其他"
    return DEFAULT_PLATFORM_TAG


def apply_tag_policy(candidates: list[str], title: str, summary: str, md_text: str) -> list[str]:
    normalized = _normalize_and_dedup_tags(candidates, limit=12)

    platform_tags = [tag for tag in normalized if tag in PLATFORM_TAGS]
    free_tags = [tag for tag in normalized if tag not in PLATFORM_TAGS]

    primary_platform_tag = platform_tags[0] if platform_tags else infer_platform_tag_by_rules(title=title, summary=summary, md_text=md_text)

    final_tags: list[str] = [primary_platform_tag]
    for tag in free_tags:
        if len(final_tags) >= TAG_MAX_COUNT:
            break
        final_tags.append(tag)

    return _normalize_and_dedup_tags(final_tags, limit=TAG_MAX_COUNT)


def _normalize_and_dedup_tags(tags: list[str], limit: int = 6) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for raw in tags:
        tag = str(raw).strip()
        if not tag:
            continue
        tag = re.sub(r"\s+", " ", tag)
        tag = tag.strip("#，,;；。.!！?？")
        if not tag:
            continue
        key = tag.casefold()
        if key in seen:
            continue
        seen.add(key)
        out.append(tag[:24])
        if len(out) >= limit:
            break
    return out


def _build_tagging_context(md_text: str, max_chars: int = 3500) -> str:
    text = re.sub(r"```[\s\S]*?```", " ", md_text)
    text = re.sub(r"<!--[\s\S]*?-->", " ", text)
    lines: list[str] = []
    for ln in text.splitlines():
        s = ln.strip()
        if not s:
            continue
        if s.startswith("#") or s.startswith(">"):
            s = s.lstrip("#>").strip()
        if s:
            lines.append(s)
    merged = re.sub(r"\s+", " ", " ".join(lines)).strip()
    return merged[:max_chars]


def _extract_tags_from_ai_response(parsed: Any, raw_text: str) -> list[str]:
    if isinstance(parsed, dict):
        direct = parsed.get("tags")
        if isinstance(direct, list):
            return _normalize_and_dedup_tags([str(x) for x in direct])

        choices = parsed.get("choices")
        if isinstance(choices, list) and choices:
            first = choices[0] if isinstance(choices[0], dict) else {}
            message = first.get("message") if isinstance(first, dict) else {}
            content = message.get("content") if isinstance(message, dict) else ""
            if isinstance(content, str) and content.strip():
                raw_text = content

    text = (raw_text or "").strip()
    if not text:
        return []

    fence_match = re.search(r"```(?:json)?\s*([\s\S]*?)\s*```", text, flags=re.IGNORECASE)
    if fence_match:
        text = fence_match.group(1).strip()

    if text.startswith("["):
        try:
            arr = json.loads(text)
            if isinstance(arr, list):
                return _normalize_and_dedup_tags([str(x) for x in arr])
        except Exception:
            pass

    obj_match = re.search(r"\{[\s\S]*\}", text)
    if obj_match:
        try:
            obj = json.loads(obj_match.group(0))
            if isinstance(obj, dict) and isinstance(obj.get("tags"), list):
                return _normalize_and_dedup_tags([str(x) for x in obj["tags"]])
        except Exception:
            pass

    return _normalize_and_dedup_tags([x.strip() for x in re.split(r"[,，\n]", text) if x.strip()])


def infer_tags_with_ai(title: str, summary: str, md_text: str) -> list[str]:
    """使用可配置的 OpenAI 兼容接口生成标签。"""
    ai_url = os.environ.get("CHATS_TAGS_AI_URL", "").strip()
    ai_token = os.environ.get("CHATS_TAGS_AI_TOKEN", "").strip()
    ai_model = os.environ.get("CHATS_TAGS_AI_MODEL", "gpt-4o-mini").strip() or "gpt-4o-mini"

    if not ai_url or not ai_token:
        return []

    context = _build_tagging_context(md_text)
    if not context:
        context = f"标题：{title}\n摘要：{summary}"

    system_prompt = (
        "你是会话分类助手。请根据会话内容生成 1-4 个中文标签。"
        "标签需要简短、可复用；并且至少包含一个平台预设标签："
        "前端/后端/客户端/通用/搞笑/其他。"
        "除预设标签外，也可以输出更细粒度的自由标签。"
    )
    user_prompt = (
        f"标题：{title}\n"
        f"摘要：{summary}\n"
        f"会话正文片段：{context}\n\n"
        "请只输出 JSON：{\"tags\":[\"标签1\",\"标签2\"]}，不要输出其它解释。"
    )

    payload = {
        "model": ai_model,
        "temperature": 0.2,
        "messages": [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": user_prompt},
        ],
    }

    status, parsed, raw = post_json(ai_url, payload, apikey=ai_token, timeout=45)
    if status != 200:
        eprint(f"[Tags] AI 接口返回异常，HTTP {status}")
        return []

    tags = _extract_tags_from_ai_response(parsed, raw)
    return _normalize_and_dedup_tags(tags)


def infer_tags_by_rules(title: str, summary: str, md_text: str) -> list[str]:
    """AI 不可用时的轻量兜底策略，尽量避免固定标签。"""
    corpus = f"{title} {summary} {_build_tagging_context(md_text, max_chars=1200)}".lower()

    keyword_map: list[tuple[tuple[str, ...], str]] = [
        (("python", "py"), "Python"),
        (("typescript", "ts", "javascript", "js"), "前端开发"),
        (("react", "vue", "next.js", "nextjs"), "Web开发"),
        (("sql", "mysql", "postgres", "数据库"), "数据库"),
        (("bug", "报错", "异常", "修复"), "问题排查"),
        (("性能", "优化", "performance"), "性能优化"),
        (("部署", "deploy", "上线", "发布"), "部署发布"),
        (("测试", "test", "单测", "e2e"), "测试"),
        (("重构", "refactor"), "代码重构"),
        (("文档", "readme", "skill"), "文档"),
    ]

    tags: list[str] = []
    for keys, tag in keyword_map:
        if any(k in corpus for k in keys):
            tags.append(tag)

    if not tags:
        if title.strip():
            tags.append(title.strip()[:12])
        elif summary.strip():
            tags.append(summary.strip()[:12])

    return _normalize_and_dedup_tags(tags)


def resolve_tags(title: str, summary: str, md_text: str) -> list[str]:
    ai_tags = infer_tags_with_ai(title=title, summary=summary, md_text=md_text)
    if ai_tags:
        eprint(f"[Tags] AI 自动生成候选标签: {', '.join(ai_tags)}")

    fallback_tags = infer_tags_by_rules(title=title, summary=summary, md_text=md_text)
    if fallback_tags and not ai_tags:
        eprint(f"[Tags] AI 不可用，使用规则候选标签: {', '.join(fallback_tags)}")

    final_tags = apply_tag_policy(
        candidates=ai_tags + fallback_tags,
        title=title,
        summary=summary,
        md_text=md_text,
    )

    if not final_tags:
        final_tags = [DEFAULT_PLATFORM_TAG]

    eprint(f"[Tags] 最终标签（1-4，含平台标签）: {', '.join(final_tags)}")
    return final_tags


def upload_via_relay(
    relay_url: str,
    token: str,
    md_file: Path,
    md_text: str,
    title: str,
    summary: str,
    tags: list[str],
    visibility: str,
) -> tuple[bool, dict[str, Any] | None, str, int]:
    eprint("[Relay] 走 Edge Function upload-session-relay ...")

    relay_apikey = os.environ.get("CHATS_RELAY_APIKEY", "").strip()

    md_b64 = base64.b64encode(md_text.encode("utf-8")).decode("ascii")
    payload = {
        "api_key": token,
        "session_title": title,
        "session_summary": summary,
        "session_file_name": md_file.name,
        "session_content_b64": md_b64,
        "session_content_encoding": "base64-utf8",
        "session_tags": tags,
        "session_visibility": visibility,
    }

    ctx = make_ssl_context()
    status, parsed, raw = post_json(relay_url, payload, apikey=relay_apikey, timeout=120, ssl_context=ctx)

    if status == 0 and "[SSL_ERROR]" in raw:
        eprint("[Relay] SSL 验证失败，尝试自动查找系统 CA bundle ...")
        retry_ctx = find_working_ca()
        if retry_ctx:
            eprint("[Relay] 找到可用 CA，重试中 ...")
            status, parsed, raw = post_json(relay_url, payload, apikey=relay_apikey, timeout=120, ssl_context=retry_ctx)
        else:
            eprint("[Relay] 未找到可用系统 CA bundle")

    if status != 200:
        return False, parsed if isinstance(parsed, dict) else None, f"relay HTTP {status}: {raw}", status

    if not isinstance(parsed, dict):
        return False, None, f"relay 返回非 JSON 对象: {raw}", status

    success = bool(parsed.get("session_id") or parsed.get("sessionId") or parsed.get("ok") or parsed.get("success"))
    if not success:
        return False, parsed, f"relay success=false: {json.dumps(parsed, ensure_ascii=False)}", status

    return True, parsed, "", status


def main() -> int:
    parser = argparse.ArgumentParser(description="Upload markdown to ChatSpark via Edge Function relay")
    parser.add_argument("--project-dir", required=True, help="项目根目录（当前仅用于兼容参数）")
    parser.add_argument("--md-file", required=True, help="要上传的 Markdown 文件")
    parser.add_argument("--title", default="", help="可选：会话标题")
    parser.add_argument("--summary", default="", help="可选：会话摘要")
    parser.add_argument(
        "--visibility",
        default=DEFAULT_VISIBILITY,
        choices=list(ALLOWED_VISIBILITIES),
        help="可选：可见范围（public=公开 / private=私密），默认 public",
    )
    parser.add_argument("--relay-url", default="", help="可选：relay URL，默认 functions/v1/upload-session-relay")
    parser.add_argument("--disable-relay", action="store_true", help="禁用 relay（不建议）")
    parser.add_argument(
        "--token-file",
        default=None,
        help=(
            "可选：token 文件路径；默认按优先级解析："
            "CLI > $CHATS_HOME/chats_token > "
            "~/.chats/chats_token"
        ),
    )
    args = parser.parse_args()

    _project_dir = Path(args.project_dir).expanduser().resolve()
    md_file = Path(args.md_file).expanduser().resolve()
    token_file = resolve_token_file(args.token_file).resolve()

    if not md_file.exists():
        eprint(f"❌ Markdown 文件不存在: {md_file}")
        return 2

    if args.disable_relay:
        print(json.dumps({"ok": False, "stage": "relay", "response": "当前版本仅支持 relay 模式"}, ensure_ascii=False))
        return 2

    try:
        token = load_token(token_file)
    except Exception as ex:
        eprint(f"❌ 配置错误: {ex}")
        return 2

    md_text = md_file.read_text(encoding="utf-8")
    title = (args.title or infer_title(md_text, md_file)).strip()[:200]
    summary = (args.summary or infer_summary(md_text)).strip()[:500]
    tags = resolve_tags(title=title, summary=summary, md_text=md_text)
    visibility = normalize_visibility(args.visibility)

    relay_url = resolve_relay_url(args.relay_url, args.disable_relay)
    if not relay_url:
        print(json.dumps({"ok": False, "stage": "relay", "response": "缺少 relay URL"}, ensure_ascii=False))
        return 2

    ok, relay_json, relay_error, relay_status = upload_via_relay(
        relay_url=relay_url,
        token=token,
        md_file=md_file,
        md_text=md_text,
        title=title,
        summary=summary,
        tags=tags,
        visibility=visibility,
    )
    if ok and isinstance(relay_json, dict):
        result = dict(relay_json)
        result.setdefault("ok", True)
        result.setdefault("mode", "relay")
        result.setdefault("title", title)
        result.setdefault("summary", summary)
        result.setdefault("tags", tags)
        result.setdefault("visibility", visibility)
        result.setdefault("md_file", str(md_file))
        print(json.dumps(result, ensure_ascii=False))
        return 0

    is_ssl = "[SSL_ERROR]" in relay_error or "CERTIFICATE_VERIFY_FAILED" in relay_error
    response = relay_json if isinstance(relay_json, dict) else {
        "ok": False,
        "stage": "relay",
        "http": relay_status,
        "response": relay_error,
        **({"ssl_error": True} if is_ssl else {}),
    }
    print(json.dumps(response, ensure_ascii=False))
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
