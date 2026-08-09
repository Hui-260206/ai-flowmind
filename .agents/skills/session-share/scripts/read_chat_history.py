#!/usr/bin/env python3
"""
Read CodeBuddy / Claude Code / Cursor chat history from local archive files.

This script reads conversation data directly from local storage of:
  - CodeBuddyExtension (Tencent CodeBuddy)
  - Claude Code / Claude Code Internal (Anthropic)
  - Cursor IDE agent transcripts

Providing complete chat history that doesn't depend on AI context window limitations.

== CodeBuddy Storage layouts (auto-detected per platform) ==

  macOS: ~/Library/Application Support/CodeBuddyExtension/Data/
    {userId}/CodeBuddyIDE/{userId}/history/{workspaceHash}/{conversationId}/

  Linux: ~/.local/share/CodeBuddyExtension/Data/
    {userId}/CodeBuddyIDE/{sessionId}/history/{workspaceHash}/{conversationId}/

  Each conversation directory contains:
    ├── index.json          (message order + metadata)
    └── messages/           (individual message JSON files)
        └── {messageId}.json

== Claude Code Storage layouts ==

  macOS/Linux (standard):  ~/.claude/projects/{encoded-path}/{session-uuid}.jsonl
  macOS/Linux (internal):  ~/.claude-internal/projects/{encoded-path}/{session-uuid}.jsonl

  Path encoding: replace '/' with '-' in the absolute project path
  Each .jsonl file contains one JSON object per line (JSONL format).

Usage:
    # Export current conversation to Markdown (auto-detects CodeBuddy or Claude Code)
    python3 read_chat_history.py --workspace-dir /path/to/workspace --conversation-id <id> --output chat.md

    # List all conversations in a workspace
    python3 read_chat_history.py --workspace-dir /path/to/workspace --list

    # Auto-detect and export (uses most recent conversation)
    python3 read_chat_history.py --workspace-dir /path/to/workspace --auto --output chat.md

    # Force a specific source (codebuddy / claudecode)
    python3 read_chat_history.py --workspace-dir /path/to/workspace --source claudecode --auto --output chat.md

    # Output as JSON instead of Markdown
    python3 read_chat_history.py --workspace-dir /path/to/workspace --conversation-id <id> --format json
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Optional


# ============ Sensitive Info Redaction ============

# 高置信度敏感信息匹配规则（仅匹配带明确前缀的 Token 和 Auth Header）
_REDACT_PATTERNS: list[tuple[re.Pattern, str]] = [
    # CodeBuddy API Key: cb_sk_<hex/alnum 8+>
    (re.compile(r"(cb_sk_)[A-Za-z0-9]{8,}"), r"\1***REDACTED***"),
    # Anthropic API Key: sk-ant-<alnum/dash 8+>
    (re.compile(r"(sk-ant-)[A-Za-z0-9\-_]{8,}"), r"\1***REDACTED***"),
    # OpenAI API Key: sk-<alnum 8+> (排除 sk-ant- 已匹配的)
    (re.compile(r"(sk-(?!ant-))([A-Za-z0-9\-_]{8,})"), r"\1***REDACTED***"),
    # GitHub Token: ghp_ / gho_ / ghs_ / ghr_ + <alnum 8+>
    (re.compile(r"(gh[posr]_)[A-Za-z0-9]{8,}"), r"\1***REDACTED***"),
    # GitLab Token: glpat-<alnum 8+>
    (re.compile(r"(glpat-)[A-Za-z0-9\-_]{8,}"), r"\1***REDACTED***"),
    # Slack Token: xoxb- / xoxp- / xoxs- + <alnum/dash 8+>
    (re.compile(r"(xox[bps]-)[A-Za-z0-9\-]{8,}"), r"\1***REDACTED***"),
    # AWS Access Key: AKIA + 16 uppercase alnum
    (re.compile(r"(AKIA)[A-Z0-9]{16}"), r"\1***REDACTED***"),
    # Bearer Auth Header: Bearer <token 8+>
    (re.compile(r"(Bearer\s+)[A-Za-z0-9\-_=+/.]{8,}", re.IGNORECASE), r"\1***REDACTED***"),
    # Basic Auth Header: Basic <base64 8+>
    (re.compile(r"(Basic\s+)[A-Za-z0-9+/=]{8,}", re.IGNORECASE), r"\1***REDACTED***"),
]


def detect_sensitive_info(text: str) -> list[dict]:
    """检测文本中的高置信度敏感信息（只检测，不替换）。

    Returns:
        [{"type": "规则名称", "match": "原始匹配文本", "preview": "前缀***（截断预览）"}, ...]
    """
    if not text:
        return []

    findings: list[dict] = []
    for pattern, replacement in _REDACT_PATTERNS:
        for m in pattern.finditer(text):
            matched = m.group(0)
            # 生成预览：保留前缀，后面截断显示
            preview = pattern.sub(replacement, matched)
            findings.append({
                "type": _pattern_label(pattern),
                "match": matched,
                "preview": preview,
            })
    return findings


def _pattern_label(pattern: re.Pattern) -> str:
    """根据正则模式推断敏感信息类型的可读标签。"""
    src = pattern.pattern
    if "cb_sk_" in src:
        return "CodeBuddy Token"
    if "sk-ant-" in src:
        return "Anthropic API Key"
    if "sk-" in src:
        return "OpenAI API Key"
    if "gh[posr]_" in src:
        return "GitHub Token"
    if "glpat-" in src:
        return "GitLab Token"
    if "xox[bps]-" in src:
        return "Slack Token"
    if "AKIA" in src:
        return "AWS Access Key"
    if "Bearer" in src:
        return "Bearer Auth Header"
    if "Basic" in src:
        return "Basic Auth Header"
    return "Unknown"


def redact_sensitive_info(text: str) -> tuple[str, int]:
    """对文本中的高置信度敏感信息进行脱敏。

    仅匹配带明确前缀的 Token（如 cb_sk_、sk-、ghp_ 等）和 Bearer/Basic Auth Header，
    不匹配通用密钥赋值模式或长十六进制串，避免误判。

    Args:
        text: 待脱敏的文本

    Returns:
        (脱敏后的文本, 脱敏处理的次数)
    """
    if not text:
        return text, 0

    total_count = 0
    result = text
    for pattern, replacement in _REDACT_PATTERNS:
        result, count = pattern.subn(replacement, result)
        total_count += count
    return result, total_count


# ============ Cross-platform Path Discovery ============

# --------------- CodeBuddy ---------------

def get_candidate_data_dirs(extra_dirs: Optional[list[str]] = None) -> list[str]:
    """返回所有实际存在的 CodeBuddy 数据存储根目录。

    优先级（从高到低）：
      1. extra_dirs：调用方显式传入（通常由 AI 从 <artifact_directory_path> 推导，或环境变量）
      2. CODEBUDDY_DATA_ROOT 环境变量（支持多个，用 ':' 分隔）
      3. 内置候选列表（按平台）：
         macOS:   ~/Library/Application Support/{CodeBuddyExtension/Data, CodeBuddy CN/..., CodeBuddy/...}
         Linux:   ~/.local/share/CodeBuddyExtension/Data  (XDG_DATA_HOME)
                  ~/.config/{CodeBuddy CN/..., CodeBuddy/...}  (XDG_CONFIG_HOME)

    任何路径都会展开 ~ 并过滤掉实际不存在的目录，结果保持原顺序、去重。
    """
    raw_candidates: list[str] = []

    # 1. 显式传入（最高优先级）
    if extra_dirs:
        raw_candidates.extend(extra_dirs)

    # 2. 环境变量
    env_root = os.environ.get("CODEBUDDY_DATA_ROOT", "").strip()
    if env_root:
        raw_candidates.extend([p for p in env_root.split(":") if p.strip()])

    # 3. 内置候选（兜底）
    system = platform.system()
    if system == "Darwin":  # macOS
        app_support = "~/Library/Application Support"
        raw_candidates.extend([
            f"{app_support}/CodeBuddyExtension/Data",
            f"{app_support}/CodeBuddy CN/User/globalStorage/tencent-cloud.coding-copilot",
            f"{app_support}/CodeBuddy/User/globalStorage/tencent-cloud.coding-copilot",
        ])
    elif system == "Linux":
        xdg_data = os.environ.get("XDG_DATA_HOME", "~/.local/share")
        xdg_config = os.environ.get("XDG_CONFIG_HOME", "~/.config")
        raw_candidates.extend([
            f"{xdg_data}/CodeBuddyExtension/Data",
            f"{xdg_config}/CodeBuddyExtension/Data",
            f"{xdg_config}/CodeBuddy CN/User/globalStorage/tencent-cloud.coding-copilot",
            f"{xdg_config}/CodeBuddy/User/globalStorage/tencent-cloud.coding-copilot",
        ])

    # 展开、去重、过滤存在的路径
    seen: set[str] = set()
    result: list[str] = []
    for p in raw_candidates:
        expanded = os.path.expanduser(p.strip())
        if not expanded or expanded in seen:
            continue
        seen.add(expanded)
        if os.path.isdir(expanded):
            result.append(expanded)
    return result


# ============ Core Functions ============


def _path_encoding_candidates(workspace_dir: str) -> list[str]:
    """生成同一 workspace 的多种合理路径表示（去重、按优先级排序）。

    背景：Claude Code Internal / Cursor / 部分 IDE 在写盘时，会先把 cwd 做
    `realpath` 解析，再编码为存档桶名；而读取端若仅用 `abspath`，遇到
    symlink 工作区（含 macOS 上 /tmp → /private/tmp 的祖传 symlink，或用户
    用 `ln -s` 把 skills 目录软链到其他位置）就会编出**与磁盘真实桶名不一致**
    的路径，导致"找不到会话文件"。

    本函数返回**所有合理候选**的有序去重列表，调用方应**逐个尝试**：
      1. abspath（保持向后兼容，原行为）
      2. realpath（解 symlink，覆盖大多数 symlink 场景）
      3. macOS /private 前缀对偶（/private/tmp ↔ /tmp）

    所有路径都会去掉尾部斜杠以保持归一化。
    """
    candidates: list[str] = []
    seen: set[str] = set()

    def _add(p: str) -> None:
        if not p:
            return
        normalized = p.rstrip("/") or p  # 保留根路径 "/" 不被剥成空串
        if normalized in seen:
            return
        seen.add(normalized)
        candidates.append(normalized)

    abs_p = os.path.abspath(workspace_dir)
    real_p = os.path.realpath(workspace_dir)
    _add(abs_p)
    _add(real_p)

    # macOS 上 /private/tmp 与 /tmp、/private/var 与 /var 互为别名
    for p in (abs_p, real_p):
        if p.startswith("/private/"):
            _add(p[len("/private"):])  # /private/tmp/x → /tmp/x
        elif p.startswith("/tmp/") or p == "/tmp":
            _add("/private" + p)
        elif p.startswith("/var/") or p == "/var":
            _add("/private" + p)

    return candidates


def get_workspace_hash(workspace_dir: str) -> str:
    """Compute the workspace hash (MD5 of absolute path without trailing slash)."""
    abs_path = os.path.abspath(workspace_dir).rstrip("/")
    return hashlib.md5(abs_path.encode("utf-8")).hexdigest()


def get_workspace_hash_candidates(workspace_dir: str) -> list[str]:
    """返回所有候选的 workspace hash（abspath / realpath / 别名前缀去重后的 MD5）。

    用于 CodeBuddy 系：写盘端用 hash(abspath) 而非编码路径，但同样存在
    symlink 不一致问题。读取时遍历所有候选 hash，命中即用。
    """
    return [
        hashlib.md5(p.encode("utf-8")).hexdigest()
        for p in _path_encoding_candidates(workspace_dir)
    ]



def find_history_base(
    workspace_hash: str,
    extra_data_dirs: Optional[list[str]] = None,
    workspace_hash_candidates: Optional[list[str]] = None,
) -> Optional[str]:
    """跨平台搜索 workspace 的历史记录目录。

    Args:
        workspace_hash:             首选 workspace hash（保持向后兼容的入参）
        extra_data_dirs:            调用方显式传入的 CodeBuddy 数据根目录列表（最高优先级）
        workspace_hash_candidates:  可选的 hash 候选列表（abspath/realpath 双候选场景）。
                                    若提供，会按顺序逐个尝试；若未提供，仅尝试 workspace_hash。

    遍历所有候选数据目录，对每个目录尝试多种子结构：
      结构1: {userId}/CodeBuddyIDE/{userId}/history/{wsHash}    （macOS 登录用户）
      结构2: {entry}/CodeBuddyIDE/history/{wsHash}              （default 等非 UUID 目录）
      结构3: {userId}/CodeBuddyIDE/{sessionId}/history/{wsHash}  （Linux，sessionId ≠ userId）

    返回第一个匹配的路径，找不到返回 None。
    """
    data_dirs = get_candidate_data_dirs(extra_dirs=extra_data_dirs)

    if not data_dirs:
        print("⚠️  未找到任何 CodeBuddy 数据目录", file=sys.stderr)
        print(f"   当前系统: {platform.system()}", file=sys.stderr)
        return None

    # 构造 hash 候选列表（去重、保序）：首选 workspace_hash，再加 candidates
    hash_list: list[str] = []
    seen_hash: set[str] = set()
    for h in [workspace_hash] + (workspace_hash_candidates or []):
        if h and h not in seen_hash:
            seen_hash.add(h)
            hash_list.append(h)

    primary_hash = hash_list[0] if hash_list else workspace_hash

    for data_dir in data_dirs:
        try:
            entries = os.listdir(data_dir)
        except OSError:
            continue

        for entry in entries:
            entry_path = os.path.join(data_dir, entry)
            if not os.path.isdir(entry_path):
                continue
            if entry in ("Public", "Cache", "Logs"):
                continue

            # 对每个 entry，按 hash 候选顺序尝试三种结构
            for ws_hash in hash_list:
                # 先尝试固定结构（结构1 和 结构2）
                candidate_paths = [
                    # 结构1: {userId}/CodeBuddyIDE/{userId}/history/{wsHash}
                    os.path.join(entry_path, "CodeBuddyIDE", entry, "history", ws_hash),
                    # 结构2: {entry}/CodeBuddyIDE/history/{wsHash} (适用于 default 等)
                    os.path.join(entry_path, "CodeBuddyIDE", "history", ws_hash),
                ]

                for hist_path in candidate_paths:
                    if os.path.isdir(hist_path):
                        if ws_hash != primary_hash:
                            print(
                                f"📂 找到数据源(回退候选): {hist_path}\n"
                                f"   （首选 hash '{primary_hash}' 未命中，已自动回退到 '{ws_hash}'，"
                                f"通常意味着 workspace 路径含 symlink）",
                                file=sys.stderr,
                            )
                        else:
                            print(f"📂 找到数据源: {hist_path}", file=sys.stderr)
                        return hist_path

                # 结构3: {userId}/CodeBuddyIDE/{sessionId}/history/{wsHash}
                # Linux 上 sessionId 与 userId 不同，需要遍历 CodeBuddyIDE 下的子目录
                codebuddy_ide_path = os.path.join(entry_path, "CodeBuddyIDE")
                if os.path.isdir(codebuddy_ide_path):
                    try:
                        for sub_entry in os.listdir(codebuddy_ide_path):
                            if sub_entry == entry:
                                continue  # 已在结构1中尝试过
                            hist_path = os.path.join(
                                codebuddy_ide_path, sub_entry, "history", ws_hash
                            )
                            if os.path.isdir(hist_path):
                                if ws_hash != primary_hash:
                                    print(
                                        f"📂 找到数据源(回退候选): {hist_path}\n"
                                        f"   （首选 hash '{primary_hash}' 未命中，已自动回退到 '{ws_hash}'）",
                                        file=sys.stderr,
                                    )
                                else:
                                    print(f"📂 找到数据源: {hist_path}", file=sys.stderr)
                                return hist_path
                    except OSError:
                        continue

    # 未找到时输出诊断信息
    print("⚠️  在以下位置未找到匹配的历史记录:", file=sys.stderr)
    for d in data_dirs:
        print(f"   📁 {d}", file=sys.stderr)
    if len(hash_list) > 1:
        print(f"   尝试过的 hash 候选: {hash_list}", file=sys.stderr)
    return None


# --------------- Claude Code ---------------


def get_claudecode_projects_dirs(
    extra_dirs: Optional[list[str]] = None,
) -> list[str]:
    """返回所有实际存在的 Claude Code projects 根目录（按优先级排序）。

    优先级（从高到低）：
      1. extra_dirs：调用方显式传入（通常由 AI 通过 shell 探测得到）
      2. CLAUDECODE_PROJECTS_DIR 环境变量（多个用 ':' 分隔）
      3. 内置候选：~/.claude-internal/projects（内部版） 和 ~/.claude/projects（标准版）

    展开 ~ 并按原顺序去重，仅保留实际存在的目录。
    """
    raw_candidates: list[str] = []

    if extra_dirs:
        raw_candidates.extend(extra_dirs)

    env_dirs = os.environ.get("CLAUDECODE_PROJECTS_DIR", "").strip()
    if env_dirs:
        raw_candidates.extend([p for p in env_dirs.split(":") if p.strip()])

    raw_candidates.extend([
        "~/.claude-internal/projects",   # Claude Code Internal (企业/内部版)
        "~/.claude/projects",             # Claude Code 标准社区版
    ])

    seen: set[str] = set()
    result: list[str] = []
    for p in raw_candidates:
        expanded = os.path.expanduser(p.strip())
        if not expanded or expanded in seen:
            continue
        seen.add(expanded)
        if os.path.isdir(expanded):
            result.append(expanded)
    return result


def get_cursor_projects_dirs(extra_dirs: Optional[list[str]] = None) -> list[str]:
    """返回所有实际存在的 Cursor projects 根目录（按优先级排序）。"""
    raw_candidates: list[str] = []

    if extra_dirs:
        raw_candidates.extend(extra_dirs)

    env_dirs = os.environ.get("CURSOR_PROJECTS_DIR", "").strip()
    if env_dirs:
        raw_candidates.extend([p for p in env_dirs.split(":") if p.strip()])

    raw_candidates.extend(["~/.cursor/projects"])

    seen: set[str] = set()
    result: list[str] = []
    for p in raw_candidates:
        expanded = os.path.expanduser(p.strip())
        if not expanded or expanded in seen:
            continue
        seen.add(expanded)
        if os.path.isdir(expanded):
            result.append(expanded)
    return result


def encode_cursor_workspace_path(workspace_dir: str) -> str:
    """将工作区绝对路径编码为 Cursor projects 的目录名（首选 abspath）。"""
    abs_path = os.path.abspath(workspace_dir)
    return abs_path.replace("/", "-").lstrip("-")


# --------------- WorkBuddy ---------------


def get_workbuddy_projects_dirs(extra_dirs: Optional[list[str]] = None) -> list[str]:
    """返回所有实际存在的 WorkBuddy projects 根目录（按优先级排序）。

    优先级（从高到低）：
      1. extra_dirs：调用方显式传入
      2. WORKBUDDY_PROJECTS_DIR 环境变量（多个用 ':' 分隔）
      3. 内置候选：~/.workbuddy/projects

    展开 ~ 并按原顺序去重，仅保留实际存在的目录。
    """
    raw_candidates: list[str] = []

    if extra_dirs:
        raw_candidates.extend(extra_dirs)

    env_dirs = os.environ.get("WORKBUDDY_PROJECTS_DIR", "").strip()
    if env_dirs:
        raw_candidates.extend([p for p in env_dirs.split(":") if p.strip()])

    raw_candidates.append("~/.workbuddy/projects")

    seen: set[str] = set()
    result: list[str] = []
    for p in raw_candidates:
        expanded = os.path.expanduser(p.strip())
        if not expanded or expanded in seen:
            continue
        seen.add(expanded)
        if os.path.isdir(expanded):
            result.append(expanded)
    return result


def find_workbuddy_session_dir(
    workspace_dir: str, extra_projects_dirs: Optional[list[str]] = None
) -> Optional[str]:
    """在 WorkBuddy projects 目录中找到对应工作区的 session 目录。

    WorkBuddy 项目目录命名格式为 {username}-WorkBuddy-{timestamp}，
    与 Cursor/Claude Code 的 workspace 路径编码不同，无法通过目录名直接定位。
    因此扫描所有项目目录，通过对话 JSONL 文件中的 cwd 字段匹配目标工作区。
    会按"abspath → realpath → 别名前缀"顺序逐候选尝试，命中即返回。
    """
    projects_dirs = get_workbuddy_projects_dirs(extra_dirs=extra_projects_dirs)
    if not projects_dirs:
        return None

    # 生成 workspace 的所有候选路径（abspath / realpath / 别名前缀）
    workspace_paths: set[str] = set()
    for p in _path_encoding_candidates(workspace_dir):
        workspace_paths.add(p.rstrip("/"))

    for projects_dir in projects_dirs:
        try:
            entries = os.listdir(projects_dir)
        except OSError:
            continue

        for entry in entries:
            project_dir = os.path.join(projects_dir, entry)
            if not os.path.isdir(project_dir):
                continue

            # 在项目目录中找第一个 .jsonl 文件，读取 cwd 进行匹配
            try:
                files = os.listdir(project_dir)
            except OSError:
                continue

            for fname in files:
                if not fname.endswith(".jsonl"):
                    continue
                fpath = os.path.join(project_dir, fname)
                try:
                    with open(fpath, "r", encoding="utf-8") as f:
                        first_line = f.readline().strip()
                    if not first_line:
                        continue
                    obj = json.loads(first_line)
                    cwd = (obj.get("cwd") or "").rstrip("/")
                    if cwd in workspace_paths:
                        print(
                            f"📂 找到 WorkBuddy 数据源: {project_dir}",
                            file=sys.stderr,
                        )
                        return project_dir
                except (json.JSONDecodeError, OSError):
                    continue
                # 同一项目目录内所有 session 共享 cwd，
                # 第一个 jsonl 不匹配即跳到下一个项目目录
                break

    return None


# --------------- Box AI ---------------


def get_boxai_db_path(extra_path: Optional[str] = None) -> Optional[str]:
    """返回实际存在的 Box AI sessions.db 路径。

    优先级（从高到低）：
      1. extra_path：调用方显式传入
      2. BOXAI_DB_PATH 环境变量
      3. 内置候选（按平台）：
         macOS: ~/Library/Application Support/Box/engine/sessions.db
         Linux: ~/.config/Box/engine/sessions.db 或 ~/.local/share/Box/engine/sessions.db
    """
    candidates: list[str] = []

    if extra_path:
        candidates.append(extra_path)

    env_path = os.environ.get("BOXAI_DB_PATH", "").strip()
    if env_path:
        candidates.append(env_path)

    system = platform.system()
    if system == "Darwin":
        candidates.append("~/Library/Application Support/Box/engine/sessions.db")
    elif system == "Linux":
        xdg_config = os.environ.get("XDG_CONFIG_HOME", "~/.config")
        xdg_data = os.environ.get("XDG_DATA_HOME", "~/.local/share")
        candidates.append(f"{xdg_config}/Box/engine/sessions.db")
        candidates.append(f"{xdg_data}/Box/engine/sessions.db")
    else:  # Windows
        appdata = os.environ.get("APPDATA", "")
        if appdata:
            candidates.append(os.path.join(appdata, "Box", "engine", "sessions.db"))

    for p in candidates:
        expanded = os.path.expanduser(p)
        if os.path.isfile(expanded):
            return expanded

    return None


def encode_cursor_workspace_path_all(workspace_dir: str) -> list[str]:
    """返回所有候选的 Cursor 桶名编码（abspath / realpath / 别名前缀，去重）。"""
    seen: set[str] = set()
    result: list[str] = []
    for p in _path_encoding_candidates(workspace_dir):
        encoded = p.replace("/", "-").lstrip("-")
        if encoded and encoded not in seen:
            seen.add(encoded)
            result.append(encoded)
    return result


def find_cursor_transcripts_dir(
    workspace_dir: str, extra_projects_dirs: Optional[list[str]] = None
) -> Optional[str]:
    """在 Cursor projects 目录中找到当前工作区的 agent-transcripts 目录。

    会按"abspath → realpath → 别名前缀"顺序逐个尝试候选编码，命中即返回。
    """
    encoded_candidates = encode_cursor_workspace_path_all(workspace_dir)
    projects_dirs = get_cursor_projects_dirs(extra_dirs=extra_projects_dirs)
    if not projects_dirs or not encoded_candidates:
        return None

    primary = encoded_candidates[0]
    for projects_dir in projects_dirs:
        for encoded in encoded_candidates:
            transcripts_dir = os.path.join(projects_dir, encoded, "agent-transcripts")
            if os.path.isdir(transcripts_dir):
                if encoded != primary:
                    print(
                        f"📂 找到 Cursor 数据源(回退候选): {transcripts_dir}\n"
                        f"   （首选 '{primary}' 未命中，已自动回退到 '{encoded}'，"
                        f"通常意味着 workspace 路径含 symlink）",
                        file=sys.stderr,
                    )
                else:
                    print(f"📂 找到 Cursor 数据源: {transcripts_dir}", file=sys.stderr)
                return transcripts_dir
    return None


def encode_workspace_path(workspace_dir: str) -> str:
    """将工作区绝对路径编码为 Claude Code 的目录名（首选 abspath）。

    规则：把路径中的 '/' 全部替换为 '-'
    示例：/Users/alice/myapp  →  -Users-alice-myapp

    注意：Claude Code Internal 在写盘时常对 cwd 做 realpath 解析，因此当
    workspace_dir 是 symlink 时，仅用本函数会编出与磁盘真实桶名不一致的路径。
    建议读取场景使用 `encode_workspace_path_all` 获得多候选并逐个尝试。
    """
    abs_path = os.path.abspath(workspace_dir)
    return abs_path.replace("/", "-")


def encode_workspace_path_all(workspace_dir: str) -> list[str]:
    """返回所有候选的 Claude Code 桶名编码（abspath / realpath / 别名前缀，去重）。

    用于解决 symlink 工作区下"读端 abspath ≠ 写端 realpath"的桶名错位问题。
    """
    seen: set[str] = set()
    result: list[str] = []
    for p in _path_encoding_candidates(workspace_dir):
        encoded = p.replace("/", "-")
        if encoded and encoded not in seen:
            seen.add(encoded)
            result.append(encoded)
    return result


def find_claudecode_session_dir(
    workspace_dir: str, extra_projects_dirs: Optional[list[str]] = None
) -> Optional[str]:
    """在所有 Claude Code projects 目录中找到对应工作区的 session 目录。

    Args:
        workspace_dir:        工作区绝对路径
        extra_projects_dirs:  调用方显式传入的 projects 根目录列表（最高优先级）

    会按"abspath → realpath → 别名前缀"顺序逐个尝试候选编码，命中即返回。
    若回退到非首选候选，会在 stderr 打印提示。
    """
    encoded_candidates = encode_workspace_path_all(workspace_dir)
    projects_dirs = get_claudecode_projects_dirs(extra_dirs=extra_projects_dirs)

    if not projects_dirs or not encoded_candidates:
        return None

    primary = encoded_candidates[0]
    for projects_dir in projects_dirs:
        for encoded in encoded_candidates:
            session_dir = os.path.join(projects_dir, encoded)
            if os.path.isdir(session_dir):
                if encoded != primary:
                    print(
                        f"📂 找到 Claude Code 数据源(回退候选): {session_dir}\n"
                        f"   （首选 '{primary}' 未命中，已自动回退到 '{encoded}'，"
                        f"通常意味着 workspace 路径含 symlink）",
                        file=sys.stderr,
                    )
                else:
                    print(f"📂 找到 Claude Code 数据源: {session_dir}", file=sys.stderr)
                return session_dir

    return None


_INTERNAL_BLOCK_TAGS = [
    "system_reminder",
    "system-reminder",   # Claude Code 使用连字符版本
    "rules",
    "always_applied_user_rules",
    "memories",
    "git_status",
    "project_context",
    "project_layout",
    "conversation_history_summary",
    "additional_data",
    "open_and_recently_viewed_files",
    "artifact_directory_path",
    "user_info",
    "claudeMd",
    "currentDate",
    "local-command-caveat",   # Claude Code 本地命令注入的告知块
    "local-command-stdout",   # Claude Code 本地命令的标准输出
    "local-command-stderr",   # Claude Code 本地命令的标准错误
]

# 通配正则：一次性匹配所有符合系统注入命名模式的 XML 标签块
# 覆盖 Claude Code 现有及未来可能新增的注入标签，无需逐个维护白名单
_SYSTEM_TAG_PATTERN = re.compile(
    r"<((?:"
    r"system[\w_-]*"                  # system_reminder, system-reminder, system_prompt, ...
    r"|local-command-[\w-]+"          # local-command-caveat, local-command-stdout, ...
    r"|claude[\w-]*context"           # claude-5-context, claude3-context, ...
    r"|cb_summary"                    # CodeBuddy 摘要注入
    r"|(?:git_status|user_info|project_context|project_layout"
    r"|additional_data|artifact_directory_path|rules"
    r"|memories|always_applied_user_rules"
    r"|conversation_history_summary|open_and_recently_viewed_files"
    r"|currentDate|claudeMd|plan_mode_reminder"
    r"|response_language|agentic_mode_overview|communication"
    r"|tool_calling|maximize_parallel_tool_calls|making_code_changes"
    r"|integrations_protocol|agent_skills|inline_line_numbers"
    r"|task_management|mcp_protocol|automations|maximize_context_understanding"
    r"|content_policy|code-explorer_subagent_usage|result_presentation)"
    r"))[^>]*>[\s\S]*?</\1>",
    re.IGNORECASE,
)

# 孤立标签（无内容匹配的开/闭标签）的通配正则
_SYSTEM_TAG_ORPHAN_PATTERN = re.compile(
    r"</?(?:"
    r"system[\w_-]*"
    r"|local-command-[\w-]+"
    r"|claude[\w-]*context"
    r"|cb_summary"
    r"|git_status|user_info|project_context|project_layout"
    r"|additional_data|artifact_directory_path|rules"
    r"|memories|always_applied_user_rules"
    r"|conversation_history_summary|open_and_recently_viewed_files"
    r"|currentDate|claudeMd|plan_mode_reminder"
    r"|response_language|agentic_mode_overview|communication"
    r"|tool_calling|making_code_changes|integrations_protocol"
    r"|agent_skills|inline_line_numbers|task_management|mcp_protocol"
    r"|automations|content_policy|result_presentation"
    r")\b[^>]*>",
    re.IGNORECASE,
)

# Claude Code context compaction 注入的摘要前缀（这类消息是系统生成的，不是用户真实输入）
_CLAUDECODE_COMPACTION_PREFIXES = (
    "This session is being continued from a previous conversation that ran out of context.",
    "The conversation has been compacted.",
)

# Claude Code 自动插入的中断提示（非用户实际输入，客户端自动生成）
_CLAUDECODE_AUTO_INTERRUPT_TEXTS = (
    "[Request interrupted by user]",
    "[Request interrupted by user for tool use]",
    "[Request interrupted by user - tool response cancelled]",
)

# Claude Code skill/command 调用时注入的 content 字段前缀
_CLAUDECODE_SKILL_PREFIXES = (
    "Base directory for this skill:",
)


def _is_claudecode_system_injection(text: str) -> bool:
    """判断一段 Claude Code user 消息文本是否为系统注入内容（非用户真实输入）。

    包含以下几类：
    1. Context compaction 摘要（对话压缩后注入的上下文总结）
    2. Skill/command 内容注入（SKILL.md 被注入为 user 消息）
    3. 仅包含 <command-message>/<command-name> 标签（skill 调用触发行）
    4. 客户端自动插入的中断提示（[Request interrupted by user] 等）
    """
    stripped = (text or "").strip()
    if not stripped:
        return False

    # 1. Context compaction 摘要
    for prefix in _CLAUDECODE_COMPACTION_PREFIXES:
        if stripped.startswith(prefix):
            return True

    # 2. Skill 内容注入（以 skill 基础目录声明开头）
    for prefix in _CLAUDECODE_SKILL_PREFIXES:
        if stripped.startswith(prefix):
            return True

    # 3. 客户端自动插入的中断提示（精确匹配）
    for auto_text in _CLAUDECODE_AUTO_INTERRUPT_TEXTS:
        if stripped == auto_text:
            return True

    # 4. 仅包含 command-message/command-name/command-args 标签（skill 触发行，无实际用户文本）
    only_command_tags = re.sub(
        r"<command-(?:message|name|args)[^>]*>[\s\S]*?</command-(?:message|name|args)>",
        "",
        stripped,
        flags=re.IGNORECASE,
    ).strip()
    if not only_command_tags and re.search(
        r"<command-(?:message|name|args)", stripped, re.IGNORECASE
    ):
        return True

    # 5. 未知命令错误提示（如 "Unknown command: /session-share"）
    if re.match(r"Unknown command:\s*/", stripped):
        return True

    # 6. XML 密度检测：系统注入消息通常由大量 XML 标签组成，正常用户输入极少如此
    #    计算文本中 XML 标签字符数（<tag ...>...</tag>）占总长度的比例
    #    阈值 40%：超过则判定为系统注入（宁过勿漏原则）
    if len(stripped) > 100:  # 太短的消息不做密度检测（避免误判简短 XML 内容）
        tag_chars = sum(len(m) for m in re.findall(r"<[^>]+>", stripped))
        if tag_chars / len(stripped) > 0.40:
            return True

    return False


def _extract_claudecode_meta_user_input(text: str) -> str:
    """从 isMeta 用户消息中提取真实用户输入。

    Claude Code 斜杠命令触发 skill 后，存档的 user 消息会包含平台注入的 dispatch 模板。
    真实用户输入位于 ARGUMENTS: 标记之后。此标记是跨语言的（不依赖中文前缀）。

    dispatch 模板示例:
        **此命令调度到完整的 deploy skill...
        ...skill 文件是**唯一权威来源**...
        ARGUMENTS: @test/ 并将项目环境变量设置到平台.

    如果 ARGUMENTS: 存在 → 返回其后的真实输入
    如果 ARGUMENTS: 不存在 → 返回空字符串（纯平台注入，无用户内容）

    对于 isMeta 且无 ARGUMENTS 的消息（如 <local-command-caveat>、<system-reminder>、
    skill 内容注入等），调用方应直接丢弃整条消息。
    """
    m = re.search(r"ARGUMENTS:\s*([\s\S]+)", text)
    if m:
        return m.group(1).strip()
    return ""


def _clean_internal_prompt_text(text: str, role: str) -> str:
    """清理会话文本中注入的系统提示/skill 说明，避免污染导出 markdown。"""
    cleaned = (text or "").strip()
    if not cleaned:
        return ""

    # Claude Code 系统注入整条消息直接丢弃
    if role == "user" and _is_claudecode_system_injection(cleaned):
        return ""

    if role == "user":
        user_query_match = re.search(
            r"<user_query>\s*(.*?)\s*</user_query>",
            cleaned,
            re.DOTALL | re.IGNORECASE,
        )
        if user_query_match:
            return user_query_match.group(1).strip()

    # 移除所有符合系统注入模式的标签块（通配正则，向后兼容 _INTERNAL_BLOCK_TAGS）
    # 迭代应用直到不再有变化（处理嵌套情况）
    prev = None
    while prev != cleaned:
        prev = cleaned
        cleaned = _SYSTEM_TAG_PATTERN.sub("", cleaned)
        # 兜底：逐个匹配白名单中可能未被通配覆盖的边界情况
        for tag in _INTERNAL_BLOCK_TAGS:
            escaped_tag = re.escape(tag)
            cleaned = re.sub(
                rf"<{escaped_tag}[^>]*>[\s\S]*?</{escaped_tag}>",
                "",
                cleaned,
                flags=re.IGNORECASE,
            )

    cleaned = re.sub(
        r"<command-(?:message|name|args)[^>]*>[\s\S]*?</command-(?:message|name|args)>",
        "",
        cleaned,
        flags=re.IGNORECASE,
    )

    # 移除残留孤立标签（通配模式）
    cleaned = _SYSTEM_TAG_ORPHAN_PATTERN.sub("", cleaned)
    # 兜底孤立标签（精确匹配，保留 user_query 标签不过滤）
    cleaned = re.sub(
        r"</?(?:user_query|system[_-]reminder|rules|always_applied_user_rules|memories"
        r"|project_context|additional_data|artifact_directory_path"
        r"|open_and_recently_viewed_files|user_info|git_status"
        r"|conversation_history_summary|project_layout|claudeMd|currentDate"
        r"|local-command-caveat|local-command-stdout|local-command-stderr)\b[^>]*>",
        "",
        cleaned,
        flags=re.IGNORECASE,
    )

    # Skill 内容注入：截断 "Base directory for this skill:" 之后的部分
    cleaned = re.split(r"(?i)Base directory for this skill:", cleaned, maxsplit=1)[0]

    # 截断分享 skill 说明文档（通过标题特征识别；兼容新旧标题）
    _SHARE_DOC_TITLE_RE = r"(?im)^\s*#\s*(?:分享对话到 ChatSpark|分享会话到 Session Hub|分享对话到 AI Community)\s*$"
    if re.search(_SHARE_DOC_TITLE_RE, cleaned):
        cleaned = re.split(_SHARE_DOC_TITLE_RE, cleaned, maxsplit=1)[0]

    # 兜底：整条消息以 skill 说明为主体时，丢弃
    if role == "user" and (
        ("分享对话到 ChatSpark" in cleaned or "分享会话到 Session Hub" in cleaned or "分享对话到 AI Community" in cleaned)
        and "完整工作流" in cleaned
    ):
        return ""

    cleaned = re.sub(r"\n{3,}", "\n\n", cleaned)
    return cleaned.strip()


def _clean_tool_result_preview(preview: str) -> str:
    """清理 tool_result 预览文本中可能包含的系统注入标签碎片。

    工具执行结果（如读取文件返回的内容）可能包含系统标签片段，
    写入 Markdown 前需要移除，避免污染导出文件。
    """
    if not preview:
        return preview
    cleaned = _SYSTEM_TAG_PATTERN.sub("", preview)
    cleaned = _SYSTEM_TAG_ORPHAN_PATTERN.sub("", cleaned)
    # 移除孤立的 < 残片（标签被截断后留下的碎片）
    cleaned = re.sub(r"<[A-Za-z][^>]{0,80}$", "", cleaned.rstrip())
    return cleaned.strip()


# 平台原生的图片文本标记（用户在输入框中的真实位置） → 替换为 [图片]
# re.ASCII 确保 \b \d 不把中文字符当作 \w
_REAL_IMAGE_MARKER_RE = re.compile(
    r"\[Image\s*#?\d+\]"
    r"|@image(?:[#:/]\S*)?(?=\s|$)",  # @image / @image#1:file.png / @image:file / @image/path
    re.ASCII,
)
# 元数据噪声（base64 省略残留 / 缓存路径 / 本地路径标记），非用户输入，直接移除
_IMAGE_META_NOISE_RE = re.compile(
    r"\[image\] media=[^\n]*"
    r"|\[Image: source: [^\]]+\]"
    r"|\[image_local_path\][^\n]*"
    r"|<image_local_path>[^<]*</image_local_path>"
)


def _replace_attachments_block(text: str) -> str:
    """将 Box AI 的 <attachments> 块替换为对应数量的 [图片] 占位符。

    Block format:
        <attachments>
        The user has attached the following files/resources...
        - /path/to/file.png (image)
        - /path/to/file2.png (image)
        </attachments>
    """
    def _repl(m):
        block = m.group(1)
        count = len(re.findall(r"\(image\)", block))
        if count == 0:
            return ""
        return "\n".join(["[图片]"] * count)
    return re.sub(r"<attachments>([\s\S]*?)</attachments>", _repl, text)


def _normalize_image_placeholders(text: str) -> str:
    """将文本中的图片占位符统一为 [图片].

    1. 将 Box AI <attachments> 块转换为对应数量的 [图片]
    2. 移除元数据噪声（base64 省略残留、缓存路径、本地路径标签）
    3. 将 [Image #N] / @image... 替换为 [图片]，保留原始位置
    4. 清理移除元数据后留下的多余空行
    """
    text = _replace_attachments_block(text)
    text = _IMAGE_META_NOISE_RE.sub("", text)
    text = _REAL_IMAGE_MARKER_RE.sub("[图片]", text)
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip()


def _extract_claudecode_user_text(content) -> str:
    """从 Claude Code user 消息的 content 字段提取用户输入文本。

    图片标记（[Image #N] 等）位于 text block 的文本流中，通过正则替换
    即可同时得到正确的占位符内容和位置。image block 不额外产出占位符，
    避免同一张图片的多种元数据表示导致重复。
    """
    if isinstance(content, str):
        return _normalize_image_placeholders(
            _clean_internal_prompt_text(content, role="user")
        )
    if isinstance(content, list):
        texts = []
        for item in content:
            if not isinstance(item, dict):
                continue
            if item.get("type") == "tool_result":
                continue
            if item.get("type") == "text":
                t = _clean_internal_prompt_text(item.get("text", ""), role="user")
                if t:
                    texts.append(_normalize_image_placeholders(t))
        return "\n".join(texts)
    return ""


def _extract_claudecode_assistant_text(content) -> str:
    """从 Claude Code assistant 消息的 content 字段提取文本回复。"""
    if isinstance(content, str):
        return _clean_internal_prompt_text(content, role="assistant")
    if isinstance(content, list):
        texts = []
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                t = _clean_internal_prompt_text(item.get("text", ""), role="assistant")
                if t:
                    texts.append(t)
        return "\n\n".join(texts)
    return ""


def _extract_anthropic_thinking(content) -> str:
    """从 anthropic 风格 content 数组中抽取 thinking / reasoning 块。

    适用于 CodeBuddy / Claude Code / Cursor / WorkBuddy（content 为 block 数组，
    存在 type=thinking / reasoning / redacted_thinking 的块）。
    """
    if not isinstance(content, list):
        return ""
    parts: list[str] = []
    for item in content:
        if not isinstance(item, dict):
            continue
        t = item.get("type", "")
        if t in ("thinking", "reasoning"):
            text = item.get("thinking") or item.get("text") or item.get("reasoning") or ""
            text = str(text).strip()
            if text:
                parts.append(text)
        elif t == "redacted_thinking":
            # 加密的思考内容，仅记录占位（保留原文则无意义，因为是 base64）
            parts.append("[模型思考过程已加密，无法展示原文]")
    return "\n\n".join(parts)


def _normalize_model_name(name: str) -> str:
    """规范化模型名称，将版本号中的连字符转为点号。

    例如：
      'claude-sonnet-4-6'        → 'claude-sonnet-4.6'
      'claude-opus-4-1'          → 'claude-opus-4.1'
      'claude-haiku-3-5'         → 'claude-haiku-3.5'
      'claude-sonnet-4-6-20250514' → 'claude-sonnet-4.6-20250514'
      'gpt-4o-2024-05-13'        → 'gpt-4o-2024-05-13'  (日期格式不变)
      'deepseek-r1'              → 'deepseek-r1'         (非版本号不变)
    """
    if not name:
        return name

    # 匹配模型版本号：字母后跟 -主版本(1位)-次版本(1位)，后面可选跟日期后缀
    # 如 sonnet-4-6、opus-4-1、haiku-3-5
    # 仅转换紧跟字母后的 单数字-单数字 模式（区分于日期中的 2024-05-13）
    normalized = re.sub(r"([a-zA-Z])-(\d)-(\d)(?=$|-)", r"\1-\2.\3", name)
    return normalized


def _pick_workbuddy_model_name(provider_data) -> str:
    """从 WorkBuddy 的 providerData 提取最友好的模型名。

    providerData 可能是 dict 或 JSON 字符串，含 4 个字段，按以下优先级取：
      1. requestModelName  人类可读长名，如 "Claude-Opus-4.7 (1M context)"
      2. model             短 ID，如 "claude-opus-4.7-1m"
      3. requestModelId    短 ID 备份
    短 ID 会再过 _normalize_model_name 把 4-6 → 4.6；长名直接返回。
    """
    if isinstance(provider_data, str) and provider_data:
        try:
            provider_data = json.loads(provider_data)
        except (json.JSONDecodeError, TypeError):
            return ""
    if not isinstance(provider_data, dict):
        return ""
    long_name = (provider_data.get("requestModelName") or "").strip()
    if long_name:
        return long_name
    short = (provider_data.get("model") or provider_data.get("requestModelId") or "").strip()
    return _normalize_model_name(short) if short else ""


def _extract_model_name_from_message_obj(msg_obj: dict) -> str:
    """从 Cursor/ClaudeCode 的 message 对象中提取模型名（多字段兜底）。"""
    if not isinstance(msg_obj, dict):
        return ""

    candidates = [
        msg_obj.get("model", ""),
        msg_obj.get("modelName", ""),
        msg_obj.get("model_name", ""),
        msg_obj.get("modelId", ""),
        msg_obj.get("model_id", ""),
    ]
    for value in candidates:
        text = str(value).strip()
        if text:
            return _normalize_model_name(text)

    meta = msg_obj.get("metadata", {})
    if isinstance(meta, dict):
        meta_candidates = [
            meta.get("model", ""),
            meta.get("modelName", ""),
            meta.get("model_name", ""),
            meta.get("modelId", ""),
            meta.get("model_id", ""),
        ]
        for value in meta_candidates:
            text = str(value).strip()
            if text:
                return _normalize_model_name(text)

    return ""


def _count_text_segments(text: str) -> int:
    """统计字符串里非空段落数量（用 \\n\\n 分段，与渲染时段落切分规则一致）。

    口径必须与 `_render_assistant_message_interleaved` 中的 paragraphs 过滤完全一致：
      - 跳过纯空白段
      - 跳过裸 "-" 占位段（部分 IDE 流式落盘的空 text 块会留下 "-" 残影）

    否则提取器 position 与渲染器段落索引不对齐，会导致 last_tool_position 计算虚高、
    本应是正文的段落（如最终结论）被错误归入 timeline。
    """
    if not text or not text.strip():
        return 0
    return sum(
        1
        for p in re.split(r"\n\s*\n", text)
        if p.strip() and p.strip() != "-"
    )


def _shift_tool_call_positions(tool_calls: list[dict], offset: int) -> list[dict]:
    """把 tool_calls 列表里每个元素的 position 字段加上 offset（原地修改并返回）。

    若元素没有 position 字段，则补一个 = offset（视为出现在已有所有段之后）。
    """
    if offset <= 0 or not tool_calls:
        # offset 为 0 也可能需要补全 position，统一走下方逻辑
        for tc in tool_calls:
            if isinstance(tc, dict) and "position" not in tc:
                tc["position"] = offset
        return tool_calls
    for tc in tool_calls:
        if not isinstance(tc, dict):
            continue
        tc["position"] = int(tc.get("position", 0)) + offset
    return tool_calls


def _extract_claudecode_tool_calls(content) -> list[dict]:
    """从 Claude Code assistant 消息的 content 字段提取 tool_use 块。

    Returns:
        [{"toolCallId": "...", "toolName": "...", "args": {...}, "position": N}, ...]
        其中 position 表示该工具调用在原 content 数组中前面有多少个非空 text 块，
        用于在渲染时把工具调用穿插回 content 段落之间，保持原始时序。
    """
    if not isinstance(content, list):
        return []

    tool_calls = []
    text_count = 0  # 记录已经遇到的非空 markdown 段落数量（与渲染层 \n\n 切分口径一致）
    for item in content:
        if not isinstance(item, dict):
            continue
        item_type = item.get("type")
        if item_type == "text":
            t = item.get("text", "")
            if isinstance(t, str) and t.strip():
                # 一个 text 块内部可能含多个 \n\n 段落，必须按渲染层口径分段计数
                text_count += _count_text_segments(t)
            continue
        if item_type != "tool_use":
            continue
        tool_id = item.get("id", "")
        tool_name = item.get("name", "")
        args = item.get("input", {})
        if not isinstance(args, dict):
            try:
                args = json.loads(args) if args else {}
            except Exception:
                args = {}
        tool_calls.append({
            "toolCallId": tool_id,
            "toolName": tool_name,
            "args": args,
            "position": text_count,
        })
    return tool_calls


def _extract_claudecode_tool_results(content) -> dict[str, str]:
    """从 Claude Code user 消息的 content 字段提取 tool_result 块。

    Returns:
        {"tool_use_id": "result_text_preview", ...}
    """
    if not isinstance(content, list):
        return {}

    results = {}
    for item in content:
        if not isinstance(item, dict):
            continue
        if item.get("type") != "tool_result":
            continue
        call_id = item.get("tool_use_id", "")
        if not call_id:
            continue
        raw = item.get("content", "")
        if isinstance(raw, list):
            parts = []
            for r in raw:
                if isinstance(r, dict) and r.get("text"):
                    parts.append(r["text"])
                elif isinstance(r, str):
                    parts.append(r)
            result_str = "\n".join(parts)
        elif isinstance(raw, dict):
            result_str = json.dumps(raw, ensure_ascii=False)
        else:
            result_str = str(raw) if raw else ""

        results[call_id] = result_str
    return results


def _extract_cursor_user_text(content) -> str:
    """从 Cursor user 消息 content 提取用户输入文本。"""
    if isinstance(content, str):
        return _normalize_image_placeholders(
            _clean_internal_prompt_text(content, role="user")
        )
    if isinstance(content, list):
        texts = []
        for item in content:
            if not isinstance(item, dict):
                continue
            if item.get("type") == "text":
                t = _clean_internal_prompt_text(item.get("text", ""), role="user")
                if t:
                    texts.append(_normalize_image_placeholders(t))
        return "\n".join(texts)
    return ""


def _extract_cursor_assistant_text(content) -> str:
    """从 Cursor assistant 消息 content 提取文本回复。"""
    if isinstance(content, str):
        return _clean_internal_prompt_text(content, role="assistant")
    if isinstance(content, list):
        texts = []
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                t = _clean_internal_prompt_text(item.get("text", ""), role="assistant")
                if t:
                    texts.append(t)
        return "\n\n".join(texts)
    return ""


def _extract_cursor_tool_calls(content) -> list[dict]:
    """从 Cursor assistant content 提取 tool_use 块。

    每个 tool_call 带 `position` 字段：表示其在原 content 数组中前置非空 text 块的数量，
    用于渲染时按时序穿插。
    """
    if not isinstance(content, list):
        return []
    tool_calls = []
    text_count = 0
    for item in content:
        if not isinstance(item, dict):
            continue
        item_type = item.get("type")
        if item_type == "text":
            t = item.get("text", "")
            if isinstance(t, str) and t.strip():
                # 一个 text 块内部可能含多个 \n\n 段落，必须按渲染层口径分段计数
                text_count += _count_text_segments(t)
            continue
        if item_type != "tool_use":
            continue
        tool_id = item.get("id", "")
        tool_name = item.get("name", "")
        args = item.get("input", {})
        if not isinstance(args, dict):
            try:
                args = json.loads(args) if args else {}
            except Exception:
                args = {}
        tool_calls.append({
            "toolCallId": tool_id,
            "toolName": tool_name,
            "args": args,
            "position": text_count,
        })
    return tool_calls


# Claude Code 工具名映射（Claude Code 使用 PascalCase 工具名）
CLAUDECODE_TOOL_NAME_MAP: dict[str, tuple[str, str]] = {
    "Read":          ("📄", "读取文件"),
    "Write":         ("📝", "写入文件"),
    "Edit":          ("✏️", "编辑文件"),
    "MultiEdit":     ("✏️", "多处编辑"),
    "Bash":          ("⚡", "执行命令"),
    "Glob":          ("📁", "搜索文件"),
    "Grep":          ("🔍", "搜索内容"),
    "Agent":         ("🤖", "子任务"),
    "WebSearch":     ("🌐", "网络搜索"),
    "WebFetch":      ("🌐", "获取网页"),
    "TodoWrite":     ("📋", "任务管理"),
    "NotebookEdit":  ("📓", "编辑笔记本"),
    "EnterPlanMode": ("🗂️", "进入规划模式"),
    "ExitPlanMode":  ("✅", "退出规划模式"),
    "Task":          ("🤖", "子任务"),
    # Cursor transcript 常见工具名
    "ReadFile":      ("📄", "读取文件"),
    "WriteFile":     ("📝", "写入文件"),
    "EditFile":      ("✏️", "编辑文件"),
    "Shell":         ("⚡", "执行命令"),
    "ApplyPatch":    ("✏️", "应用补丁"),
    "SemanticSearch": ("🔍", "语义搜索"),
    "SwitchMode":    ("🗂️", "切换模式"),
}


def _humanize_claudecode_tool_name(tool_name: str) -> str:
    """将 Claude Code 工具名转换为人类可读的 图标+标签 格式。"""
    if tool_name in CLAUDECODE_TOOL_NAME_MAP:
        icon, label = CLAUDECODE_TOOL_NAME_MAP[tool_name]
        return f"{icon} {label}"
    return f"🔧 {tool_name}"


def _summarize_claudecode_args(tool_name: str, args: dict) -> str:
    """根据 Claude Code 工具类型，提取最有价值的参数摘要。"""
    if tool_name in ("Read", "Write", "Edit", "MultiEdit", "NotebookEdit"):
        path = args.get("file_path", args.get("notebook_path", ""))
        return os.path.basename(path) if path else ""
    elif tool_name == "Bash":
        cmd = args.get("command", "")
        return (cmd[:60] + "...") if len(cmd) > 60 else cmd
    elif tool_name in ("Glob",):
        return args.get("pattern", "")
    elif tool_name in ("Grep",):
        return args.get("pattern", "")
    elif tool_name == "WebSearch":
        return args.get("query", "")[:60]
    elif tool_name == "WebFetch":
        return args.get("url", "")[:60]
    elif tool_name == "Agent":
        desc = args.get("description", args.get("prompt", ""))
        return desc[:60] if desc else ""
    else:
        for v in args.values():
            if isinstance(v, str) and v:
                return v[:50]
        return ""


def list_claudecode_conversations(
    workspace_dir: str, extra_projects_dirs: Optional[list[str]] = None
) -> list[dict]:
    """列出 Claude Code 工作区下所有会话，按最近修改时间倒序。"""
    session_dir = find_claudecode_session_dir(
        workspace_dir, extra_projects_dirs=extra_projects_dirs
    )
    if not session_dir:
        return []

    conversations = []
    try:
        files = os.listdir(session_dir)
    except OSError:
        return []

    for fname in files:
        if not fname.endswith(".jsonl"):
            continue
        conv_id = fname[:-6]  # strip .jsonl
        fpath = os.path.join(session_dir, fname)
        mtime = os.path.getmtime(fpath)

        # Count messages and get first user text as preview
        msg_count = 0
        first_user_text = ""
        try:
            with open(fpath, "r", encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        obj = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    t = obj.get("type")
                    if t in ("user", "assistant"):
                        msg_count += 1
                    if t == "user" and not first_user_text:
                        msg_obj = obj.get("message", {})
                        text = _extract_claudecode_user_text(msg_obj.get("content", ""))
                        if text and text.strip():
                            first_user_text = text.strip()
        except OSError:
            continue

        conversations.append({
            "id": conv_id,
            "message_count": msg_count,
            "request_count": 0,
            "last_modified": datetime.fromtimestamp(mtime).isoformat(),
            "preview": first_user_text[:100] if first_user_text else "(empty)",
            "source": "claudecode",
        })

    conversations.sort(key=lambda c: c["last_modified"], reverse=True)
    return conversations


def read_claudecode_conversation(
    workspace_dir: str,
    conversation_id: str,
    extra_projects_dirs: Optional[list[str]] = None,
) -> list[dict]:
    """读取 Claude Code 指定会话的完整消息列表。

    Returns:
        [{"role": "user"/"assistant", "content": "...", "tool_calls": [...], "model_name": "..."}, ...]
    """
    session_dir = find_claudecode_session_dir(
        workspace_dir, extra_projects_dirs=extra_projects_dirs
    )
    if not session_dir:
        print(f"❌ 未找到工作区 '{workspace_dir}' 的 Claude Code 历史记录目录", file=sys.stderr)
        sys.exit(1)

    jsonl_path = os.path.join(session_dir, f"{conversation_id}.jsonl")
    if not os.path.exists(jsonl_path):
        print(f"❌ 未找到 Claude Code 会话文件: {jsonl_path}", file=sys.stderr)
        sys.exit(1)

    # 收集所有记录
    records = []
    with open(jsonl_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError:
                continue

    # 按 timestamp 排序（JSONL 本身是追加顺序，但保险起见）
    records.sort(key=lambda r: r.get("timestamp", ""))

    # 累积 tool_result：user 消息中可能包含 tool_result 块，需回填给前一条 assistant 消息
    pending_tool_calls: dict[str, dict] = {}  # toolCallId -> enriched call dict

    conversation: list[dict] = []

    for record in records:
        rec_type = record.get("type")
        if rec_type not in ("user", "assistant"):
            continue

        msg_obj = record.get("message", {})
        if not isinstance(msg_obj, dict):
            continue

        content = msg_obj.get("content", "")

        if rec_type == "user":
            # 先处理其中的 tool_result（回填到 pending_tool_calls）
            tool_results = _extract_claudecode_tool_results(content)
            for call_id, result_str in tool_results.items():
                if call_id in pending_tool_calls:
                    preview = _clean_tool_result_preview(result_str)
                    if len(preview) > 200:
                        preview = preview[:197] + "..."
                    pending_tool_calls[call_id]["result_preview"] = preview

            # 提取用户真正输入的文本
            text = _extract_claudecode_user_text(content)
            if not text or not text.strip():
                continue

            # 处理 isMeta 消息：Claude Code 斜杠命令的调度模板（跨语言通用）
            # isMeta=true 时，真实用户输入位于 ARGUMENTS: 之后；无 ARGUMENTS 则为纯平台注入，丢弃整条
            if record.get("isMeta"):
                real_input = _extract_claudecode_meta_user_input(text)
                if real_input:
                    text = real_input
                else:
                    continue

            if conversation and conversation[-1]["role"] == "user":
                conversation[-1]["content"] += "\n\n" + text
            else:
                conversation.append({"role": "user", "content": text})

        elif rec_type == "assistant":
            # 跳过 Claude Code 客户端注入的合成占位消息（如 "No response requested."）
            if msg_obj.get("model") == "<synthetic>":
                continue

            text = _extract_claudecode_assistant_text(content)
            thinking = _extract_anthropic_thinking(content)
            tool_calls_raw = _extract_claudecode_tool_calls(content)
            model_name = _extract_model_name_from_message_obj(msg_obj)

            # 为每个 tool_call 添加人类可读信息
            enriched_calls = []
            for tc in tool_calls_raw:
                enriched = {
                    "toolCallId": tc["toolCallId"],
                    "toolName": tc["toolName"],
                    "displayName": _humanize_claudecode_tool_name(tc["toolName"]),
                    "argsSummary": _summarize_claudecode_args(tc["toolName"], tc.get("args", {})),
                    "result_preview": "",
                    "isError": False,
                    "position": tc.get("position", 0),
                }
                pending_tool_calls[tc["toolCallId"]] = enriched
                enriched_calls.append(enriched)

            if not text and not enriched_calls and not thinking:
                continue

            # 合并连续 assistant 消息
            if conversation and conversation[-1]["role"] == "assistant":
                # 关键：在合并 text 之前先记下旧 content 的段数，作为 enriched_calls 的 position 偏移
                prev_segments = _count_text_segments(conversation[-1].get("content", ""))
                if text:
                    prev = conversation[-1].get("content", "")
                    conversation[-1]["content"] = (prev + "\n\n" + text) if prev else text
                if thinking:
                    prev_th = conversation[-1].get("thinking", "")
                    conversation[-1]["thinking"] = (prev_th + "\n\n" + thinking) if prev_th else thinking
                if enriched_calls:
                    _shift_tool_call_positions(enriched_calls, prev_segments)
                    conversation[-1].setdefault("tool_calls", []).extend(enriched_calls)
                if model_name and not conversation[-1].get("model_name"):
                    conversation[-1]["model_name"] = model_name
            else:
                entry: dict = {"role": "assistant", "content": text or ""}
                if thinking:
                    entry["thinking"] = thinking
                if enriched_calls:
                    entry["tool_calls"] = enriched_calls
                if model_name:
                    entry["model_name"] = model_name
                conversation.append(entry)

    # 清理空消息
    conversation = [
        m for m in conversation
        if m.get("content", "").strip() or m.get("tool_calls") or m.get("thinking", "").strip()
    ]
    return conversation


# ============ WorkBuddy JSONL Functions ============


def _extract_workbuddy_user_text(content: list) -> str:
    """从 WorkBuddy user 消息的 content 数组中提取用户真实文本。

    WorkBuddy content 格式: [{"type": "input_text", "text": "..."}]
    需要过滤掉系统注入的内容（<system-reminder> 等标签）。

    图片标记优先由文本中的 [Image #N] 等占位符统一替换（位置准确），
    input_image block 仅在没有文本标记时作为兜底产出 [图片]。
    """
    if not isinstance(content, list):
        return ""

    texts = []
    has_image_block = False
    for block in content:
        if not isinstance(block, dict):
            continue

        block_type = block.get("type", "")

        if block_type in ("image", "input_image"):
            has_image_block = True
            continue

        if block_type != "input_text":
            continue

        raw_text = block.get("text", "")
        if not raw_text:
            continue

        # 复用 Claude Code 的系统注入过滤逻辑
        cleaned = _SYSTEM_TAG_PATTERN.sub("", raw_text)
        cleaned = _SYSTEM_TAG_ORPHAN_PATTERN.sub("", cleaned)
        cleaned = cleaned.strip()

        if not cleaned:
            continue
        if _is_claudecode_system_injection(cleaned):
            continue

        # 统一替换图片占位符（保留文本中的正确位置，合并重复标记）
        cleaned = _normalize_image_placeholders(cleaned)

        # 提取 <user_query> 标签内容（如果有）
        uq_match = re.search(
            r"<user_query>([\s\S]*?)</user_query>", cleaned, re.IGNORECASE
        )
        if uq_match:
            texts.append(uq_match.group(1).strip())
        else:
            texts.append(cleaned)

    # 兜底：有 image block 但文本中没有产生 [图片] 标记
    if has_image_block and not any("[图片]" in t for t in texts):
        texts.insert(0, "[图片]")

    return "\n\n".join(texts)


def _extract_workbuddy_assistant_text(content: list) -> str:
    """从 WorkBuddy assistant 消息的 content 数组中提取助手文本回复。

    WorkBuddy content 格式: [{"type": "output_text", "text": "..."}]
    过滤掉 tool_use 等非文本块。
    """
    if not isinstance(content, list):
        return ""

    texts = []
    for block in content:
        if not isinstance(block, dict):
            continue
        block_type = block.get("type", "")
        if block_type == "output_text":
            text = block.get("text", "")
            if text and text.strip():
                texts.append(text.strip())
        elif block_type == "text":
            # 兼容可能的 Claude Code 风格 text 块
            text = block.get("text", "")
            if text and text.strip():
                texts.append(text.strip())

    return "\n\n".join(texts)


def _extract_workbuddy_tool_calls(content: list) -> list[dict]:
    """从 WorkBuddy assistant 消息的 content 数组中提取工具调用。

    每个 tool_call 带 `position` 字段：表示其在原 content 数组中前置非空 text 块的数量，
    用于渲染时按时序穿插。WorkBuddy 文本块类型可能是 output_text 或 text。
    """
    if not isinstance(content, list):
        return []

    calls = []
    text_count = 0
    for block in content:
        if not isinstance(block, dict):
            continue
        block_type = block.get("type", "")
        if block_type in ("output_text", "text"):
            t = block.get("text", "")
            if isinstance(t, str) and t.strip():
                # 一个 text 块内部可能含多个 \n\n 段落，必须按渲染层口径分段计数
                text_count += _count_text_segments(t)
            continue
        if block_type != "tool_use":
            continue
        calls.append({
            "toolCallId": block.get("id", ""),
            "toolName": block.get("name", ""),
            "args": block.get("input", {}),
            "position": text_count,
        })
    return calls


def list_workbuddy_conversations(
    workspace_dir: str, extra_projects_dirs: Optional[list[str]] = None
) -> list[dict]:
    """列出 WorkBuddy 工作区下所有会话，按最近修改时间倒序。"""
    session_dir = find_workbuddy_session_dir(
        workspace_dir, extra_projects_dirs=extra_projects_dirs
    )
    if not session_dir:
        return []

    conversations = []
    try:
        files = os.listdir(session_dir)
    except OSError:
        return []

    for fname in files:
        if not fname.endswith(".jsonl"):
            continue
        conv_id = fname[:-6]  # strip .jsonl
        fpath = os.path.join(session_dir, fname)
        mtime = os.path.getmtime(fpath)

        msg_count = 0
        first_user_text = ""
        try:
            with open(fpath, "r", encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        obj = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if obj.get("type") != "message":
                        continue
                    role = obj.get("role")
                    if role in ("user", "assistant"):
                        msg_count += 1
                    if role == "user" and not first_user_text:
                        text = _extract_workbuddy_user_text(obj.get("content", []))
                        if text and text.strip():
                            first_user_text = text.strip()
        except OSError:
            continue

        conversations.append({
            "id": conv_id,
            "message_count": msg_count,
            "request_count": 0,
            "last_modified": datetime.fromtimestamp(mtime).isoformat(),
            "preview": first_user_text[:100] if first_user_text else "(empty)",
            "source": "workbuddy",
        })

    conversations.sort(key=lambda c: c["last_modified"], reverse=True)
    return conversations


def read_workbuddy_conversation(
    workspace_dir: str,
    conversation_id: str,
    extra_projects_dirs: Optional[list[str]] = None,
) -> list[dict]:
    """读取 WorkBuddy 指定会话的完整消息列表。

    Returns:
        [{"role": "user"/"assistant", "content": "...", "tool_calls": [...], "model_name": "..."}, ...]
    """
    session_dir = find_workbuddy_session_dir(
        workspace_dir, extra_projects_dirs=extra_projects_dirs
    )
    if not session_dir:
        print(f"❌ 未找到工作区 '{workspace_dir}' 的 WorkBuddy 历史记录目录", file=sys.stderr)
        sys.exit(1)

    jsonl_path = os.path.join(session_dir, f"{conversation_id}.jsonl")
    if not os.path.exists(jsonl_path):
        print(f"❌ 未找到 WorkBuddy 会话文件: {jsonl_path}", file=sys.stderr)
        sys.exit(1)

    records = []
    with open(jsonl_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError:
                continue

    # WorkBuddy 的 timestamp 是字符串毫秒（如 "1780038450236"），需转 int 排序
    def _ts(r):
        v = r.get("timestamp", 0)
        try:
            return int(v)
        except (TypeError, ValueError):
            return 0

    records.sort(key=_ts)

    pending_tool_calls: dict[str, dict] = {}
    conversation: list[dict] = []

    def _ensure_assistant_entry() -> dict:
        """获取/创建当前可写入的 assistant entry（最后一条非 user 即可）。"""
        if conversation and conversation[-1]["role"] == "assistant":
            return conversation[-1]
        entry: dict = {"role": "assistant", "content": ""}
        conversation.append(entry)
        return entry

    for record in records:
        rec_type = record.get("type")

        # ========== 顶层 message 记录 ==========
        if rec_type == "message":
            role = record.get("role")
            content = record.get("content", [])

            if role == "user":
                # 兼容 Claude Code 风格：user 消息里可能含 tool_result block
                if isinstance(content, list):
                    for block in content:
                        if isinstance(block, dict) and block.get("type") == "tool_result":
                            call_id = block.get("tool_use_id", "")
                            if call_id and call_id in pending_tool_calls:
                                raw = block.get("content", "")
                                preview = _clean_tool_result_preview(str(raw))
                                if len(preview) > 200:
                                    preview = preview[:197] + "..."
                                pending_tool_calls[call_id]["result_preview"] = preview

                text = _extract_workbuddy_user_text(content)
                if not text or not text.strip():
                    continue

                if conversation and conversation[-1]["role"] == "user":
                    conversation[-1]["content"] += "\n\n" + text
                else:
                    conversation.append({"role": "user", "content": text})
                continue

            if role == "assistant":
                text = _extract_workbuddy_assistant_text(content)
                thinking = _extract_anthropic_thinking(content)

                # 兼容老格式：content 内部可能直接含 tool_use（早期 WorkBuddy）
                tool_calls_raw = _extract_workbuddy_tool_calls(content)

                model_name = _pick_workbuddy_model_name(record.get("providerData"))

                inline_enriched = []
                for tc in tool_calls_raw:
                    enriched = {
                        "toolCallId": tc["toolCallId"],
                        "toolName": tc["toolName"],
                        "displayName": _humanize_claudecode_tool_name(tc["toolName"]),
                        "argsSummary": _summarize_claudecode_args(tc["toolName"], tc.get("args", {})),
                        "result_preview": "",
                        "isError": False,
                        "position": tc.get("position", 0),
                    }
                    if enriched["toolCallId"]:
                        pending_tool_calls[enriched["toolCallId"]] = enriched
                    inline_enriched.append(enriched)

                if not text and not inline_enriched and not thinking:
                    continue

                # 合并到当前 assistant entry（同一 turn 可能含多条 assistant message + 多个 function_call）
                if conversation and conversation[-1]["role"] == "assistant":
                    entry = conversation[-1]
                    prev_segments = _count_text_segments(entry.get("content", ""))
                    if text:
                        prev = entry.get("content", "")
                        entry["content"] = (prev + "\n\n" + text) if prev else text
                    if thinking:
                        prev_th = entry.get("thinking", "")
                        entry["thinking"] = (prev_th + "\n\n" + thinking) if prev_th else thinking
                    if inline_enriched:
                        _shift_tool_call_positions(inline_enriched, prev_segments)
                        entry.setdefault("tool_calls", []).extend(inline_enriched)
                    if model_name and not entry.get("model_name"):
                        entry["model_name"] = model_name
                else:
                    entry = {"role": "assistant", "content": text or ""}
                    if thinking:
                        entry["thinking"] = thinking
                    if inline_enriched:
                        entry["tool_calls"] = inline_enriched
                    if model_name:
                        entry["model_name"] = model_name
                    conversation.append(entry)
                continue

            # 其它 role 跳过
            continue

        # ========== 顶层 function_call 记录（WorkBuddy 主格式） ==========
        if rec_type == "function_call":
            call_id = record.get("callId", "")
            tool_name = record.get("name", "")
            args_raw = record.get("arguments", "")
            args: dict
            if isinstance(args_raw, str):
                try:
                    args = json.loads(args_raw) if args_raw else {}
                except (json.JSONDecodeError, TypeError):
                    args = {"_raw": args_raw}
            elif isinstance(args_raw, dict):
                args = args_raw
            else:
                args = {"_value": args_raw}

            entry = _ensure_assistant_entry()
            position = _count_text_segments(entry.get("content", ""))
            enriched = {
                "toolCallId": call_id,
                "toolName": tool_name,
                "displayName": _humanize_claudecode_tool_name(tool_name),
                "argsSummary": _summarize_claudecode_args(tool_name, args),
                "result_preview": "",
                "isError": False,
                "position": position,
            }
            if call_id:
                pending_tool_calls[call_id] = enriched
            entry.setdefault("tool_calls", []).append(enriched)

            # 兼容：function_call 上也可能带 providerData 含模型名
            if not entry.get("model_name"):
                m = _pick_workbuddy_model_name(record.get("providerData"))
                if m:
                    entry["model_name"] = m
            continue

        # ========== 顶层 function_call_result 记录 ==========
        if rec_type == "function_call_result":
            call_id = record.get("callId", "")
            if not call_id or call_id not in pending_tool_calls:
                continue
            output = record.get("output", "")
            # output 可能是 JSON 字符串、dict、list
            if isinstance(output, (dict, list)):
                try:
                    output_str = json.dumps(output, ensure_ascii=False)
                except (TypeError, ValueError):
                    output_str = str(output)
            else:
                output_str = str(output or "")
                # WorkBuddy 常用 {"type":"text","text":"..."} 包装，尝试解开
                try:
                    parsed = json.loads(output_str)
                    if isinstance(parsed, dict) and parsed.get("type") == "text" and isinstance(parsed.get("text"), str):
                        output_str = parsed["text"]
                except (json.JSONDecodeError, TypeError):
                    pass
            preview = _clean_tool_result_preview(output_str)
            if len(preview) > 200:
                preview = preview[:197] + "..."
            pending_tool_calls[call_id]["result_preview"] = preview
            status = (record.get("status") or "").lower()
            pending_tool_calls[call_id]["isError"] = status not in ("", "completed", "success", "ok")
            continue

        # 其它顶层 type（file-history-snapshot / ai-title 等）忽略

    # 清理：丢弃完全空的 entry
    conversation = [
        m for m in conversation
        if m.get("content", "").strip() or m.get("tool_calls") or m.get("thinking", "").strip()
    ]
    return conversation


# ============ Box AI SQLite Functions ============


def list_boxai_conversations(
    workspace_dir: str, extra_db_path: Optional[str] = None
) -> list[dict]:
    """列出 Box AI 所有会话，按最近更新时间倒序。"""
    import sqlite3

    db_path = get_boxai_db_path(extra_path=extra_db_path)
    if not db_path:
        return []

    conversations = []
    try:
        conn = sqlite3.connect(db_path)
        conn.row_factory = sqlite3.Row
        cur = conn.cursor()

        cur.execute(
            "SELECT id, title, mode, work_dir, summary, created_at, updated_at "
            "FROM sessions ORDER BY updated_at DESC"
        )
        for row in cur.fetchall():
            session_id = row["id"]
            # 计算消息数
            cur2 = conn.cursor()
            cur2.execute(
                "SELECT COUNT(*) as cnt FROM messages WHERE session_id=? AND role IN ('user','assistant')",
                (session_id,),
            )
            msg_count = cur2.fetchone()["cnt"]

            # 获取第一条用户消息作为预览
            cur2.execute(
                "SELECT content FROM messages WHERE session_id=? AND role='user' ORDER BY id LIMIT 1",
                (session_id,),
            )
            preview_row = cur2.fetchone()
            first_user_text = preview_row["content"][:100] if preview_row else "(empty)"

            conversations.append({
                "id": session_id,
                "message_count": msg_count,
                "request_count": 0,
                "last_modified": row["updated_at"] or row["created_at"] or "",
                "preview": first_user_text,
                "source": "boxai",
                "title": row["title"] or "",
            })

        conn.close()
    except Exception as e:
        print(f"⚠️  读取 Box AI 数据库失败: {e}", file=sys.stderr)
        return []

    return conversations


def read_boxai_conversation(
    workspace_dir: str,
    conversation_id: str,
    extra_db_path: Optional[str] = None,
) -> list[dict]:
    """读取 Box AI 指定会话的完整消息列表。

    Returns:
        [{"role": "user"/"assistant", "content": "...", "model_name": "..."}, ...]
    """
    import sqlite3

    db_path = get_boxai_db_path(extra_path=extra_db_path)
    if not db_path:
        print("❌ 未找到 Box AI sessions.db 数据库文件", file=sys.stderr)
        sys.exit(1)

    try:
        conn = sqlite3.connect(db_path)
        conn.row_factory = sqlite3.Row
        cur = conn.cursor()

        # 从 sessions 表获取会话级别的 model 信息
        cur.execute(
            "SELECT metadata FROM sessions WHERE id=?",
            (conversation_id,),
        )
        session_row = cur.fetchone()
        session_model_name = ""
        if session_row and session_row["metadata"]:
            try:
                session_meta = json.loads(session_row["metadata"])
                if isinstance(session_meta, dict):
                    # 优先取 modelConfig.model，其次 selectedModelId
                    model_config = session_meta.get("modelConfig", {})
                    if isinstance(model_config, dict):
                        session_model_name = model_config.get("model", "")
                    if not session_model_name:
                        session_model_name = session_meta.get("selectedModelId", "")
            except (json.JSONDecodeError, TypeError):
                pass
        # 规范化模型名（如 claude-sonnet-4-6 → claude-sonnet-4.6）
        session_model_name = _normalize_model_name(session_model_name)

        cur.execute(
            "SELECT role, content, reasoning_content, tool_calls, tool_results, timestamp "
            "FROM messages WHERE session_id=? ORDER BY id",
            (conversation_id,),
        )
        rows = cur.fetchall()
        conn.close()
    except Exception as e:
        print(f"❌ 读取 Box AI 数据库失败: {e}", file=sys.stderr)
        sys.exit(1)

    if not rows:
        print(f"❌ 未找到 Box AI 会话 '{conversation_id}' 或会话无消息", file=sys.stderr)
        sys.exit(1)

    conversation: list[dict] = []
    # tool_call_id -> 对应 enriched dict 的引用，便于 role=='tool' 行回填 result
    pending_tool_calls: dict[str, dict] = {}

    def _parse_json_col(val) -> list:
        """安全解析 Box AI tool_calls / tool_results 列（可能是 JSON 字符串、None、空串）。"""
        if not val:
            return []
        if isinstance(val, (list, dict)):
            return val if isinstance(val, list) else [val]
        try:
            parsed = json.loads(val)
        except (json.JSONDecodeError, TypeError):
            return []
        if isinstance(parsed, list):
            return parsed
        if isinstance(parsed, dict):
            return [parsed]
        return []

    for row in rows:
        role = row["role"]
        content = _normalize_image_placeholders(row["content"] or "")
        # Box AI 在数据库列里直接存了 reasoning_content（链式思考），仅 assistant 有
        try:
            reasoning = row["reasoning_content"] or ""
        except (IndexError, KeyError):
            reasoning = ""
        try:
            raw_tool_calls = row["tool_calls"]
        except (IndexError, KeyError):
            raw_tool_calls = None
        try:
            raw_tool_results = row["tool_results"]
        except (IndexError, KeyError):
            raw_tool_results = None

        # role == "tool"：把 tool_results 回填到上一条 assistant 的对应 tool_call
        if role == "tool":
            results = _parse_json_col(raw_tool_results)
            # 兜底：若 tool_results 列为空，但 content 列里有 JSON（部分版本会这么存），
            # 当作单个 result 文本处理 —— 此时无法对应到具体 toolCallId，跳过。
            for r in results:
                if not isinstance(r, dict):
                    continue
                call_id = r.get("toolCallId") or r.get("tool_call_id") or ""
                output = r.get("output", "")
                if isinstance(output, (dict, list)):
                    try:
                        output = json.dumps(output, ensure_ascii=False)
                    except (TypeError, ValueError):
                        output = str(output)
                preview = _clean_tool_result_preview(str(output))
                if len(preview) > 200:
                    preview = preview[:197] + "..."
                if call_id and call_id in pending_tool_calls:
                    pending_tool_calls[call_id]["result_preview"] = preview
                    pending_tool_calls[call_id]["isError"] = bool(r.get("isError", False))
            continue

        if role not in ("user", "assistant"):
            continue

        # 解析 assistant 的 tool_calls 列 → enriched schema（与 codebuddy 路径对齐）
        enriched_calls: list[dict] = []
        if role == "assistant":
            for tc in _parse_json_col(raw_tool_calls):
                if not isinstance(tc, dict):
                    continue
                tool_name = tc.get("name") or tc.get("toolName") or ""
                args = tc.get("arguments")
                if args is None:
                    args = tc.get("args", {})
                # arguments 可能是 JSON 字符串
                if isinstance(args, str):
                    try:
                        args = json.loads(args)
                    except (json.JSONDecodeError, TypeError):
                        args = {"_raw": args}
                if not isinstance(args, dict):
                    args = {"_value": args}
                call_id = tc.get("id") or tc.get("toolCallId") or ""
                enriched = {
                    "toolCallId": call_id,
                    "toolName": tool_name,
                    "displayName": _humanize_tool_name(tool_name),
                    "argsSummary": _summarize_args(tool_name, args),
                    "result_preview": "",
                    "isError": False,
                    "position": 0,  # 默认 0，下面合并阶段再按 prev_segments 偏移
                }
                if call_id:
                    pending_tool_calls[call_id] = enriched
                enriched_calls.append(enriched)

        # 跳过纯空消息：content/reasoning/tool_calls 都没有
        if (
            not content.strip()
            and not (role == "assistant" and reasoning.strip())
            and not enriched_calls
        ):
            continue

        # 合并连续同角色消息
        if conversation and conversation[-1]["role"] == role:
            # 在合并 content 之前先记下旧段数，作为新 tool_calls 的 position 偏移
            prev_segments = (
                _count_text_segments(conversation[-1].get("content", ""))
                if role == "assistant"
                else 0
            )
            if content.strip():
                conversation[-1]["content"] = (
                    (conversation[-1]["content"] + "\n\n" + content)
                    if conversation[-1].get("content")
                    else content
                )
            if role == "assistant" and reasoning.strip():
                prev_th = conversation[-1].get("thinking", "")
                conversation[-1]["thinking"] = (
                    (prev_th + "\n\n" + reasoning) if prev_th else reasoning
                )
            if role == "assistant" and enriched_calls:
                # 当前这批 tool_calls 出现在「合并后内容」的当前段之后
                cur_segments = _count_text_segments(content)
                _shift_tool_call_positions(enriched_calls, prev_segments + cur_segments)
                prev_calls = conversation[-1].get("tool_calls", [])
                conversation[-1]["tool_calls"] = prev_calls + enriched_calls
        else:
            entry: dict = {"role": role, "content": content}
            if role == "assistant" and reasoning.strip():
                entry["thinking"] = reasoning
            if role == "assistant" and enriched_calls:
                # 该批 tool_calls 出现在 content 切完后所有段之后
                _shift_tool_call_positions(enriched_calls, _count_text_segments(content))
                entry["tool_calls"] = enriched_calls
            if role == "assistant" and session_model_name:
                entry["model_name"] = session_model_name
            conversation.append(entry)

    return conversation


def list_cursor_conversations(
    workspace_dir: str, extra_projects_dirs: Optional[list[str]] = None
) -> list[dict]:
    """列出 Cursor 工作区下所有会话，按最近修改时间倒序。"""
    transcripts_dir = find_cursor_transcripts_dir(
        workspace_dir, extra_projects_dirs=extra_projects_dirs
    )
    if not transcripts_dir:
        return []

    conversations = []
    try:
        entries = os.listdir(transcripts_dir)
    except OSError:
        return []

    for entry in entries:
        root = os.path.join(transcripts_dir, entry)
        if not os.path.isdir(root):
            continue
        conv_id = entry
        jsonl_path = os.path.join(root, f"{conv_id}.jsonl")
        if not os.path.isfile(jsonl_path):
            continue

        mtime = os.path.getmtime(jsonl_path)
        msg_count = 0
        first_user_text = ""
        try:
            with open(jsonl_path, "r", encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        obj = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    role = obj.get("role")
                    if role in ("user", "assistant"):
                        msg_count += 1
                    if role == "user" and not first_user_text:
                        msg_obj = obj.get("message", {})
                        text = _extract_cursor_user_text(msg_obj.get("content", ""))
                        if text and text.strip():
                            first_user_text = text.strip()
        except OSError:
            continue

        conversations.append({
            "id": conv_id,
            "message_count": msg_count,
            "request_count": 0,
            "last_modified": datetime.fromtimestamp(mtime).isoformat(),
            "preview": first_user_text[:100] if first_user_text else "(empty)",
            "source": "cursor",
        })

    conversations.sort(key=lambda c: c["last_modified"], reverse=True)
    return conversations


def read_cursor_conversation(
    workspace_dir: str,
    conversation_id: str,
    extra_projects_dirs: Optional[list[str]] = None,
) -> list[dict]:
    """读取 Cursor 指定会话的完整消息列表。"""
    transcripts_dir = find_cursor_transcripts_dir(
        workspace_dir, extra_projects_dirs=extra_projects_dirs
    )
    if not transcripts_dir:
        print(f"❌ 未找到工作区 '{workspace_dir}' 的 Cursor 历史记录目录", file=sys.stderr)
        sys.exit(1)

    jsonl_path = os.path.join(transcripts_dir, conversation_id, f"{conversation_id}.jsonl")
    if not os.path.exists(jsonl_path):
        print(f"❌ 未找到 Cursor 会话文件: {jsonl_path}", file=sys.stderr)
        sys.exit(1)

    records = []
    with open(jsonl_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError:
                continue

    conversation: list[dict] = []
    pending_tool_calls: dict[str, dict] = {}

    for record in records:
        rec_type = record.get("role")
        if rec_type not in ("user", "assistant", "tool"):
            continue

        msg_obj = record.get("message", {})
        if not isinstance(msg_obj, dict):
            continue
        content = msg_obj.get("content", "")

        if rec_type == "user":
            text = _extract_cursor_user_text(content)
            if not text or not text.strip():
                continue
            if conversation and conversation[-1]["role"] == "user":
                conversation[-1]["content"] += "\n\n" + text
            else:
                conversation.append({"role": "user", "content": text})

        elif rec_type == "assistant":
            text = _extract_cursor_assistant_text(content)
            thinking = _extract_anthropic_thinking(content)
            tool_calls_raw = _extract_cursor_tool_calls(content)
            model_name = _extract_model_name_from_message_obj(msg_obj)

            enriched_calls = []
            for tc in tool_calls_raw:
                enriched = {
                    "toolCallId": tc["toolCallId"],
                    "toolName": tc["toolName"],
                    "displayName": _humanize_claudecode_tool_name(tc["toolName"]),
                    "argsSummary": _summarize_claudecode_args(tc["toolName"], tc.get("args", {})),
                    "result_preview": "",
                    "isError": False,
                    "position": tc.get("position", 0),
                }
                if tc["toolCallId"]:
                    pending_tool_calls[tc["toolCallId"]] = enriched
                enriched_calls.append(enriched)

            if not text and not enriched_calls and not thinking:
                continue

            if conversation and conversation[-1]["role"] == "assistant":
                prev_segments = _count_text_segments(conversation[-1].get("content", ""))
                if text:
                    prev = conversation[-1].get("content", "")
                    conversation[-1]["content"] = (prev + "\n\n" + text) if prev else text
                if thinking:
                    prev_th = conversation[-1].get("thinking", "")
                    conversation[-1]["thinking"] = (prev_th + "\n\n" + thinking) if prev_th else thinking
                if enriched_calls:
                    _shift_tool_call_positions(enriched_calls, prev_segments)
                    conversation[-1].setdefault("tool_calls", []).extend(enriched_calls)
                if model_name and not conversation[-1].get("model_name"):
                    conversation[-1]["model_name"] = model_name
            else:
                entry: dict = {"role": "assistant", "content": text or ""}
                if thinking:
                    entry["thinking"] = thinking
                if enriched_calls:
                    entry["tool_calls"] = enriched_calls
                if model_name:
                    entry["model_name"] = model_name
                conversation.append(entry)

        elif rec_type == "tool":
            if not isinstance(content, list):
                continue
            for item in content:
                if not isinstance(item, dict):
                    continue
                if item.get("type") != "tool_result":
                    continue
                call_id = item.get("tool_use_id", "")
                if not call_id or call_id not in pending_tool_calls:
                    continue
                raw = item.get("content", "")
                preview = _clean_tool_result_preview(str(raw))
                if len(preview) > 200:
                    preview = preview[:197] + "..."
                pending_tool_calls[call_id]["result_preview"] = preview

    conversation = [
        m for m in conversation
        if m.get("content", "").strip() or m.get("tool_calls") or m.get("thinking", "").strip()
    ]
    return conversation


# ============ Unified Source Detection ============


def detect_source(
    workspace_dir: str,
    extra_data_dirs: Optional[list[str]] = None,
    extra_projects_dirs: Optional[list[str]] = None,
) -> str:
    """自动探测当前工作区使用哪种 AI 工具的历史记录。

    优先级：WorkBuddy > Cursor > Claude Code > Box AI > CodeBuddy
    返回 'workbuddy' / 'cursor' / 'claudecode' / 'boxai' / 'codebuddy'
    """
    if find_workbuddy_session_dir(workspace_dir, extra_projects_dirs=extra_projects_dirs):
        return "workbuddy"
    if find_cursor_transcripts_dir(workspace_dir, extra_projects_dirs=extra_projects_dirs):
        return "cursor"
    if find_claudecode_session_dir(workspace_dir, extra_projects_dirs=extra_projects_dirs):
        return "claudecode"
    if get_boxai_db_path():
        return "boxai"
    ws_hash = get_workspace_hash(workspace_dir)
    ws_hash_candidates = get_workspace_hash_candidates(workspace_dir)
    if find_history_base(
        ws_hash,
        extra_data_dirs=extra_data_dirs,
        workspace_hash_candidates=ws_hash_candidates,
    ):
        return "codebuddy"
    return "codebuddy"  # 默认回退


def list_conversations(
    workspace_dir: str,
    source: str = "auto",
    extra_data_dirs: Optional[list[str]] = None,
    extra_projects_dirs: Optional[list[str]] = None,
) -> list[dict]:
    """List all conversations for a workspace, sorted by most recent first.

    Args:
        workspace_dir:        工作区目录路径
        source:               数据来源 ('auto' | 'codebuddy' | 'claudecode' | 'cursor' | 'workbuddy' | 'boxai')
                              auto 时优先 workbuddy，再 cursor，再 claudecode，再 boxai，最后 codebuddy
        extra_data_dirs:      显式传入的 CodeBuddy 数据根目录列表
        extra_projects_dirs:  显式传入的 Claude Code / WorkBuddy projects 目录列表
    """
    if source == "auto":
        source = detect_source(
            workspace_dir,
            extra_data_dirs=extra_data_dirs,
            extra_projects_dirs=extra_projects_dirs,
        )

    if source == "workbuddy":
        return list_workbuddy_conversations(
            workspace_dir, extra_projects_dirs=extra_projects_dirs
        )

    if source == "cursor":
        return list_cursor_conversations(
            workspace_dir, extra_projects_dirs=extra_projects_dirs
        )

    if source == "claudecode":
        return list_claudecode_conversations(
            workspace_dir, extra_projects_dirs=extra_projects_dirs
        )

    if source == "boxai":
        return list_boxai_conversations(workspace_dir)

    # ---------- CodeBuddy 原有逻辑 ----------
    ws_hash = get_workspace_hash(workspace_dir)
    ws_hash_candidates = get_workspace_hash_candidates(workspace_dir)
    history_base = find_history_base(
        ws_hash,
        extra_data_dirs=extra_data_dirs,
        workspace_hash_candidates=ws_hash_candidates,
    )

    if not history_base:
        return []

    conversations = []
    for conv_id in os.listdir(history_base):
        conv_path = os.path.join(history_base, conv_id)
        if not os.path.isdir(conv_path):
            continue

        idx_path = os.path.join(conv_path, "index.json")
        if not os.path.exists(idx_path):
            continue

        try:
            with open(idx_path, "r", encoding="utf-8") as f:
                idx = json.load(f)

            msg_count = len(idx.get("messages", []))
            req_count = len(idx.get("requests", []))

            # Get modification time as proxy for "last active"
            mtime = os.path.getmtime(idx_path)

            # Try to get the first user message as a preview
            first_user_text = ""
            messages = idx.get("messages", [])
            msg_dir = os.path.join(conv_path, "messages")
            for msg_meta in messages:
                if msg_meta.get("role") == "user":
                    msg_file = os.path.join(msg_dir, f"{msg_meta['id']}.json")
                    if os.path.exists(msg_file):
                        first_user_text = _extract_user_original_text(msg_file)
                        break

            conversations.append(
                {
                    "id": conv_id,
                    "message_count": msg_count,
                    "request_count": req_count,
                    "last_modified": datetime.fromtimestamp(mtime).isoformat(),
                    "preview": first_user_text[:100] if first_user_text else "(empty)",
                    "source": "codebuddy",
                }
            )
        except (json.JSONDecodeError, IOError):
            continue

    # Sort by last modified, most recent first
    conversations.sort(key=lambda c: c["last_modified"], reverse=True)
    return conversations


def _extract_user_original_text(msg_file_path: str) -> str:
    """Extract the user's original text from a message file.

    For user messages, the original text is primarily stored in
    extra.sourceContentBlocks. Besides plain text blocks, command/skill mention
    blocks are usually stored as resource_link and should be reconstructed as
    user-visible text (e.g. /session-share).

    Falls back to extracting from inputPhrase and then <user_query> tags in
    message.content.
    """
    try:
        with open(msg_file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, IOError):
        return ""

    extra_raw = data.get("extra", "{}")
    try:
        extra = json.loads(extra_raw) if isinstance(extra_raw, str) else extra_raw
    except json.JSONDecodeError:
        extra = {}

    def _mention_to_text(block: dict) -> str:
        if not isinstance(block, dict) or block.get("type") != "resource_link":
            return ""
        meta = block.get("_meta", {}) or {}
        mention_type = meta.get("mentionType")
        if mention_type == "image":
            return "[图片]"
        if mention_type not in ("command", "skill"):
            return ""

        name = (
            meta.get("commandName")
            or block.get("name")
            or meta.get("displayText")
            or ""
        )
        name = str(name).strip()
        if not name:
            return ""
        return f"/{name.lstrip('/')}"

    # Priority 1: sourceContentBlocks (clean user input + skill/command mention)
    source_blocks = extra.get("sourceContentBlocks", [])
    if source_blocks:
        parts = []
        for blk in source_blocks:
            if not isinstance(blk, dict):
                continue

            if blk.get("type") == "text":
                text = blk.get("text", "")
                # Filter out internal metadata markers
                if text and not text.startswith("_meta"):
                    parts.append(text)
                continue

            if blk.get("type") in ("image", "image_url"):
                parts.append("[图片]")
                continue

            mention_text = _mention_to_text(blk)
            if mention_text:
                parts.append(mention_text)

        merged = "".join(parts).strip()
        if merged:
            return merged

    # Priority 2: inputPhrase from extra
    input_phrases = extra.get("inputPhrase", [])
    if input_phrases:
        parts = []
        for phrase in input_phrases:
            if not isinstance(phrase, dict):
                continue
            p_type = phrase.get("type", "")
            content = str(phrase.get("content", "")).strip()
            if not content:
                continue

            if p_type in ("command", "skill"):
                parts.append(f"/{content.lstrip('/')}")
            else:
                parts.append(content)

        merged = "".join(parts).strip()
        if merged:
            return merged

    # Priority 3: Extract from <user_query> tags in message content
    msg_raw = data.get("message", "")
    try:
        msg = json.loads(msg_raw) if isinstance(msg_raw, str) else msg_raw
        content = msg.get("content", [])
        if isinstance(content, list):
            for c in content:
                if isinstance(c, dict) and c.get("type") == "text":
                    text = c.get("text", "")
                    # Try to extract from <user_query> tags
                    import re

                    match = re.search(
                        r"<user_query>\s*(.*?)\s*</user_query>", text, re.DOTALL
                    )
                    if match:
                        return match.group(1).strip()
                    # If no user_query tag, return the text but warn it may contain metadata
                    return text
        elif isinstance(content, str):
            return content
    except (json.JSONDecodeError, TypeError):
        pass

    return ""


def _extract_model_name(msg_file_path: str) -> str:
    """Extract the model name from a message file's extra metadata.

    Looks for extra.modelName first, falls back to extra.modelId.
    Returns empty string if not found.
    """
    try:
        with open(msg_file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, IOError):
        return ""

    extra_raw = data.get("extra", "{}")
    try:
        extra = json.loads(extra_raw) if isinstance(extra_raw, str) else extra_raw
    except json.JSONDecodeError:
        extra = {}

    raw = extra.get("modelName", "") or extra.get("modelId", "")
    return _normalize_model_name(raw)


def _extract_assistant_text(msg_file_path: str) -> str:
    """Extract the assistant's text response from a message file.

    Extracts only text content blocks, skipping tool_use blocks.
    """
    try:
        with open(msg_file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, IOError):
        return ""

    msg_raw = data.get("message", "")
    try:
        msg = json.loads(msg_raw) if isinstance(msg_raw, str) else msg_raw
        content = msg.get("content", [])
        texts = []
        if isinstance(content, list):
            for c in content:
                if isinstance(c, dict) and c.get("type") == "text":
                    text = c.get("text", "").strip()
                    if text:
                        texts.append(text)
        elif isinstance(content, str):
            if content.strip():
                texts.append(content.strip())
        return "\n\n".join(texts)
    except (json.JSONDecodeError, TypeError):
        return ""


def _extract_assistant_thinking(msg_file_path: str) -> str:
    """Extract the assistant's thinking / reasoning content from a CodeBuddy message file."""
    try:
        with open(msg_file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, IOError):
        return ""

    msg_raw = data.get("message", "")
    try:
        msg = json.loads(msg_raw) if isinstance(msg_raw, str) else msg_raw
        content = msg.get("content", [])
        return _extract_anthropic_thinking(content)
    except (json.JSONDecodeError, TypeError):
        return ""


# ============ Tool Call Extraction ============


# 工具名 → (图标, 中文标签) 映射
TOOL_NAME_MAP: dict[str, tuple[str, str]] = {
    "read_file":        ("📄", "读取文件"),
    "write_to_file":    ("📝", "写入文件"),
    "replace_in_file":  ("✏️", "编辑文件"),
    "search_content":   ("🔍", "搜索内容"),
    "search_file":      ("📁", "搜索文件"),
    "list_dir":         ("📂", "浏览目录"),
    "execute_command":  ("⚡", "执行命令"),
    "task":             ("🤖", "子任务"),
    "web_search":       ("🌐", "网络搜索"),
    "web_fetch":        ("🌐", "获取网页"),
    "todo_write":       ("📋", "任务管理"),
    "image_gen":        ("🎨", "生成图片"),
    "delete_file":      ("🗑️", "删除文件"),
    "RAG_search":       ("📚", "知识检索"),
    "preview_url":      ("👁️", "预览网页"),
    "use_skill":        ("🧩", "使用技能"),
    "read_lints":       ("⚠️", "检查错误"),
    "open_result_view": ("📊", "展示结果"),
    "ask_followup_question": ("❓", "追问用户"),
}


def _humanize_tool_name(tool_name: str) -> str:
    """将工具内部名称转换为人类可读的 图标+标签 格式。"""
    if tool_name in TOOL_NAME_MAP:
        icon, label = TOOL_NAME_MAP[tool_name]
        return f"{icon} {label}"
    return f"🔧 {tool_name}"


def _summarize_args(tool_name: str, args: dict) -> str:
    """根据不同工具类型，提取最有价值的参数摘要。"""
    if tool_name in ("read_file", "write_to_file", "replace_in_file", "delete_file"):
        path = args.get("filePath", args.get("target_file", ""))
        return os.path.basename(path) if path else ""
    elif tool_name == "search_content":
        return args.get("pattern", "")
    elif tool_name == "search_file":
        return args.get("pattern", "")
    elif tool_name == "execute_command":
        cmd = args.get("command", "")
        return (cmd[:60] + "...") if len(cmd) > 60 else cmd
    elif tool_name == "list_dir":
        d = args.get("target_directory", "")
        return os.path.basename(d.rstrip("/")) if d else ""
    elif tool_name == "web_search":
        return args.get("query", "")
    elif tool_name == "web_fetch":
        return args.get("url", "")[:60]
    elif tool_name == "task":
        return args.get("description", "")
    elif tool_name == "RAG_search":
        return args.get("queryString", "")[:50]
    elif tool_name == "use_skill":
        return args.get("command", "")
    else:
        # 通用：返回第一个有值的字符串参数
        for v in args.values():
            if isinstance(v, str) and v:
                return v[:50]
        return ""


def _extract_tool_calls(msg_file_path: str) -> list[dict]:
    """从 assistant 消息文件中提取所有 tool-call 块。

    Returns:
        [{"toolCallId": "...", "toolName": "...", "args": {...}, "position": N}, ...]
        其中 position 表示该工具调用在原 content 数组中前置非空 text 块的数量，
        用于在渲染时把工具调用穿插回 content 段落之间，保持原始时序。
    """
    try:
        with open(msg_file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, IOError):
        return []

    msg_raw = data.get("message", "")
    try:
        msg = json.loads(msg_raw) if isinstance(msg_raw, str) else msg_raw
    except (json.JSONDecodeError, TypeError):
        return []

    content = msg.get("content", [])
    if not isinstance(content, list):
        return []

    tool_calls = []
    text_count = 0
    for block in content:
        if not isinstance(block, dict):
            continue
        block_type = block.get("type")
        if block_type == "text":
            t = block.get("text", "")
            if isinstance(t, str) and t.strip():
                # 关键：一个 text 块内部可能含多个 \n\n 段落，必须按渲染层口径分段计数，
                # 否则跨 block 后渲染时段落与 tool_call.position 会错位（详见
                # _render_assistant_message_interleaved 的段落切分规则）。
                text_count += _count_text_segments(t)
            continue
        if block_type != "tool-call":
            continue

        tool_call_id = block.get("toolCallId", "")
        tool_name = block.get("toolName", "")
        args_raw = block.get("args", "{}")

        # args 可能是 JSON 字符串或已解析的 dict
        if isinstance(args_raw, str):
            try:
                args = json.loads(args_raw)
            except json.JSONDecodeError:
                args = {}
        else:
            args = args_raw if isinstance(args_raw, dict) else {}

        tool_calls.append({
            "toolCallId": tool_call_id,
            "toolName": tool_name,
            "args": args,
            "position": text_count,
        })

    return tool_calls


def _extract_tool_results(msg_file_path: str) -> dict[str, dict]:
    """从 tool 消息文件中提取所有 tool-result 块，按 toolCallId 索引。

    Returns:
        {"callId": {"toolName": "...", "result": "...", "isError": False}, ...}
    """
    try:
        with open(msg_file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, IOError):
        return {}

    msg_raw = data.get("message", "")
    try:
        msg = json.loads(msg_raw) if isinstance(msg_raw, str) else msg_raw
    except (json.JSONDecodeError, TypeError):
        return {}

    content = msg.get("content", [])
    if not isinstance(content, list):
        return {}

    results = {}
    for block in content:
        if not isinstance(block, dict):
            continue
        if block.get("type") != "tool-result":
            continue

        call_id = block.get("toolCallId", "")
        if not call_id:
            continue

        result_raw = block.get("result", "")
        # result 可能是字符串、列表或 dict
        if isinstance(result_raw, list):
            # 有些 result 是 [{type: "text", text: "..."}] 格式
            parts = []
            for r in result_raw:
                if isinstance(r, dict) and r.get("text"):
                    parts.append(r["text"])
                elif isinstance(r, str):
                    parts.append(r)
            result_str = "\n".join(parts)
        elif isinstance(result_raw, dict):
            result_str = json.dumps(result_raw, ensure_ascii=False)
        else:
            result_str = str(result_raw) if result_raw else ""

        results[call_id] = {
            "toolName": block.get("toolName", ""),
            "result": result_str,
            "isError": block.get("isError", False),
        }

    return results


def read_conversation(
    workspace_dir: str,
    conversation_id: str,
    source: str = "auto",
    extra_data_dirs: Optional[list[str]] = None,
    extra_projects_dirs: Optional[list[str]] = None,
) -> list[dict]:
    """Read all messages from a conversation, in order.

    Args:
        workspace_dir:        工作区目录路径
        conversation_id:      会话 ID
        source:               数据来源 ('auto' | 'codebuddy' | 'claudecode' | 'cursor' | 'workbuddy' | 'boxai')
                              auto 时根据 conversation_id 是否含 '-' 判断：
                              UUID 格式（含多个 '-'）→ workbuddy/cursor/claudecode/boxai，否则自动探测
        extra_data_dirs:      显式传入的 CodeBuddy 数据根目录列表（最高优先级）
        extra_projects_dirs:  显式传入的 Claude Code / WorkBuddy projects 目录列表（最高优先级）

    Returns a list of dicts:
        [
            {"role": "user", "content": "..."},
            {"role": "assistant", "content": "...", "tool_calls": [...]},
            ...
        ]

    Each tool_calls entry:
        {
            "toolCallId": "...",
            "toolName": "read_file",
            "displayName": "📄 读取文件",
            "argsSummary": "app.ts",
            "result_preview": "import React...",
            "isError": false
        }
    """
    # 自动判断来源：UUID 格式（8-4-4-4-12，含 4 个 '-'）视为 claudecode/workbuddy/boxai
    if source == "auto":
        dash_count = conversation_id.count("-")
        if dash_count >= 4:
            # UUID 格式：按优先级依次探测
            if find_workbuddy_session_dir(
                workspace_dir, extra_projects_dirs=extra_projects_dirs
            ):
                source = "workbuddy"
            elif find_cursor_transcripts_dir(
                workspace_dir, extra_projects_dirs=extra_projects_dirs
            ):
                source = "cursor"
            elif get_boxai_db_path():
                source = "boxai"
            else:
                source = "claudecode"
        else:
            source = detect_source(
                workspace_dir,
                extra_data_dirs=extra_data_dirs,
                extra_projects_dirs=extra_projects_dirs,
            )

    if source == "workbuddy":
        return read_workbuddy_conversation(
            workspace_dir,
            conversation_id,
            extra_projects_dirs=extra_projects_dirs,
        )

    if source == "cursor":
        return read_cursor_conversation(
            workspace_dir,
            conversation_id,
            extra_projects_dirs=extra_projects_dirs,
        )

    if source == "claudecode":
        return read_claudecode_conversation(
            workspace_dir,
            conversation_id,
            extra_projects_dirs=extra_projects_dirs,
        )

    if source == "boxai":
        return read_boxai_conversation(
            workspace_dir,
            conversation_id,
        )

    # ---------- CodeBuddy 原有逻辑 ----------
    ws_hash = get_workspace_hash(workspace_dir)
    ws_hash_candidates = get_workspace_hash_candidates(workspace_dir)
    history_base = find_history_base(
        ws_hash,
        extra_data_dirs=extra_data_dirs,
        workspace_hash_candidates=ws_hash_candidates,
    )

    if not history_base:
        print(
            f"❌ 未找到工作区 '{workspace_dir}' 的历史记录目录",
            file=sys.stderr,
        )
        print(
            f"   (workspace hash: {ws_hash})",
            file=sys.stderr,
        )
        sys.exit(1)

    conv_path = os.path.join(history_base, conversation_id)
    if not os.path.isdir(conv_path):
        print(
            f"❌ 未找到对话 '{conversation_id}'",
            file=sys.stderr,
        )
        sys.exit(1)

    idx_path = os.path.join(conv_path, "index.json")
    if not os.path.exists(idx_path):
        print(f"❌ 对话索引文件缺失: {idx_path}", file=sys.stderr)
        sys.exit(1)

    with open(idx_path, "r", encoding="utf-8") as f:
        idx = json.load(f)

    messages_meta = idx.get("messages", [])
    msg_dir = os.path.join(conv_path, "messages")

    conversation: list[dict] = []
    # 用于将 tool-result 回填到对应 tool-call
    pending_tool_calls: dict[str, dict] = {}

    for msg_meta in messages_meta:
        msg_id = msg_meta.get("id")
        role = msg_meta.get("role")
        is_complete = msg_meta.get("isComplete", True)

        if not msg_id or not role:
            continue

        # Skip incomplete messages
        if not is_complete:
            continue

        msg_file = os.path.join(msg_dir, f"{msg_id}.json")
        if not os.path.exists(msg_file):
            continue

        if role == "user":
            text = _extract_user_original_text(msg_file)
            if not text or not text.strip():
                continue
            # Merge consecutive same-role messages
            if conversation and conversation[-1]["role"] == "user":
                conversation[-1]["content"] += "\n\n" + text
            else:
                conversation.append({"role": "user", "content": text})

        elif role == "assistant":
            text = _extract_assistant_text(msg_file)
            thinking = _extract_assistant_thinking(msg_file)
            tool_calls = _extract_tool_calls(msg_file)
            model_name = _extract_model_name(msg_file)

            # 为每个 tool-call 添加人类可读信息
            enriched_calls = []
            for tc in tool_calls:
                enriched = {
                    "toolCallId": tc["toolCallId"],
                    "toolName": tc["toolName"],
                    "displayName": _humanize_tool_name(tc["toolName"]),
                    "argsSummary": _summarize_args(tc["toolName"], tc.get("args", {})),
                    "result_preview": "",
                    "isError": False,
                    "position": tc.get("position", 0),
                }
                pending_tool_calls[tc["toolCallId"]] = enriched
                enriched_calls.append(enriched)

            # 即使没有文本，只要有 tool_calls 或 thinking 也要记录
            if not text and not enriched_calls and not thinking:
                continue

            # 合并连续的 assistant 消息
            if conversation and conversation[-1]["role"] == "assistant":
                # 关键：在合并 text 之前先记下旧 content 的段数，作为 enriched_calls 的 position 偏移
                prev_segments = _count_text_segments(conversation[-1].get("content", ""))
                if text:
                    prev_content = conversation[-1].get("content", "")
                    conversation[-1]["content"] = (
                        (prev_content + "\n\n" + text) if prev_content else text
                    )
                if thinking:
                    prev_th = conversation[-1].get("thinking", "")
                    conversation[-1]["thinking"] = (
                        (prev_th + "\n\n" + thinking) if prev_th else thinking
                    )
                if enriched_calls:
                    _shift_tool_call_positions(enriched_calls, prev_segments)
                    prev_calls = conversation[-1].get("tool_calls", [])
                    conversation[-1]["tool_calls"] = prev_calls + enriched_calls
                # 如果之前没有 modelName，用当前的补上
                if model_name and not conversation[-1].get("model_name"):
                    conversation[-1]["model_name"] = model_name
            else:
                entry: dict = {"role": "assistant", "content": text or ""}
                if thinking:
                    entry["thinking"] = thinking
                if enriched_calls:
                    entry["tool_calls"] = enriched_calls
                if model_name:
                    entry["model_name"] = model_name
                conversation.append(entry)

        elif role == "tool":
            # 提取 tool-result，回填到对应的 pending_tool_calls
            results = _extract_tool_results(msg_file)
            for call_id, result_info in results.items():
                if call_id in pending_tool_calls:
                    preview = result_info.get("result", "")
                    preview = _clean_tool_result_preview(preview)
                    # 截断预览文本
                    if len(preview) > 200:
                        preview = preview[:197] + "..."
                    pending_tool_calls[call_id]["result_preview"] = preview
                    pending_tool_calls[call_id]["isError"] = result_info.get(
                        "isError", False
                    )

    # 清理：移除没有内容也没有 tool_calls 也没有 thinking 的 assistant 消息
    conversation = [
        m
        for m in conversation
        if m.get("content", "").strip() or m.get("tool_calls") or m.get("thinking", "").strip()
    ]

    return conversation


def _escape_tool_field(text: str) -> str:
    """转义工具调用字段中的特殊字符，防止破坏 TOOL 标记结构。

    - | → 全角 ｜（防止 pipe 分隔符歧义）
    - --> → -‐> （防止 HTML 注释提前闭合；HTML 不支持嵌套注释，-- > 会提前结束注释）
    - 行首 # → \\# （防止 Markdown 标题语法，避免 ## User / ## Assistant 等内容
      从 TOOL 预览中"泄漏"成真正的文档章节标题，破坏导出文档的结构）
    - \\r 移除
    - \\n 保留（HTML 注释允许跨行；多行预览逐行展示，保持可读性）

    注意：不转义 <!--。HTML 注释不可嵌套，内部的 <!-- 只是普通文本，不会开始新注释。
    只有 --> 能真正闭合注释，所以只转义 --> 就足够安全。
    """
    result = (
        text.replace("|", "｜")
            .replace("-->", "-‐>")     # 零宽连字符替换，视觉无感知但不破坏注释
            .replace("\r", "")
    )
    # 转义行首 Markdown 标题语法：# / ## / ### 等
    # 当 TOOL preview 包含 Markdown 文档（如 AI 读取了旧导出文件），其中的
    # ## User / ## Assistant 等行若原样保留，渲染器会将其识别为 h2 章节标题，
    # 导致文档结构错乱（假章节混入真实对话结构）。
    lines = result.split("\n")
    escaped = []
    for line in lines:
        stripped = line.lstrip()
        if stripped.startswith("#"):
            indent = line[: len(line) - len(stripped)]
            line = indent + "\\" + stripped
        escaped.append(line)
    return "\n".join(escaped)


def _extract_invoked_skills(messages: list[dict]) -> set[str]:
    """从 assistant 的 use_skill 工具调用中提取技能名集合。"""
    skills: set[str] = set()
    for msg in messages:
        if msg.get("role") != "assistant":
            continue
        for tc in msg.get("tool_calls", []):
            if not isinstance(tc, dict):
                continue
            if tc.get("toolName") != "use_skill":
                continue
            raw = str(tc.get("argsSummary", "")).strip()
            if not raw:
                continue
            skill = raw.lstrip("/").strip()
            if skill:
                skills.add(skill)
    return skills


def _normalize_user_skill_invocation(text: str, invoked_skills: set[str]) -> str:
    """将裸技能名用户输入归一化为 `/skill 使用这个技能`。"""
    normalized = (text or "").strip()
    if not normalized or "\n" in normalized:
        return text

    matched = re.fullmatch(r"/?([a-z0-9][a-z0-9-]{1,80})", normalized, flags=re.IGNORECASE)
    if not matched:
        return text

    skill = matched.group(1)
    if skill not in invoked_skills:
        return text

    return f"/{skill} 使用这个技能"


def _apply_assistant_model_fallback(messages: list[dict], fallback: str) -> None:
    """对未从存档解析出 model_name 的 assistant 消息填入 fallback（不覆盖已有）。"""
    fb = (fallback or "").strip()
    if not fb:
        return
    for m in messages:
        if m.get("role") != "assistant":
            continue
        existing = str(m.get("model_name", "")).strip()
        if existing:
            continue
        m["model_name"] = fb


# 当前 skill 名（与 SKILL.md frontmatter 一致）
_SHARE_SKILL_NAME = "session-share"
# 兼容旧名（历史会话存档里可能仍写着旧 skill 名）
_LEGACY_SHARE_SKILL_NAMES = ("share-to-community",)
_ALL_SHARE_SKILL_NAMES = (_SHARE_SKILL_NAME, *_LEGACY_SHARE_SKILL_NAMES)

# 触发 session-share 技能的用户消息特征（兼容历史触发词）
_SHARE_TRIGGER_PATTERNS = [
    re.compile(r"@skill://session-share", re.IGNORECASE),
    re.compile(r"@skill://share-to-community", re.IGNORECASE),
    re.compile(r"share\s+session", re.IGNORECASE),
    re.compile(r"session\s+share", re.IGNORECASE),
    re.compile(r"share\s+to\s+community", re.IGNORECASE),
    re.compile(r"分享会话", re.IGNORECASE),
    re.compile(r"分享对话", re.IGNORECASE),
    re.compile(r"分享到社区", re.IGNORECASE),
    re.compile(r"上传对话", re.IGNORECASE),
    re.compile(r"导出并上传", re.IGNORECASE),
]


def _is_share_trigger_message(text: str) -> bool:
    """判断用户消息是否是触发 session-share 技能的意图。

    支持以下格式：
    - @skill://session-share / @command://session-share / /session-share
    - @skill://share-to-community（历史触发词，仅做兼容）
    - share session / session share / 分享会话 / 分享对话 / 分享到社区 / 上传对话 / 导出并上传
    """
    if not text:
        return False
    for pattern in _SHARE_TRIGGER_PATTERNS:
        if pattern.search(text):
            return True
    # 匹配命令格式：/<skill_name> [args]（含历史名）
    for name in _ALL_SHARE_SKILL_NAMES:
        if re.search(r"(?:^|\s)/" + re.escape(name) + r"(?:\s|$)", text, re.IGNORECASE):
            return True
        # 匹配 @skill://<name> 和 @command://<name>
        if re.search(r"@(?:skill|command)://" + re.escape(name), text, re.IGNORECASE):
            return True
    return False


def _filter_last_share_trigger(messages: list[dict]) -> list[dict]:
    """过滤掉最后一次触发 session-share 的对话轮次及其后所有内容。

    逻辑：
    1. 从后往前扫描，找到最后一条触发 session-share 的 user 消息索引
    2. 如果通过 user content 匹配失败（IDE skill 按钮触发时 sourceContentBlocks 为空），
       则从后往前检查 assistant 消息中是否包含 use_skill("session-share") 工具调用
       （也兼容历史名 share-to-community），找到后截断到该 assistant 之前最近的 user 消息
    3. 截断该消息及其后的所有内容，仅保留之前的消息
    4. 若会话从未触发 session-share，原样返回
    """
    if not messages:
        return messages

    last_trigger_idx = -1

    # 策略1：直接匹配 user 消息的 content
    for i in range(len(messages) - 1, -1, -1):
        msg = messages[i]
        if msg.get("role") == "user" and _is_share_trigger_message(msg.get("content", "")):
            last_trigger_idx = i
            break

    # 策略2：通过 assistant 的 use_skill 工具调用反推
    # （IDE skill 按钮触发时，user 的 sourceContentBlocks 为空，content 也被清洗为空）
    if last_trigger_idx < 0:
        for i in range(len(messages) - 1, -1, -1):
            msg = messages[i]
            if msg.get("role") != "assistant":
                continue
            for tc in msg.get("tool_calls", []):
                if not isinstance(tc, dict):
                    continue
                if tc.get("toolName") == "use_skill":
                    skill_arg = str(tc.get("argsSummary", "")).strip().lstrip("/")
                    if skill_arg in _ALL_SHARE_SKILL_NAMES:
                        # 找到了 assistant 使用 session-share 技能的调用
                        # 截断点是该 assistant 之前最近的 user 消息
                        for j in range(i - 1, -1, -1):
                            if messages[j].get("role") == "user":
                                last_trigger_idx = j
                                break
                        # 如果该 assistant 就是第一条消息（没有前置 user），则截断到 0
                        if last_trigger_idx < 0:
                            last_trigger_idx = i
                        break
            if last_trigger_idx >= 0:
                break

    if last_trigger_idx < 0:
        return messages

    return messages[:last_trigger_idx]


def _compute_last_tool_position(tool_calls: list[dict]) -> int:
    """返回 tool_calls 中最大的 position；无 tool_call 或全部缺失 position 时返回 -1。

    用于把一次 assistant 回答的 text 段切成两部分：
      - text 段索引 <= 该值 → narration（进 timeline 折叠）
      - text 段索引 >  该值 → 正文（最终回答，原样输出）

    "工具调用前的所有文字都是 narration（含第一个工具调用之前），工具调用完成后才是正文"
    —— 这正是用户的硬约束。
    """
    if not tool_calls:
        return -1
    positions = [
        int(tc.get("position", -1))
        for tc in tool_calls
        if isinstance(tc, dict) and "position" in tc
    ]
    positions = [p for p in positions if p >= 0]
    if not positions:
        return -1
    return max(positions)


def _render_thinking_block(thinking: str) -> list[str]:
    """[已废弃] 旧版本把 thinking 包装成 <details> 折叠块。

    保留函数仅为兼容外部调用方；新逻辑下 thinking 已并入 timeline 容器，
    作为第一个 NARRATION/THINKING marker 输出（见 _format_thinking_markers
    与 _render_assistant_message_interleaved）。设计理由：
    用户的设计原则是「最后一次工具调用前的所有内容（含 thinking）都属于 timeline，
    工具调用之后的内容才是正文」。把 thinking 单独挂在 timeline 之外的 <details>
    会破坏这个原则，且 codebuddy/workbuddy/box 数据源不存 thinking、claudecode/cursor
    存 thinking 时各家显示风格不一致。统一进 timeline 后所有平台风格对齐。

    本函数现在直接返回空列表，避免重复输出。
    """
    return []


def _format_thinking_markers(text: str) -> list[str]:
    """把模型思考过程按行拆分为多条 <!-- THINKING:|...| --> 注释。

    原实现将多行文本中的 \\n 转义为 \\\\n（字面量）塞进单行注释，依赖前端
    解码回换行。但前端未可靠解码，导致思考过程全部挤成一行，可读性极差。

    现改为按 \\n 拆分，每一非空行独立成一条 THINKING marker：
      - 原文本中的单 \\n 保留为行间分隔（多条独立 marker）
      - 连续空行（\\n\\n）跳过，不产生空白 marker

    格式与 NARRATION/TOOL 注释保持同构（`|` 包围内容、单行注释），
    前端可用同一套解析框架处理；不识别该类型的渲染器也会把它当
    HTML 注释忽略，不会暴露明文。空内容返回空列表。
    """
    lines = text.strip().split("\n")
    markers: list[str] = []
    for line in lines:
        stripped = line.strip()
        if not stripped:
            continue
        encoded = _escape_narration_field(stripped)
        if encoded:
            markers.append(f"<!-- THINKING:|{encoded}| -->")
    return markers


# ============ Timeline 标记常量 ============
# Timeline 是 assistant 消息中按时序穿插展示工具调用与过程叙述的容器。
# 前端契约：识别 START/END 之间的所有行，按出现顺序渲染成可折叠时间线组件。
_TIMELINE_START = "<!-- TIMELINE_START -->"
_TIMELINE_END = "<!-- TIMELINE_END -->"


def _escape_narration_field(text: str) -> str:
    """转义 NARRATION/THINKING 注释行中的叙述文本，保证单行安全。

    需要单行存放并防止破坏 HTML 注释，做以下转换：
    - `--` (含 `-->`) → `-‐` (零宽连字符替换，肉眼无感知但避免注释提前闭合)
    - `\\` → `\\\\` (转义反斜杠本身)
    - `\\r` 移除
    - 首尾空白 strip

    注意：调用方应在传入前先对原始多行文本按 \\n 拆分，每行独立调用本函数
    生成一条 marker；不再使用 \\\\n 字面量编码——前端未可靠解码该编码，
    多行内容挤成一行严重影响可读性。
    """
    s = text.strip()
    # 顺序很重要：先处理 \\，再处理 \r，最后处理 -->
    s = s.replace("\\", "\\\\")
    s = s.replace("\r", "")
    s = s.replace("-->", "-‐>")  # 防止注释提前闭合
    return s


def _format_narration_markers(text: str) -> list[str]:
    """把叙述段落按行拆分为多条 <!-- NARRATION:|...| --> 注释。

    原实现将多行文本中的 \\n 转义为 \\\\n（字面量）塞进单行注释，依赖前端
    解码回换行。但前端未可靠解码，导致过程叙述全部挤成一行，可读性极差。

    现改为按 \\n 拆分，每一非空行独立成一条 NARRATION marker：
      - 原文本中的单 \\n 保留为行间分隔（多条独立 marker）
      - 连续空行（\\n\\n）跳过，不产生空白 marker

    使用 `|` 包围内容是为了与 TOOL 注释行风格保持一致，前端可用同一套
    解析框架。空叙述返回空列表（调用方应自行过滤）。
    """
    lines = text.strip().split("\n")
    markers: list[str] = []
    for line in lines:
        stripped = line.strip()
        if not stripped:
            continue
        encoded = _escape_narration_field(stripped)
        if encoded:
            markers.append(f"<!-- NARRATION:|{encoded}| -->")
    return markers


def _infer_code_language(code: str) -> str:
    """根据代码内容启发式推断编程语言，用于给无语言标注的代码围栏补全标注。

    Returns:
        语言标识符（如 'python', 'javascript', 'bash' 等），推断不出则返回 'text'。
    """
    code_stripped = code.strip()
    lines = code_stripped.split("\n")
    first_line = lines[0].strip() if lines else ""

    # ---- 路径/单行输出 → text ----
    if len(lines) <= 2 and (
        code_stripped.startswith("/")
        or code_stripped.startswith("~")
        or code_stripped.startswith("C:\\")
    ):
        return "text"

    # ---- Shell / Bash ----
    if first_line.startswith("#!") and ("bash" in first_line or "sh" in first_line or "zsh" in first_line):
        return "bash"
    shell_patterns = [
        r"^\s*(export|source|alias|echo|cd|ls|mkdir|rm|cp|mv|cat|grep|awk|sed|curl|wget|chmod|sudo|apt|brew|npm|yarn|pip|docker|git)\s",
        r"^\s*\$\s",
        r"^\s*#[^!]",
    ]
    shell_score = sum(1 for line in lines[:10] for pat in shell_patterns if re.match(pat, line))
    if shell_score >= 2 or (len(lines) <= 3 and shell_score >= 1):
        return "bash"

    # ---- Python ----
    py_patterns = [
        r"^\s*(import|from)\s+\w+",
        r"^\s*def\s+\w+\s*\(",
        r"^\s*class\s+\w+",
        r"^\s*(if|elif|else|for|while|with|try|except|finally|return|yield|raise|assert)\s*[^{]*:",
        r"^\s*print\s*\(",
        r"^\s*self\.",
        r'^\s*"""',
        r"^\s*f['\"]",
    ]
    py_score = sum(1 for line in lines[:15] for pat in py_patterns if re.match(pat, line))
    if py_score >= 2:
        return "python"

    # ---- TypeScript / JavaScript ----
    ts_patterns = [
        r"^\s*(import|export)\s+",
        r"^\s*(const|let|var)\s+\w+",
        r"^\s*(interface|type|enum)\s+\w+",
        r"^\s*(async\s+)?function\s+\w+",
        r"^\s*(public|private|protected|readonly)\s+",
        r"=>\s*[{(]",
        r"^\s*console\.(log|error|warn)\(",
        r"^\s*return\s+[<(]",
        r"^\s*\w+\(\{",                          # 函数调用带对象参数: funcName({
        r"^\s*\w+\(\[",                          # 函数调用带数组参数: funcName([
        r"^\s*(true|false|null|undefined)\s*[,})\]]",  # JS 字面量
        r'^\s*\w+:\s*["\[\{]',                   # 对象属性: key: "val" / key: [...] / key: {...}
        r'^\s*\w+:\s*(true|false|null|\d+)\s*[,}]',   # 对象属性: key: true/false/null/数字
    ]
    ts_score = sum(1 for line in lines[:15] for pat in ts_patterns if re.search(pat, line))
    # 区分 TS vs JS
    ts_specific = [r":\s*(string|number|boolean|void|any|unknown|never)\b", r"<\w+>", r"^\s*(interface|type|enum)\s+"]
    is_ts = any(re.search(pat, line) for line in lines[:15] for pat in ts_specific)
    if ts_score >= 2:
        return "typescript" if is_ts else "javascript"

    # ---- JSON ----
    if (first_line.startswith("{") or first_line.startswith("[")) and (
        code_stripped.endswith("}") or code_stripped.endswith("]")
    ):
        try:
            import json as _json
            _json.loads(code_stripped)
            return "json"
        except (ValueError, TypeError):
            pass

    # ---- HTML / XML ----
    if re.match(r"^\s*<(!DOCTYPE|html|div|span|head|body|script|style|template|svg)", first_line, re.IGNORECASE):
        return "html"
    if re.match(r"^\s*<\?xml", first_line):
        return "xml"

    # ---- CSS ----
    css_patterns = [
        r"^\s*[\.\#][a-zA-Z][\w\-]*\s*\{",              # .class { 或 #id {
        r"^\s*[a-z][\w\-]*\s*\{",                        # element { (仅小写开头，避免匹配函数调用)
        r"^\s*(color|font|margin|padding|display|flex|grid|background|border|width|height|position|top|left|right|bottom)\s*:",
    ]
    css_score = sum(1 for line in lines[:10] for pat in css_patterns if re.match(pat, line))
    if css_score >= 2:
        return "css"

    # ---- SQL ----
    sql_keywords = r"^\s*(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP|FROM|WHERE|JOIN|GROUP|ORDER|HAVING)\s"
    sql_score = sum(1 for line in lines[:10] if re.match(sql_keywords, line, re.IGNORECASE))
    if sql_score >= 1:
        return "sql"

    # ---- YAML ----
    yaml_patterns = [r"^\w[\w\-]*:\s", r"^\s*-\s+\w+:\s"]
    yaml_score = sum(1 for line in lines[:8] for pat in yaml_patterns if re.match(pat, line))
    if yaml_score >= 2:
        return "yaml"

    # ---- Go ----
    go_patterns = [r"^\s*package\s+\w+", r"^\s*func\s+", r"^\s*(import|var|type|struct)\s"]
    go_score = sum(1 for line in lines[:10] for pat in go_patterns if re.match(pat, line))
    if go_score >= 2:
        return "go"

    # ---- Rust ----
    rust_patterns = [r"^\s*(fn|let|mut|impl|struct|enum|pub|use|mod|crate)\s", r"^\s*#\[(derive|cfg|test)"]
    rust_score = sum(1 for line in lines[:10] for pat in rust_patterns if re.match(pat, line))
    if rust_score >= 2:
        return "rust"

    # ---- Java / Kotlin / C# ----
    java_patterns = [r"^\s*(public|private|protected)\s+(static\s+)?(void|class|int|String)", r"^\s*System\.out\.print"]
    java_score = sum(1 for line in lines[:10] for pat in java_patterns if re.match(pat, line))
    if java_score >= 1:
        return "java"

    # ---- Markdown ----
    md_patterns = [r"^#{1,6}\s", r"^\s*[-*+]\s", r"^\s*\d+\.\s"]
    md_score = sum(1 for line in lines[:10] for pat in md_patterns if re.match(pat, line))
    if md_score >= 3:
        return "markdown"

    # 兜底
    return "text"


def _ensure_code_fence_language(content: str) -> str:
    """为 markdown 内容中缺少语言标注的代码围栏自动补全语言标识。

    已有语言标注的代码块保持不变；仅对 ``` 后无语言名的代码块进行推断补全。
    """
    # 匹配代码围栏：开头 ```（可能有语言），到闭合 ```
    pattern = re.compile(r"^(`{3,})(\w*)\s*\n(.*?)\n\1\s*$", re.MULTILINE | re.DOTALL)

    def _replacer(m: re.Match) -> str:
        fence = m.group(1)       # ``` or more
        lang = m.group(2)        # existing language or empty
        code = m.group(3)        # code body

        if lang:
            # 已有语言标注，保持不变
            return m.group(0)

        # 推断语言
        inferred = _infer_code_language(code)
        return f"{fence}{inferred}\n{code}\n{fence}"

    return pattern.sub(_replacer, content)


def _ensure_all_fences_annotated(md: str) -> str:
    """最终兜底：逐行扫描完整 markdown，为所有裸开启围栏补全语言标注。

    处理 _ensure_code_fence_language 无法覆盖的边缘情况（如跨消息的未闭合代码块）。
    规则：
      - 带语言标注的围栏（```lang）始终视为开启围栏
      - 裸围栏（```）在代码块内部时视为闭合围栏，否则视为需要补全标注的开启围栏
    """
    lines = md.split("\n")
    result: list[str] = []
    in_code = False

    i = 0
    while i < len(lines):
        line = lines[i]
        fence_match = re.match(r"^(`{3,})(\w*)\s*$", line)

        if fence_match:
            fence = fence_match.group(1)
            lang = fence_match.group(2)

            if lang:
                # 带语言标注 → 始终视为开启围栏
                if in_code:
                    # 先隐式关闭前一个未闭合的代码块（不插入额外行）
                    pass
                in_code = True
                result.append(line)
            else:
                # 裸围栏 ```
                if in_code:
                    # 闭合围栏，保持不变
                    in_code = False
                    result.append(line)
                else:
                    # 开启围栏但缺少语言标注 → 推断并补全
                    in_code = True
                    lookahead: list[str] = []
                    for j in range(i + 1, min(i + 16, len(lines))):
                        if re.match(r"^`{3,}\s*$", lines[j]):
                            break
                        lookahead.append(lines[j])
                    code_sample = "\n".join(lookahead)
                    inferred = _infer_code_language(code_sample)
                    result.append(f"{fence}{inferred}")
        else:
            result.append(line)
        i += 1

    return "\n".join(result)


def _format_tool_call_marker(tc: dict) -> str:
    """把单个 tool_call 渲染成一行 <!-- TOOL:... --> 注释。"""
    t_name = _escape_tool_field(tc.get("toolName", ""))
    t_display = _escape_tool_field(tc.get("displayName", ""))
    t_args = _escape_tool_field(tc.get("argsSummary", ""))
    t_status = "error" if tc.get("isError") else "ok"
    t_preview = _escape_tool_field(
        _clean_tool_result_preview(tc.get("result_preview", ""))
    )
    return f"<!-- TOOL:{t_name}|{t_display}|{t_args}|{t_status}|{t_preview} -->"


def _wrap_timeline(items: list[str]) -> list[str]:
    """把一组 timeline 行（TOOL/NARRATION 注释）包进 START/END 容器。

    items 应为已格式化好的注释行列表（每项一行字符串）。空 items 返回 []。
    """
    if not items:
        return []
    return [_TIMELINE_START, *items, _TIMELINE_END]


def _render_assistant_message_interleaved(
    content: str, tool_calls: list[dict], thinking: str = ""
) -> str:
    """按 position 把 tool_calls 与过程叙述穿插到 timeline 容器，最终结论留在外面。

    输出契约（与前端 timeline 组件对接）：
      <!-- TIMELINE_START -->
      <!-- THINKING:|...| -->     ← 模型思考过程（若有，固定第一项）
      <!-- TOOL:... -->            ← 工具调用
      <!-- NARRATION:|...| -->    ← 过程叙述
      <!-- TOOL:... -->
      ...
      <!-- TIMELINE_END -->

      最终结论文字（普通 markdown，留在 timeline 外）

    设计原则（来自用户）：
      最后一次工具调用之前的所有内容（含 thinking、过程叙述、所有工具调用）
      都属于 timeline；最后一次工具调用之后的内容才是「正文」。
      因此 thinking 也必须并入 timeline，不再独立成 <details> 折叠块——
      这样所有平台（codebuddy/claudecode/cursor/workbuddy/boxai）输出风格一致。

    切分规则（确定性，零启发式）：
    - 设 `last_tool_pos = max(tc.position)`；无 tool_call 时 = -1。
      position 语义：tool_call 在原 content 数组里"前面有几个非空 text 块"，
      也即"在 idx == position-1 这段之后输出"。
    - 第 i 段 text（按 content 数组顺序，已剔除空段与裸 "-" 占位）：
        - i <  last_tool_pos → narration，缓冲到 timeline（位于最后一个 tool 之前）
        - i >= last_tool_pos → 正文，原样输出（位于最后一个 tool 之后）
      注意取严格小于，因为 last_tool_pos==N 表示最后一个 tool 出现在 idx=N-1 之后、
      idx=N 之前；idx=N 这段已经是「最后一个 tool 调用之后产生的文字」，必须当正文。
    - tool_call 按其 position 插入 timeline；同一 position 多个 tool_call 按出现顺序排列。
    - 无 tool_call → 整段都是正文，不生成 timeline 容器。
    - 最后一个 tool_call 之后无 text → 整段都是 narration，无正文段（严格遵循
      "工具调用完成后才是正文"）。

    thinking 现已并入 timeline（作为第一项 THINKING marker），不再单独输出
    <details> 折叠块——见上方设计原则与 _format_thinking_markers。

    THINKING/NARRATION marker 均为单行 HTML 注释。若原始文本含 \\n，会在调用
    _format_*_markers 时拆分为多条独立 marker，从而在渲染时自然保留行间分隔，
    无需依赖前端解码 \\\\n 字面量。

    position 含义：tool_call 在原 content 数组里"前面有几个非空 text 块"。
    退化路径：当所有 tool_calls 都缺失 position（如 Box AI 等无时序来源）时，
    所有 tool_calls 排在 timeline 最前；正文按 last_tool_pos=-1 视为全部正文。
    """
    paragraphs = re.split(r"\n\s*\n", content) if content else []
    # 过滤空段与裸 "-" 占位段（部分 IDE 的空 text 块会被合并器留下 "-" 残影）
    paragraphs = [p for p in (p.strip() for p in paragraphs) if p and p != "-"]
    n_paras = len(paragraphs)

    has_position = any(
        isinstance(tc, dict) and "position" in tc for tc in (tool_calls or [])
    )

    # 按 position 分桶；无 position 时全部归到 0 号桶（即 timeline 最前）
    buckets: dict[int, list[dict]] = {}
    if tool_calls:
        for tc in tool_calls:
            if not isinstance(tc, dict):
                continue
            if has_position:
                pos = max(int(tc.get("position", 0)), 0)
            else:
                pos = 0
            buckets.setdefault(pos, []).append(tc)

    # 计算 narration / 正文的切分点
    # - 有 position：取最大 position，text 段索引 <= 它即 narration
    # - 无 position（退化路径）：last_tool_pos = -1，所有 text 段都是正文，
    #   tool_calls 全部排在 timeline 最前
    last_tool_pos = _compute_last_tool_position(tool_calls or []) if has_position else -1

    timeline_items: list[str] = []
    body_segments: list[str] = []

    # thinking 永远是 timeline 的第一项（在所有 tool_call / narration 之前）。
    # 即使本条 assistant 没有 tool_calls 也保留 timeline 容器，确保 thinking 不污染正文。
    if isinstance(thinking, str) and thinking.strip():
        thinking_markers = _format_thinking_markers(thinking.strip())
        timeline_items.extend(thinking_markers)

    def _emit_bucket(pos: int) -> None:
        for tc in buckets.get(pos, []):
            timeline_items.append(_format_tool_call_marker(tc))

    # position=0 桶：所有 text 段之前的工具调用（含无 position 退化场景的全部 tool_calls）
    _emit_bucket(0)

    for idx, p in enumerate(paragraphs):
        if idx < last_tool_pos:
            # 最后一个工具调用之前 → narration，进 timeline
            narration_markers = _format_narration_markers(p)
            timeline_items.extend(narration_markers)
        else:
            # idx >= last_tool_pos → 在最后一个工具调用之后产生 → 正文
            body_segments.append(p)
        # 该段位置之后的工具调用（position == idx + 1）
        _emit_bucket(idx + 1)

    # 兜底：把 position > n_paras 的工具调用（异常数据）追加到 timeline 末尾，避免丢失
    leftover_keys = sorted(k for k in buckets.keys() if k > n_paras)
    for k in leftover_keys:
        for tc in buckets[k]:
            timeline_items.append(_format_tool_call_marker(tc))

    # 组装最终输出：timeline 容器（仅在有内容时） + 空行 + 正文段
    out: list[str] = []
    if timeline_items:
        out.extend(_wrap_timeline(timeline_items))
        out.append("")
    for seg in body_segments:
        out.append(seg)
        out.append("")

    # 移除末尾多余空行
    while out and not out[-1].strip():
        out.pop()
    return "\n".join(out)


_SOURCE_DISPLAY_NAMES: dict[str, str] = {
    "codebuddy": "CodeBuddy",
    "workbuddy": "WorkBuddy",
    "cursor": "Cursor",
    "claudecode": "Claude Code",
    "boxai": "Box",
}


def conversation_to_markdown(
    messages: list[dict], title: Optional[str] = None, source: str = "codebuddy"
) -> str:
    """Convert a conversation message list to formatted Markdown.

    Args:
        messages: List of dicts with "role", "content", and optional "tool_calls"
        title: Optional title; auto-generated from first user message if None

    Returns:
        Formatted Markdown string with embedded tool call markers
    """
    # 过滤掉最后一次触发 session-share 的对话轮次
    messages = _filter_last_share_trigger(messages)

    if not messages:
        return "# (空对话)\n"

    # Auto-generate title from first user message
    if not title:
        first_user = next(
            (m["content"] for m in messages if m["role"] == "user"), ""
        )
        # Truncate to ~20 chars for title
        title = first_user[:50].split("\n")[0].strip()
        if len(title) > 20:
            title = title[:18] + "…"
        if not title:
            title = "AI 对话记录"

    lines = [
        f"# {title}",
        "",
        f"> 本文由 {_SOURCE_DISPLAY_NAMES.get(source, 'CodeBuddy')} AI 对话导出，分享至 ChatSpark",
        "",
    ]

    invoked_skills = _extract_invoked_skills(messages)

    for msg in messages:
        if msg["role"] == "user":
            lines.append("## User")
        else:
            model_name = msg.get("model_name", "")
            if model_name:
                lines.append(f"## Assistant <!-- MODEL:{model_name} -->")
            else:
                lines.append("## Assistant")
        lines.append("")

        # 对 assistant 消息：tool_calls / thinking / content 段落统一进 timeline。
        # 设计原则：最后一次工具调用之前的所有内容（含 thinking 与过程叙述）都属于
        # timeline；工具调用之后的内容才是「正文」。thinking 因此并入 timeline 第一项，
        # 不再单独输出 <details> 折叠块——确保 5 个平台风格一致。
        if msg["role"] == "assistant":
            tool_calls = msg.get("tool_calls", [])
            content = msg.get("content", "") or ""
            thinking = msg.get("thinking", "") or ""

            if tool_calls or content.strip() or (isinstance(thinking, str) and thinking.strip()):
                rendered = _render_assistant_message_interleaved(content, tool_calls, thinking)
                if rendered:
                    # 为缺少语言标注的代码围栏自动补全语言标识
                    rendered = _ensure_code_fence_language(rendered)
                    lines.append(rendered)
                    lines.append("")
        else:
            # user 消息：原样输出（带 skill 调用归一化和代码围栏补全）
            if msg.get("content", "").strip():
                content = msg["content"]
                content = _normalize_user_skill_invocation(content, invoked_skills)
                content = _ensure_code_fence_language(content)
                lines.append(content)
                lines.append("")

    # 最终兜底：对完整 markdown 逐行扫描，确保不存在任何裸开启围栏（处理跨消息未闭合等边缘情况）
    result = "\n".join(lines)
    result = _ensure_all_fences_annotated(result)
    return result


def generate_summary(messages: list[dict], max_length: int = 200) -> str:
    """Generate a simple summary from conversation messages.

    Creates a summary from the first user message and a brief description
    of the conversation scope.
    """
    # 过滤掉最后一次触发 session-share 的对话轮次
    messages = _filter_last_share_trigger(messages)

    if not messages:
        return "空对话"

    user_messages = [m["content"] for m in messages if m["role"] == "user"]
    assistant_messages = [m["content"] for m in messages if m["role"] == "assistant"]

    first_question = user_messages[0] if user_messages else ""
    # Clean up the first question for summary use
    first_q_clean = first_question.split("\n")[0].strip()[:100]

    parts = [f"用户提问了「{first_q_clean}」"]
    if len(user_messages) > 1:
        parts.append(f"，经过 {len(user_messages)} 轮对话")
    if assistant_messages:
        # Get a snippet of the last assistant message as conclusion hint
        # 取最后一段非空段落的首行作为结论提示。
        last_answer = assistant_messages[-1]
        paragraphs = [
            p.strip() for p in re.split(r"\n\s*\n", last_answer) if p.strip()
        ]
        last_snippet = ""
        if paragraphs:
            last_snippet = paragraphs[-1].split("\n")[0].strip()[:60]
        if not last_snippet:
            last_snippet = last_answer.split("\n")[0].strip()[:60]
        if last_snippet:
            parts.append(f"，最终讨论了「{last_snippet}」")

    summary = "".join(parts)
    if len(summary) > max_length:
        summary = summary[: max_length - 1] + "…"
    return summary


# ============ CLI ============


def main():
    parser = argparse.ArgumentParser(
        description="Read CodeBuddy / Claude Code chat history from local archive"
    )
    parser.add_argument(
        "--workspace-dir",
        required=True,
        help="Workspace directory path (used to locate history files)",
    )
    parser.add_argument(
        "--conversation-id",
        help="Conversation ID to read",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="List all conversations in the workspace",
    )
    parser.add_argument(
        "--auto",
        action="store_true",
        help="Auto-select the most recent conversation",
    )
    parser.add_argument(
        "--source",
        choices=["auto", "codebuddy", "claudecode", "cursor", "workbuddy", "boxai"],
        default="auto",
        help="Force data source: 'codebuddy' / 'claudecode' / 'cursor' / 'workbuddy' / 'boxai' (default: auto-detect)",
    )
    parser.add_argument(
        "--output",
        "-o",
        help="Output file path (default: stdout)",
    )
    parser.add_argument(
        "--format",
        choices=["markdown", "json"],
        default="markdown",
        help="Output format (default: markdown)",
    )
    parser.add_argument(
        "--title",
        help="Custom title for the Markdown export",
    )
    parser.add_argument(
        "--with-summary",
        action="store_true",
        help="Also output a summary (printed to stderr or included in JSON)",
    )
    parser.add_argument(
        "--data-root",
        action="append",
        default=[],
        help=(
            "CodeBuddy 数据根目录（可多次指定，最高优先级）。"
            "通常由 AI 从 <artifact_directory_path> 推导得到，"
            "例如：~/Library/Application Support/CodeBuddy CN/User/globalStorage/tencent-cloud.coding-copilot"
        ),
    )
    parser.add_argument(
        "--claudecode-projects-dir",
        action="append",
        default=[],
        help=(
            "Claude Code projects 根目录（可多次指定，最高优先级）。"
            "通常由 AI 通过 shell 探测得到，例如：~/.claude-internal/projects"
        ),
    )
    parser.add_argument(
        "--cursor-projects-dir",
        action="append",
        default=[],
        help=(
            "Cursor projects 根目录（可多次指定，最高优先级）。"
            "通常为 ~/.cursor/projects"
        ),
    )
    parser.add_argument(
        "--workbuddy-projects-dir",
        action="append",
        default=[],
        help=(
            "WorkBuddy projects 根目录（可多次指定，最高优先级）。"
            "通常为 ~/.workbuddy/projects"
        ),
    )
    parser.add_argument(
        "--boxai-db-path",
        default="",
        help=(
            "Box AI sessions.db 数据库文件路径。"
            "通常为 ~/Library/Application Support/Box/engine/sessions.db"
        ),
    )
    parser.add_argument(
        "--assistant-model",
        default="",
        help=(
            "导出前由 AI 向 IDE 确认后的当前模型名。"
            "仅填充存档中未带 model 字段的 Assistant 消息，"
            "Markdown 中写入 ## Assistant <!-- MODEL:... -->；不覆盖 transcript 已有模型。"
        ),
    )
    parser.add_argument(
        "--detect-only",
        action="store_true",
        help="仅检测敏感信息并输出 JSON 报告到 stdout，不进行导出",
    )
    parser.add_argument(
        "--no-redact",
        action="store_true",
        help="导出时不进行敏感信息脱敏（用户明确选择不脱敏时使用）",
    )

    args = parser.parse_args()
    source = args.source
    extra_data_dirs: list[str] = list(args.data_root or [])
    extra_projects_dirs: list[str] = list(args.claudecode_projects_dir or []) + list(
        args.cursor_projects_dir or []
    ) + list(args.workbuddy_projects_dir or [])

    # Box AI: 若用户传入了 --boxai-db-path，设置环境变量让 get_boxai_db_path 优先使用
    boxai_db_path = (args.boxai_db_path or "").strip()
    if boxai_db_path:
        os.environ["BOXAI_DB_PATH"] = boxai_db_path

    # ---- List mode ----
    if args.list:
        convs = list_conversations(
            args.workspace_dir,
            source=source,
            extra_data_dirs=extra_data_dirs,
            extra_projects_dirs=extra_projects_dirs,
        )
        if not convs:
            print("❌ 未找到任何对话记录", file=sys.stderr)
            print(
                f"   workspace hash: {get_workspace_hash(args.workspace_dir)}",
                file=sys.stderr,
            )
            sys.exit(1)

        print(f"📋 找到 {len(convs)} 个对话记录:\n")
        for i, c in enumerate(convs):
            status = "📝" if c["message_count"] > 0 else "📭"
            src_tag = f" [{c.get('source', '')}]" if c.get("source") else ""
            print(f"  {status} [{i+1}]{src_tag} {c['id']}")
            print(f"      消息数: {c['message_count']}, 请求数: {c['request_count']}")
            print(f"      最后修改: {c['last_modified']}")
            print(f"      预览: {c['preview']}")
            print()
        return

    # ---- Determine conversation ID ----
    conv_id = args.conversation_id

    if args.auto and not conv_id:
        convs = list_conversations(
            args.workspace_dir,
            source=source,
            extra_data_dirs=extra_data_dirs,
            extra_projects_dirs=extra_projects_dirs,
        )
        # Find the most recent non-empty conversation
        for c in convs:
            if c["message_count"] > 0:
                conv_id = c["id"]
                src_tag = c.get("source", "")
                print(
                    f"🔍 自动选择最近的对话 [{src_tag}]: {conv_id} ({c['message_count']} 条消息)",
                    file=sys.stderr,
                )
                # 若 source=auto，根据选中对话的来源锁定 source
                if source == "auto":
                    source = src_tag or source
                break
        if not conv_id:
            print("❌ 未找到包含消息的对话", file=sys.stderr)
            sys.exit(1)

    if not conv_id:
        print("❌ 请指定 --conversation-id 或使用 --auto / --list", file=sys.stderr)
        sys.exit(1)

    # ---- Resolve 'auto' source before reading, so footer knows the real platform ----
    if source == "auto":
        dash_count = conv_id.count("-")
        if dash_count >= 4:
            if find_workbuddy_session_dir(
                args.workspace_dir, extra_projects_dirs=extra_projects_dirs
            ):
                source = "workbuddy"
            elif find_cursor_transcripts_dir(
                args.workspace_dir, extra_projects_dirs=extra_projects_dirs
            ):
                source = "cursor"
            elif get_boxai_db_path():
                source = "boxai"
            else:
                source = "claudecode"
        else:
            source = detect_source(
                args.workspace_dir,
                extra_data_dirs=extra_data_dirs,
                extra_projects_dirs=extra_projects_dirs,
            )

    # ---- Read conversation ----
    messages = read_conversation(
        args.workspace_dir,
        conv_id,
        source=source,
        extra_data_dirs=extra_data_dirs,
        extra_projects_dirs=extra_projects_dirs,
    )

    if not messages:
        print(f"⚠️  对话 {conv_id} 没有有效消息", file=sys.stderr)
        sys.exit(1)

    _apply_assistant_model_fallback(messages, args.assistant_model)

    # ---- Detect-only mode: scan for sensitive info and exit ----
    if args.detect_only:
        all_text = "\n".join(m.get("content", "") or "" for m in messages)
        findings = detect_sensitive_info(all_text)
        report = {
            "has_sensitive_info": len(findings) > 0,
            "count": len(findings),
            "findings": findings,
        }
        print(json.dumps(report, ensure_ascii=False, indent=2))
        sys.exit(0)

    # ---- Redact sensitive info (default unless --no-redact) ----
    total_redacted = 0
    if not args.no_redact:
        for msg in messages:
            content = msg.get("content", "")
            if content:
                redacted, count = redact_sensitive_info(content)
                if count > 0:
                    msg["content"] = redacted
                    total_redacted += count
        if total_redacted > 0:
            print(f"🔒 已脱敏 {total_redacted} 处敏感信息", file=sys.stderr)

    user_count = sum(1 for m in messages if m["role"] == "user")
    assistant_count = sum(1 for m in messages if m["role"] == "assistant")
    print(
        f"✅ 读取到 {len(messages)} 条消息 (用户: {user_count}, 助手: {assistant_count})",
        file=sys.stderr,
    )

    # ---- Generate summary if requested ----
    summary = ""
    if args.with_summary:
        summary = generate_summary(messages)
        print(f"📝 摘要: {summary}", file=sys.stderr)

    # ---- Format output ----
    if args.format == "json":
        output_data = {
            "conversation_id": conv_id,
            "workspace_dir": args.workspace_dir,
            "message_count": len(messages),
            "messages": messages,
        }
        if summary:
            output_data["summary"] = summary
        if args.title:
            output_data["title"] = args.title
        output = json.dumps(output_data, ensure_ascii=False, indent=2)
    else:
        output = conversation_to_markdown(messages, title=args.title, source=source)

    # ---- Write output ----
    if args.output:
        out_path = Path(args.output)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(output, encoding="utf-8")
        print(f"📄 已保存到: {out_path.absolute()}", file=sys.stderr)
    else:
        print(output)


if __name__ == "__main__":
    main()
