# session-share

将当前 Cursor / CodeBuddy / Claude Code 的会话（session）一键导出为 Markdown 并上传到 **ChatSpark** 平台的 Skill。

> 与 `SKILL.md`（AI Agent 指令文档）相对，本 README 面向**人类用户**，介绍 skill 是什么、能做什么、怎么配置和使用。

---

## 功能概览

- **完整对话导出**：直接读取 IDE 本地对话存档（`~/Library/Application Support/CodeBuddy CN/...`、`~/.claude*/projects`、`~/.cursor/projects` 等），不受 AI 上下文窗口长度限制。
- **多 IDE 适配**：自动识别 Cursor / CodeBuddy / Claude Code / WorkBuddy / Box AI 的存档格式与路径约定。
- **敏感信息脱敏**：默认对 API Token、Auth Header 等高置信度敏感串自动 `***REDACTED***`，可用 `--no-redact` 关闭。
- **思考过程与过程叙述折叠**：自动识别模型思考（`thinking` / `reasoning_content`）以及工具调用之间的过渡叙述，用 HTML5 原生 `<details>` 折叠呈现，渲染端默认收起，让长会话更紧凑；展开后原文完整可读。
- **一键上传**：通过 Supabase Edge Function 中转，将 Markdown + 元数据上传到 ChatSpark；失败可自动回退到浏览器手动上传页。
- **AI 引导式交互**：通过 `SKILL.md` 中定义的工作流，AI Agent 会主动询问用户「是否上传 / 是否脱敏 / API Key 配置方式」等关键决策点。

---

## 目录结构

```
session-share/
├── SKILL.md              # AI Agent 读取的技能指令文档
├── README.md             # 当前文件，面向用户的说明文档
├── package.json          # skill 元数据
├── .gitignore            # 忽略本地产物
└── scripts/
    ├── read_chat_history.py   # 读取并导出对话存档为 Markdown
    └── upload_session_rpc.py  # 上传 Markdown 到 ChatSpark
```

---

## 环境要求

| 依赖 | 版本 | 说明 |
|---|---|---|
| Python | ≥ 3.8 | 脚本运行时，仅使用标准库（`urllib` / `argparse` / `json` / `pathlib`） |
| 操作系统 | macOS / Linux / Windows | 浏览器调起命令分别为 `open` / `xdg-open` / `start` |

无需 `pip install` 任何第三方包，也无需 Node 依赖；`package.json` 仅作为 skill 元数据登记用。

---

## 快速开始

### 1. 触发 Skill

在你的 AI IDE 对话中说出以下任意一句即可触发：

- "分享对话"
- "上传到 ChatSpark"
- "把对话发到 ChatSpark"
- "share session" / "session share"
- "导出并上传对话"

AI Agent 会按 `SKILL.md` 定义的工作流，依次完成：

1. 探测本地对话存档路径
2. 导出当前会话为 Markdown
3. 扫描敏感信息并询问是否脱敏
4. 询问是否自动上传到 ChatSpark
5. 自动上传 / 手动上传 / 仅保留本地 三选一

### 2. 配置 API Key（仅自动上传需要）

自动上传需要一个 ChatSpark API Key。三种方式任选：

| 方式 | 配置位置 | 适用场景 |
|---|---|---|
| 环境变量 | `export CHATS_TOKEN="cb_sk_xxx"` | 临时使用、CI 场景 |
| 自定义目录 | `export CHATS_HOME=/path/to/dir`，并把 token 写入 `$CHATS_HOME/chats_token` | 多账号切换 |
| 默认路径 | `~/.chats/chats_token`（权限 600） | 个人长期使用（**推荐**） |

> Skill 流程会在未检测到 token 时主动引导申请，无需手动操作。
> 申请页：<https://chats.woa.com/session/api-keys>

### 3. 手动上传（无 API Key 时的兜底）

如果不想申请 API Key，可在 skill 流程中选择「转为手动上传」，会自动打开：

<https://chats.woa.com/session/upload>

把 skill 导出的 `chat-export-*.md` 文件直接拖进去即可。

---

## 脚本独立调用（高级）

如果你想脱离 AI Agent 直接跑脚本：

### 导出对话

```bash
# 列出所有可用对话
python3 scripts/read_chat_history.py --workspace-dir "/path/to/your/project" --list

# 自动导出最近一条对话
python3 scripts/read_chat_history.py \
  --workspace-dir "/path/to/your/project" \
  --auto \
  --output "./chat-export.md" \
  --with-summary
```

### 上传对话

```bash
python3 scripts/upload_session_rpc.py \
  --project-dir "/path/to/your/project" \
  --md-file "./chat-export.md" \
  --title "我的会话标题" \
  --summary "本次会话讨论了 X 问题"
# 标签由 AI 自动生成（总数 1-4 个，且至少包含一个平台预设标签）
```

更多参数请运行 `python3 scripts/<name>.py --help` 查看。

---

## 环境变量速查

| 变量名 | 用途 | 默认值 |
|---|---|---|
| `CHATS_TOKEN` | 直接传入 API Key | — |
| `CHATS_HOME` | 自定义 token 存放目录 | `~/.chats` |
| `CHATS_UPLOAD_RELAY_URL` | 覆盖默认上传中转地址 | Supabase Edge Function 默认地址 |
| `CHATS_RELAY_APIKEY` | 中转层 API Key（一般不用配置） | — |
| `CODEBUDDY_DATA_ROOT` | CodeBuddy 数据根，多个用 `:` 分隔 | 自动探测 |
| `CLAUDECODE_PROJECTS_DIR` | Claude Code projects 根 | 自动探测 |

---

## 隐私与安全

- 默认开启敏感信息脱敏，覆盖 GitHub Token、Bearer、OpenAI / Anthropic API Key 等常见格式。
- API Key 文件默认权限 `600`，仅当前用户可读。
- 脚本**只读取**本地存档，不会修改原始 IDE 数据。
- 上传内容为脱敏后的 Markdown，原始对话不出本机。

---

## 命名说明

- token 文件、环境变量统一使用 `chats_*` / `CHATS_*` 前缀，对应平台 **ChatSpark**（`chats.woa.com`）。
- 触发词、文档措辞、UI 文案统一使用 **ChatSpark**。

---

## License

MIT
