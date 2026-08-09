---
name: session-share
description: 当用户想要导出、保存、归档、分享、上传当前 AI 对话/会话到 ChatSpark 平台时使用此 Skill。触发词包括："分享对话"、"分享 session"、"导出对话"、"保存聊天记录"、"归档这次对话"、"上传到 ChatSpark"、"share session"、"session share"、"export this chat"、"备份这次对话"、"沉淀到知识库"，或直接提到 "ChatSpark"。也适用于用户想把当前对话作为复盘材料、发给队友查看、或发布到团队知识库的场景。支持五种 IDE：Cursor、CodeBuddy、Claude Code、WorkBuddy、Box AI。当用户仅讨论对话内容或清空历史时不触发。
---

# 分享对话到 ChatSpark

将当前 Cursor/CodeBuddy/Claude Code/WorkBuddy/Box AI 会话导出为 Markdown 并保存到本地，优先通过线上中转 API 自动上传到 ChatSpark；若自动上传失败，再回退到浏览器手动上传。

**核心能力**：通过 `read_chat_history.py` 脚本直接读取 Cursor/CodeBuddy/Claude Code/WorkBuddy/Box AI 本地对话存档文件，获取**完整**的对话历史，不受 AI 上下文窗口长度限制。

> 🚫 **最高优先级约束 — 所有 IDE 必须遵守**：
>
> **一、禁止探索数据结构。** 你不需要"先看看数据长什么样"——脚本已经知道。
> 不管你是什么 IDE，不要试图在调用脚本之前去读存档文件的内容：
> - Box AI → 不要 `sqlite3` / `SELECT` / `PRAGMA table_info`
> - Claude Code / Cursor / WorkBuddy → 不要 `cat` / `head` / `python3 -c "open(...)"` 读 jsonl 文件
> - CodeBuddy → 不要读 `index.json` 或 `messages/*.json`
>
> **探索数据 → 手写导出是一条滑坡路径**：模型看到数据格式后会产生"这东西很简单我自己写一个就行"的错觉，
> 然后写出几十行内联代码——缺失工具解析、Timeline 标记、敏感信息脱敏等全部能力。
>
> **二、禁止自行编写导出逻辑：**
> - ❌ `python3 -c "..."` 内联 Python 代码
> - ❌ `python3 << 'EOF' ... EOF` heredoc 脚本
> - ❌ 任何不经过 `read_chat_history.py` 的直接读文件 + 手写 Markdown 的方式
>
> **三、导出对话的唯一正确方式是调用本 skill 自带的脚本：**
> ```
> python3 {SKILL_DIR}/scripts/read_chat_history.py --workspace-dir ... --source ... --conversation-id ...
> ```
>
> 该脚本经过 4000+ 行代码的充分测试，能正确处理：
> - 五种 IDE 的不同存档格式（JSONL / SQLite / 多层目录结构）
> - 工具调用的完整解析（工具名、参数摘要、执行结果预览、错误状态）
> - 思考过程（thinking / reasoning）的提取与 Timeline 整合
> - 敏感信息自动检测与脱敏（API Key、Token、Auth Header 等）
> - 系统注入内容过滤（system_reminder、skill 内容注入、compaction 摘要等）
> - 与 ChatSpark 前端渲染契约兼容的 `<!-- TOOL:... -->` / `<!-- TIMELINE_START -->` 标记格式
>
> **任何自行编写的简化版导出代码都必然丢失上述能力，导致导出结果残缺——**
> 典型症状：所有工具调用仅显示一句通用的「🔧 调用了工具」，看不见具体调用了什么工具、传了什么参数、返回了什么结果。
>
> **脚本执行失败 ≠ 授权你手写替代**。如果脚本返回非 0，按本文档「错误处理」章节修正参数重试，
> 不要转而写内联代码"补救"。

## 触发场景

当用户提到以下意图时使用此 Skill：
- "分享会话" / "分享这个对话"
- "上传到 ChatSpark"
- "把对话发到 ChatSpark"
- "share session" / "session share"
- "导出并上传对话"

## 完整工作流

### 第零步：探测本地对话存档位置（必做）

**不要依赖任何硬编码路径。** 你（AI）应像一个开发者一样，利用操作系统约定和文件系统命令，主动探测当前 IDE 的对话存档位置。

**执行原则（强约束）**：
- 必须先执行路径探测，再告诉用户本次命中的真实路径。
- 必须把探测结果显式传给脚本（`--data-root` / `--claudecode-projects-dir` / `--cursor-projects-dir`）。
- 脚本中的内置候选路径仅作为最后兜底，不应作为首选方案或默认假设。
- 当探测路径与内置候选不一致时，以探测结果为准，并在回复中明确说明。

以下是一个**从快到慢、逐级深入**的探测流程。每一级成功即可停止，把找到的路径通过 `--data-root`（CodeBuddy 系）/`--claudecode-projects-dir`（Claude Code 系）/`--cursor-projects-dir`（Cursor 系）传给脚本。

#### Level 0：从上下文信息快速推导（零成本）

如果你的运行环境注入了 `<artifact_directory_path>`，可以直接推导：

```
<artifact_directory_path> 形如：
  .../SomeApp/User/globalStorage/tencent-cloud.coding-copilot/brain/<convId>

DATA_ROOT = 去掉 "/brain/<convId>" 后的部分
CONVERSATION_ID = 最后一段路径
```

> 注意：这个路径**不一定包含 history 子目录**（history 可能在同级别的另一个应用目录下）。没关系，脚本会先试这个路径，命不中自动回退。把它当作"第一个线索"即可。

#### Level 1：从 OS 标准数据目录 + 模糊搜索定位应用

几乎所有桌面应用都遵循操作系统的标准数据目录约定：

| 操作系统 | 标准数据根目录 |
|---------|--------------|
| macOS   | `~/Library/Application Support/` |
| Linux   | `~/.config/` 或 `~/.local/share/` |

根据**你自己是哪个 AI IDE**，用对应的关键词模糊搜索：

```bash
# 你是 CodeBuddy → 搜 codebuddy
ls ~/Library/Application\ Support/ | grep -i "codebuddy"

# 你是 Claude Code → 搜 claude
ls -d ~/.claude*/projects 2>/dev/null

# 你是 Cursor → 搜 cursor projects
ls -d ~/.cursor/projects 2>/dev/null

# 你是 WorkBuddy → 搜 workbuddy projects
ls -d ~/.workbuddy/projects 2>/dev/null

# 你是 Box AI → 搜 Box engine
ls ~/Library/Application\ Support/Box/engine/sessions.db 2>/dev/null

# Linux 同理，搜 ~/.config/ 和 ~/.local/share/
ls ~/.config/ | grep -i "codebuddy"
ls ~/.local/share/ | grep -i "codebuddy"
```

> 💡 如果是基于 VS Code / Electron 的扩展（如 CodeBuddy、Cursor），路径中通常会带 `User/globalStorage/` 或 `User/workspaceStorage/`。

#### Level 2：用语义关键词定位会话子目录

进入 Level 1 找到的应用数据目录后，搜索可能存放会话的子目录：

```bash
# 常见的会话相关目录名关键词
find "{APP_DATA_DIR}" -maxdepth 6 -type d \( \
  -iname "*history*" -o -iname "*conversation*" -o \
  -iname "*session*" -o -iname "*chat*" \
\) 2>/dev/null
```

#### Level 3：热文件捕获（利用"正在对话"这个事实）

你和用户正在对话，意味着存档文件**此刻正在被写入**。利用这一点：

```bash
# 找最近 5 分钟内修改过的 JSON/JSONL 文件
find "{APP_DATA_DIR}" -type f \( -name "*.json" -o -name "*.jsonl" \) -mmin -5 2>/dev/null
```

从结果中反推父目录结构，即可定位 history 根目录。

#### Level 3.5：反推校正 workspace_dir（应对 symlink 工作区）

> ⚠️ **重要**：脚本在 v1.x 之后已内置 abspath / realpath 双候选 fallback，
> 即使你传一个 symlink 路径作为 `--workspace-dir`，脚本也会自动尝试 realpath
> 候选并在命中时打印「(回退候选)」提示。**所以这一步并非必做**，但加上能让
> AI 先于脚本主动校准 `--workspace-dir`，更准确、更可解释。

**典型触发场景**：
- 用户用 `ln -s` 把 claude 标准版 skills 链到 `~/.claude-internal/skills`
- 用户从 `/tmp/...` 启动 IDE（macOS 上 `/tmp` 本身是 `/private/tmp` 的 symlink）
- 工作区路径含 iCloud Drive / Docker bind mount 等 symlink 重定向

**反推步骤**（依赖 Level 3 已经找到了热 jsonl）：

1. 拿到热文件路径，例如：
   `~/.claude-internal/projects/-Users-xxx-Desktop-realproj/375fa47b-....jsonl`

2. 取**倒数第二段**（桶名）：`-Users-xxx-Desktop-realproj`

3. 反编码：把首字符 `-` 之后的所有 `-` 替换为 `/`
   → `/Users/xxx/Desktop/realproj`

4. 与 `<user_info>` 中 `Workspace Folder` 对比：
   - **一致** → 直接用，无 symlink 问题
   - **不一致** → ⚠️ workspace 是 symlink，**用反推出的路径作为 `--workspace-dir`**

5. 交叉验证（可选）：
   ```bash
   python3 -c "import os; print(os.path.realpath('<user_info 路径>'))"
   ```
   输出应与第 3 步反推结果一致。

**实操示例**：

```bash
# 假设 Level 3 找到了刚被写的热文件
HOT_JSONL=$(find ~/.claude-internal/projects -name "*.jsonl" -mmin -2 | head -1)

# 反推真实 workspace
BUCKET_NAME=$(basename "$(dirname "$HOT_JSONL")")
TRUE_WORKSPACE="/${BUCKET_NAME#-}"           # 去掉首字符 '-' 加根斜杠
TRUE_WORKSPACE="${TRUE_WORKSPACE//-//}"      # 把 '-' 还原成 '/'
echo "Claude 实际感知的 workspace: $TRUE_WORKSPACE"

# 与 user_info 比较，不一致则用 $TRUE_WORKSPACE 传给 --workspace-dir
```

> 注意：如果工作区路径本身含 `-` 字符（罕见），上述反编码会失真。这种情况下
> 直接信任脚本内置的 abspath/realpath 候选 fallback 即可。

#### Level 4：从进程信息反推（兜底）

如果前几步都不够明确，可以从运行中的进程入手：

```bash
# macOS：查看当前 IDE 进程打开了哪些文件
lsof -c "CodeBuddy" 2>/dev/null | grep -iE "history|conversation|session|\.json" | head -10

# 或者用进程名模糊匹配
lsof -p $(pgrep -f "codebuddy" | head -1) 2>/dev/null | grep -E "\.(json|db|sqlite)" | head -10
```

#### Level 5：确认文件格式

找到候选文件后，快速验证它是对话存档：

```bash
cat "{CANDIDATE_FILE}" | python3 -m json.tool 2>/dev/null | head -30
```

对话存档的典型特征：
- 有 `role` 字段（`user` / `assistant` / `tool`）
- 有 `content` 字段（消息正文）
- 有时间戳或 ID 信息
- CodeBuddy：目录中包含 `index.json` + `messages/` 子目录
- Claude Code：单个 `.jsonl` 文件，每行一条消息
- Cursor：`~/.cursor/projects/{workspace-encoded}/agent-transcripts/{conversationId}/{conversationId}.jsonl`
- WorkBuddy：`~/.workbuddy/projects/{workspace-encoded}/{sessionId}.jsonl`（JSONL 格式，编码规则去掉开头 `-`）
- Box AI：SQLite 数据库 `~/Library/Application Support/Box/engine/sessions.db`，`messages` 表存储对话内容

#### Level 6：问用户（最终兜底）

如果以上所有方式都未能定位，直接询问用户：

```
未能自动找到对话存档目录。请问您的 CodeBuddy（或 Claude Code）安装在哪个位置？
或者，您能提供对话存档的路径吗？
```

#### 探测结果的使用

将探测到的路径传给脚本（可多次指定，脚本会按顺序尝试，自动跳过不存在的）：

```bash
# CodeBuddy 系
--data-root "{探测到的路径1}" --data-root "{探测到的路径2}"

# Claude Code 系
--claudecode-projects-dir "{探测到的路径}"

# Cursor 系
--cursor-projects-dir "{探测到的路径}"

# WorkBuddy 系
--workbuddy-projects-dir "{探测到的路径}"

# Box AI 系
--boxai-db-path "{探测到的路径}"
```

同时根据你自己的身份确定 `--source` 参数：
- 你是 **CodeBuddy** → `--source codebuddy`
- 你是 **Claude Code** → `--source claudecode`
- 你是 **Cursor** → `--source cursor`
- 你是 **WorkBuddy** → `--source workbuddy`
- 你是 **Box AI** → `--source boxai`
- 不确定 → 省略 `--source`，脚本会自动判别

在执行导出前，先向用户回报一次探测结果（建议）：

```text
已探测到本次会话存档路径：
- source: {cursor|claudecode|codebuddy}
- path: {detected_path}

将优先使用该路径导出；仅在读取失败时才回退脚本内置候选路径。
```

> 💡 **关于 symlink workspace 的兜底**：如果 `--workspace-dir` 是 symlink 路径
> （或路径里任何一段是 symlink），脚本会**自动尝试 abspath / realpath / 别名前缀**
> 三种编码候选，命中时 stderr 会提示「(回退候选)」。所以即使第零步未能反推校正，
> 脚本端依然能兜住，不会出现"找不到会话"的硬失败。

### 第一步前：当前模型（仅 Cursor，须在生成 MD 之前完成）

Cursor 的本地 transcript 可能**不写**模型字段。导出 Markdown 前必须先拿到当前模型名，再通过脚本写入 `## Assistant <!-- MODEL:... -->`，**不要**在生成文件后再手改 MD。

1. **在运行 `read_chat_history.py` 之前**，在当前对话中向 IDE/运行时确认：**「你现在使用的是什么模型？」**（或等价英文），得到 `{CURRENT_MODEL}`。
2. 调用导出命令时**必须**传入 `--assistant-model "{CURRENT_MODEL}"`。
3. 脚本行为：对每条 **transcript 中未解析出模型** 的 Assistant 消息，使用 `{CURRENT_MODEL}`，输出为 `## Assistant <!-- MODEL:{CURRENT_MODEL} -->`；若某条消息存档里**已有**模型字段，**以存档为准，不覆盖**。

CodeBuddy / Claude Code 一般以存档内模型为准；若个别消息缺字段，也可传 `--assistant-model` 仅填补空缺。

### 第一步：从本地存档读取完整对话历史

**通过脚本直接读取本地对话存档文件，不依赖 AI 上下文记忆。**

> ⚠️ **Box AI 特别注意**：Box AI 的对话存档是 **SQLite 数据库**（`sessions.db`），不是 JSON/JSONL 文件。
> 与其他 IDE 不同，Box AI 的存档是单一 `.db` 文件而非多个文本文件，这更容易诱发「让我用 sqlite3 查一下数据结构」的探索冲动。
> 请遵守顶部「最高优先级约束」：**不要探索、不要手写，直接用下面的命令模板**。
>
> **Box AI 命令模板（直接套用，替换 `{CONVERSATION_ID}` 和 `{PROJECT_DIR}`）：**
>
> ```bash
> python3 {SKILL_DIR}/scripts/read_chat_history.py \
>   --workspace-dir "{PROJECT_DIR}" \
>   --source boxai \
>   --boxai-db-path ~/Library/Application\ Support/Box/engine/sessions.db \
>   --conversation-id "{CONVERSATION_ID}" \
>   --output "{PROJECT_DIR}/chat-export-{YYYY-MM-DD-HHmmss}.md"
> ```

#### 方式 A：导出指定对话（推荐）

如果知道当前对话的 conversation ID（通常可从 `<artifact_directory_path>` 末段或 Level 2/3 探测结果中获取），直接导出：

```bash
python3 {SKILL_DIR}/scripts/read_chat_history.py \
  --workspace-dir "{PROJECT_DIR}" \
  --source {SOURCE} \
  --data-root "{DETECTED_PATH_1}" \
  --data-root "{DETECTED_PATH_2}" \
  --conversation-id "{CONVERSATION_ID}" \
  --output "{PROJECT_DIR}/chat-export-{YYYY-MM-DD-HHmmss}.md" \
  --with-summary
```

**Cursor 时在同一命令中追加**（`{CURRENT_MODEL}` 来自「第一步前」向 IDE 确认后的结果）：

```bash
  --cursor-projects-dir "{DETECTED_PATH}" \
  --assistant-model "{CURRENT_MODEL}" \
```

> `{DETECTED_PATH_1}`、`{DETECTED_PATH_2}` 等为第零步探测到的路径。可传入多个，脚本按顺序尝试。
> Claude Code 场景请把 `--data-root` 改为 `--claudecode-projects-dir`；Cursor 场景改为 `--cursor-projects-dir`，并务必附带 `--assistant-model`。

#### 方式 B：自动选择最近的对话

如果无法确定 conversation ID，可用 `--auto` 自动选择最近一个有消息的对话：

```bash
python3 {SKILL_DIR}/scripts/read_chat_history.py \
  --workspace-dir "{PROJECT_DIR}" \
  --data-root "{DETECTED_PATH_1}" \
  --data-root "{DETECTED_PATH_2}" \
  --auto \
  --output "{PROJECT_DIR}/chat-export-{YYYY-MM-DD-HHmmss}.md" \
  --with-summary
```

Cursor 时同样追加 `--cursor-projects-dir` 与 `--assistant-model "{CURRENT_MODEL}"`（见方式 A）。

#### 方式 C：列出所有对话后选择

```bash
python3 {SKILL_DIR}/scripts/read_chat_history.py \
  --workspace-dir "{PROJECT_DIR}" \
  --data-root "{DETECTED_PATH_1}" \
  --data-root "{DETECTED_PATH_2}" \
  --list
```

脚本会自动：
- **优先**使用 AI 探测后显式传入的路径（`--data-root` / `--claudecode-projects-dir` / `--cursor-projects-dir`）
- 若未传入或路径下找不到 history，再回退到内置候选自动探测
- 按 `index.json` 中的消息顺序还原完整对话
- 提取用户原始输入（从 `sourceContentBlocks`，过滤系统元数据）
- 提取助手文本回复（过滤 tool_use 等技术细节）
- 合并连续同角色消息
- 生成标准 Markdown 格式输出
- 若传入 `--assistant-model`，仅对缺少存档模型字段的 Assistant 消息写入 `<!-- MODEL:... -->`（见「第一步前」）

> 🚨 **导出脚本执行成功后的强制延续动作**：脚本输出 "📄 已保存到: ..." **不是终点**。AI **必须立即在同一轮**继续执行：第二步（优化标题/摘要）→ 第三步（向用户展示并调用 `AskUserQuestion` / `ask_followup_question` 询问是否上传）。**绝不允许在 `📄 已保存到: ...` 之后就停止响应让用户看一个孤零零的导出结果**。这一条对 CodeBuddy / Box AI 尤其关键——这两个 IDE 看到 artifact 文件产出可能误判为"任务完成"，所以 AI 必须主动把整个 1→2→3 链条跑完才能让出对话权。

### 第一步半：敏感信息检测与用户确认（导出前必做）

在正式导出 Markdown 之前，先用 `--detect-only` 模式扫描对话中是否包含敏感信息，**让用户决定是否脱敏**：

```bash
python3 {SKILL_DIR}/scripts/read_chat_history.py \
  --workspace-dir "{PROJECT_DIR}" \
  --source {SOURCE} \
  --data-root "{DETECTED_PATH_1}" \
  --conversation-id "{CONVERSATION_ID}" \
  --detect-only
```

脚本会以 JSON 格式输出检测结果到 stdout：

```json
{
  "has_sensitive_info": true,
  "count": 3,
  "findings": [
    {"type": "GitHub Token", "match": "ghp_ABCDEFghijklmnop1234567890", "preview": "ghp_***REDACTED***"},
    {"type": "Bearer Auth Header", "match": "Bearer eyJhbGci...", "preview": "Bearer ***REDACTED***"},
    {"type": "OpenAI API Key", "match": "sk-1234567890abcdef...", "preview": "sk-***REDACTED***"}
  ]
}
```

**根据检测结果与用户交互**：

- 若 `has_sensitive_info` 为 `false`：直接告知用户「未检测到敏感信息」，进入第一步正式导出（使用 `--no-redact` 跳过多余的脱敏步骤）。
- 若 `has_sensitive_info` 为 `true`：先输出检测结果文本（逐条列出具体发现），然后根据当前运行环境选择交互方式：

```
🔍 导出前敏感信息扫描完成，检测到 {count} 处敏感信息：
  1. 🔑 GitHub Token: ghp_ABCDEF... → ghp_***REDACTED***
  2. 🔑 Bearer Auth Header: Bearer eyJhbG... → Bearer ***REDACTED***
  3. 🔑 OpenAI API Key: sk-123456... → sk-***REDACTED***
```

**【Claude Code】** 调用 `AskUserQuestion` 工具弹出点选 UI：

```
AskUserQuestion({
  questions: [{
    header: "敏感信息",
    question: "检测到 {count} 处敏感信息，是否在导出时自动脱敏？",
    multiSelect: false,
    options: [
      { label: "自动脱敏（推荐）", description: "将 Token、密钥等替换为 ***REDACTED***，保护隐私" },
      { label: "保留原始内容", description: "不进行脱敏，保留所有原始数据导出" }
    ]
  }]
})
```

**【Cursor】** 调用 `ask_followup_question` 工具，通过 `suggestions` 渲染点选按钮：

```
ask_followup_question({
  question: "检测到 {count} 处敏感信息，是否在导出时自动脱敏？",
  suggestions: ["自动脱敏（推荐）", "保留原始内容"]
})
```

**【CodeBuddy】** 调用 `ask_followup_question` 工具，通过 `options` 渲染点选按钮；若工具不可用则退回文字话术等待用户输入「是/否」：

```
ask_followup_question({
  question: "检测到 {count} 处敏感信息，是否在导出时自动脱敏？",
  options: ["自动脱敏（推荐）", "保留原始内容"]
})
```

**【WorkBuddy】** 调用 `AskUserQuestion` 工具（与 Claude Code 相同）：

```
AskUserQuestion({
  questions: [{
    header: "敏感信息",
    question: "检测到 {count} 处敏感信息，是否在导出时自动脱敏？",
    multiSelect: false,
    options: [
      { label: "自动脱敏（推荐）", description: "将 Token、密钥等替换为 ***REDACTED***，保护隐私" },
      { label: "保留原始内容", description: "不进行脱敏，保留所有原始数据导出" }
    ]
  }]
})
```

**【Box AI】** 调用 `ask_followup_question` 工具（与 CodeBuddy 一致）：

```
ask_followup_question({
  question: "检测到 {count} 处敏感信息，是否在导出时自动脱敏？",
  options: ["自动脱敏（推荐）", "保留原始内容"]
})
```

根据用户点选（或输入）结果，在第一步正式导出时：
- 用户选「自动脱敏（推荐）」/ 回复「是」：正常运行导出命令（默认启用脱敏）
- 用户选「保留原始内容」/ 回复「否」：导出时追加 `--no-redact` 参数

### 第二步：生成标题和摘要

> 🚨 **关键执行约束（CodeBuddy / Box AI 必读）**：
> **第一步导出 Markdown 完成后，绝对不允许停下来等待用户回复**。导出脚本结束 → **必须立即在同一轮回复内**继续执行第二步（生成标题/摘要）和第三步（向用户展示并询问是否上传）。看到 "📄 已保存到: ..." 不是任务完成信号，只是工作流的中间状态。
>
> **典型错误**（绝对禁止）：
> - ❌ 导出成功 → 输出"任务完成 / 已导出 / 请查看本地文件" → 停止响应
> - ❌ 导出成功 → 直接结束本轮，等用户说"继续"
> - ❌ 导出成功 → 仅展示文件路径就退出
>
> **正确行为**：
> - ✅ 导出成功 → 同一轮内继续：读取 MD → 生成标题摘要 → 调用 AskUserQuestion / ask_followup_question 询问上传 → **此时**才允许等待用户响应

脚本已自动生成基础标题和摘要。**AI 应阅读导出的 Markdown 文件，优化标题和摘要**：

1. 读取导出的 Markdown 文件
2. 从对话核心内容概括一个**不超过 20 字**的标题
3. 撰写一段**不超过 200 字**的摘要，概括对话讨论了什么问题、如何解决的
4. 默认可见范围 `visibility="public"`（公开），后续在第三步由用户确认或修改

完成后**立即**进入第三步（同一轮内调用询问工具，不要先停下输出"已生成标题/摘要"）。

### 第三步：先征询用户是否自动上传

导出成功后，**必须先把当前的「标题 / 摘要 / 可见范围」三项展示给用户**，再询问是否要自动上传到 ChatSpark。**此时不需要检查本地 token 是否存在**——token 的检查与处理放在第四步入口。

> ⚠️ **可见范围只暴露公开 / 私密两个选项**：数据库实际支持 `public/team/private` 三种值，但「团队 (team)」需要单独绑定 team 关联表，CLI/对话场景下交互成本高，本 skill 暂不暴露团队选项；如需团队可见，请在 ChatSpark 网页端二次编辑。

先输出导出完成信息（含三项默认值），然后根据当前运行环境选择交互方式：

```
✅ 对话已导出完成。
📄 本地文件：{exported_md_file_path}
📝 标题：{title}
📋 摘要：{summary}
🔒 可见范围：公开（public）

ℹ️ 上传选项说明：
  • 立即上传 → 直接使用上方默认值上传（可见范围=公开）
  • 修改后上传 → 可修改「标题 / 摘要 / 可见范围」三项中的任意一项或多项再上传
    └─ 注：可见范围仅支持「公开 / 私密」，不支持「团队可见」（如需团队可见，请上传后到 ChatSpark 网页端二次编辑）
  • 暂不上传 → 仅保留本地文件
```

**【Claude Code】** 调用 `AskUserQuestion` 工具弹出点选 UI：

```
AskUserQuestion({
  questions: [{
    header: "上传 ChatSpark",
    question: "是否现在自动上传到 ChatSpark？「修改后上传」可调整标题、摘要、可见范围。",
    multiSelect: false,
    options: [
      { label: "立即上传", description: "使用默认值直接上传，可见范围=公开" },
      { label: "修改后上传", description: "可修改标题/摘要/可见范围中的任意一项或多项（可见范围仅支持公开或私密）" },
      { label: "暂不上传", description: "仅保留本地文件，稍后可手动上传" }
    ]
  }]
})
```

**【Cursor】** 调用 `ask_followup_question` 工具，通过 `suggestions` 渲染点选按钮：

```
ask_followup_question({
  question: "是否现在自动上传到 ChatSpark？「修改后上传」可调整标题、摘要、可见范围。",
  suggestions: ["立即上传", "修改后上传", "暂不上传"]
})
```

**【CodeBuddy】** 调用 `ask_followup_question` 工具，通过 `options` 渲染点选按钮；若工具不可用则退回文字话术等待用户输入「立即上传 / 修改 / 否」：

```
ask_followup_question({
  question: "是否现在自动上传到 ChatSpark？「修改后上传」可调整标题、摘要、可见范围。",
  options: ["立即上传", "修改后上传", "暂不上传"]
})
```

**【WorkBuddy】** 调用 `AskUserQuestion` 工具（与 Claude Code 相同的交互方式）：

```
AskUserQuestion({
  questions: [{
    header: "上传 ChatSpark",
    question: "是否现在自动上传到 ChatSpark？「修改后上传」可调整标题、摘要、可见范围。",
    multiSelect: false,
    options: [
      { label: "立即上传", description: "使用默认值直接上传，可见范围=公开" },
      { label: "修改后上传", description: "可修改标题/摘要/可见范围中的任意一项或多项（可见范围仅支持公开或私密）" },
      { label: "暂不上传", description: "仅保留本地文件，稍后可手动上传" }
    ]
  }]
})
```

**【Box AI】** 调用 `ask_followup_question` 工具（与 CodeBuddy 一致）：

```
ask_followup_question({
  question: "是否现在自动上传到 ChatSpark？「修改后上传」可调整标题、摘要、可见范围。",
  options: ["立即上传", "修改后上传", "暂不上传"]
})
```

- 用户点选（或回复）「立即上传」→ 进入第四步（先做 token 预检），使用默认 `title / summary / visibility="public"`
- 用户点选（或回复）「修改后上传」→ 进入**第三步半**逐项修改，修改完成后再进入第四步
- 用户点选（或回复）「暂不上传」/ 「否」→ 跳过自动上传，执行第五步手动上传引导

#### 第三步半：修改标题 / 摘要 / 可见范围（仅在用户选「修改后上传」时进入）

**核心原则**：标题、摘要、可见范围三项**互相独立、不存在前后依赖**。让用户自由勾选要改哪些（多选），不强制顺序，未勾选的项**保留默认**。

> 🚫 **绝对禁止的反模式（典型故障案例）**：
>
> ❌ **不允许把「选要改哪些」和「最终确认」合并成一个菜单**。下面这种菜单是**严重违规**，绝对禁止生成：
> ```
> ❌ 错误示例（禁止生成）：
> 最终确认 - 确认上传，还是继续修改？
> · 确认上传
> · 修改摘要
> · 改为私密
> · 重新修改标题
> · 取消上传
> ```
> 这种"5 选 1 单选合并菜单"违反了三项独立原则——它逼用户改完一项后回到这个菜单再选下一项，体验极差，并且把「选改哪些项 ①」和「最终确认 ⑤」混淆为同一步。
>
> ✅ **正确做法**：①（选要改哪些）和 ⑤（最终确认）**必须是两个独立的工具调用**。① 调用一次让用户多选；中间穿插 ②③④ 修改具体值；最后 ⑤ 再单独调用一次让用户确认。
>
> 🔒 **选项标签必须照搬，禁止 AI 自由发挥**：必须使用下面给出的精确文案，不允许改成"改为私密 / 重新修改标题"这类自由发挥措辞。

##### ① 让用户多选要修改哪些项

> 🎯 **统一原则：所有 IDE 都使用「多选」工具**。用户一次勾选要改的项，未勾选的保留默认值。**禁止退化为单选 + 循环菜单**——单选循环体验差，且 AI 容易在循环中跑偏。

```
【Claude Code / WorkBuddy】使用 AskUserQuestion 的 multiSelect:true
AskUserQuestion({
  questions: [{
    header: "选择修改项",
    question: "请勾选要修改的项目（可多选；未勾选的保留默认值；不勾选任何项 = 全部保留）",
    multiSelect: true,
    options: [
      { label: "修改标题", description: "当前：{title}" },
      { label: "修改摘要", description: "当前：{summary}" },
      { label: "修改可见范围", description: "当前：公开（仅支持公开/私密，不支持团队）" }
    ]
  }]
})

【Cursor / CodeBuddy / Box AI】使用 ask_followup_question 的 multiSelect 参数
ask_followup_question({
  question: "请勾选要修改的项目（可多选；未勾选的保留默认值；不勾选任何项 = 全部保留）",
  multiSelect: true,
  options: [
    "修改标题（当前：{title}）",
    "修改摘要（当前：{summary}）",
    "修改可见范围（当前：公开）"
  ]
})
```

> 💡 **执行规则**：
> - 用户**勾选 0 项**（确认了空选择）→ 跳过 ②③④ 直接到 ⑤ 最终确认（全部保留默认）
> - 用户**勾选 1～3 项**→ 仅对勾选项按 ②（可见范围）→ ③（标题）→ ④（摘要）的顺序询问，未勾选的保持默认
> - 顺序仅是 AI 询问的内部顺序，对用户来说三项是平等的并列选项
>
> ⚠️ **兼容性兜底（IDE 不支持 multiSelect 时使用）**：
>
> 当某 IDE 的 `ask_followup_question` 确实不支持 `multiSelect: true` 参数时（实测发现 CodeBuddy 当前版本不支持），退化为「单选 + 完成修改」循环菜单——但**仍是一个完整的、独立于最终确认的步骤**：
>
> ```
> ask_followup_question({
>   question: "请选择要修改的项（每次选一个，修改完会回到本菜单；选「完成修改」结束修改阶段进入最终确认）",
>   options: [
>     "修改标题（当前：{title}）",
>     "修改摘要（当前：{summary}）",
>     "修改可见范围（当前：公开）",
>     "完成修改"
>   ]
> })
> ```
>
> **循环模式严格执行规则**：
> - 用户选 "修改标题" → 跳到 ③ 改完后**回到本菜单再问一次**（菜单中"当前值"已更新为新值）
> - 用户选 "修改摘要" → 跳到 ④ 改完后回到本菜单
> - 用户选 "修改可见范围" → 跳到 ② 改完后回到本菜单
> - 用户选 "完成修改" → **退出循环**，进入 ⑤ 最终确认
> - **绝对禁止**：在这个菜单里塞"确认上传"或"取消上传"选项——那是 ⑤ 最终确认的职责，本菜单只管"修改阶段"

##### ② 修改可见范围（仅在用户选了"修改可见范围"时执行）

> ⚠️ **只提供「公开 / 私密」两个选项**，不提供「团队可见」。如用户问起，告知："团队可见需在上传完成后到 ChatSpark 网页端 → 我的会话 → 编辑会话进行二次设置"。

```
【Claude Code / WorkBuddy】
AskUserQuestion({
  questions: [{
    header: "可见范围",
    question: "请选择会话可见范围（不支持团队可见）",
    multiSelect: false,
    options: [
      { label: "公开 (public)", description: "ChatSpark 上所有用户都可看到" },
      { label: "私密 (private)", description: "只有你自己可见" }
    ]
  }]
})

【Cursor / CodeBuddy / Box AI】
ask_followup_question({
  question: "请选择会话可见范围（不支持团队可见）",
  suggestions / options: ["公开", "私密"]
})
```

把用户选择映射为 `visibility` 字符串：`公开 → public`、`私密 → private`。

##### ③ 修改标题（仅在用户选了"修改标题"时执行；自由文本，**禁止**用 `ask_followup_question / AskUserQuestion` 加 options）

用纯话术展示当前值，等待用户在对话框输入新标题：

```
当前标题：{title}
请直接发送新的标题（不超过 20 字）。
```

- 用户回复任意文本 → 截断到 200 字符内作为新 `title`
- 用户发送空内容/纯空白 → 沿用当前 `title`（不报错）

##### ④ 修改摘要（仅在用户选了"修改摘要"时执行；规则同上）

```
当前摘要：{summary}
请直接发送新的摘要（不超过 200 字）。
```

- 用户回复任意文本 → 截断到 500 字符内作为新 `summary`
- 用户发送空内容/纯空白 → 沿用当前 `summary`

##### ⑤ 最终确认

把当前的三项（**包括未修改的**）汇总展示，再问一次：

```
即将上传：
📝 标题：{title}
📋 摘要：{summary}
🔒 可见范围：{public|private}

【点选】确认上传 / 重新修改 / 取消上传
```

- 「确认上传」→ 进入**第三步 ¾**（同步本地文件名），再进入第四步
- 「重新修改」→ 回到第三步半 ① 重新选要改哪些项
- 「取消上传」→ 走第五步 B（暂不上传）

#### 第三步 ¾：同步本地文件名（仅在用户修改了标题时执行）

**原则**：本地导出的 `.md` 文件名应当与最终标题保持一致，方便用户在文件管理器中识别。

执行条件：**仅当用户在第三步半 ② 修改了 `title`（即新值 ≠ 原 `chat-export-{时间戳}` 风格的默认标题）时执行**；如果用户在 ② 选择「保留」默认标题，**跳过本步**保留原文件名。

步骤：

1. 用 `title` 派生**安全文件名**（与 Edge Function `sanitizeFileName()` 一致，避免上下游差异）：

   ```python
   import re
   safe = re.sub(r'[^A-Za-z0-9_.\-一-鿿]', '_', title)   # 保留中英文/数字/._-
   safe = re.sub(r'^[._]+|[._]+$', '', safe)                      # 去首尾 ._
   safe = safe[:80] or 'session'                                  # 截断到 80
   ```

2. 拼接新路径：`new_md_path = {原目录}/{safe}.md`

3. **冲突处理**：若 `new_md_path` 已存在且不是当前文件，自动追加短时间戳后缀避免覆盖：
   ```
   {safe}-{HHMMSS}.md
   ```

4. 用 `mv {exported_md_file_path} {new_md_path}` 重命名（Python 用 `os.rename` 或 `shutil.move`），并更新后续步骤使用的 `exported_md_file_path = new_md_path`。

5. 失败兜底：重命名失败（权限、跨设备等）→ **不报错中断流程**，仅在终端打印一条 warning，沿用原文件名继续上传。

**示例**：
- 原文件：`/Users/me/Desktop/chat-export-20260601-103500.md`
- 用户改标题为：`这是一个test`
- 重命名为：`/Users/me/Desktop/这是一个test.md`

> ⚠️ 第四步往后所有引用 `{exported_md_file_path}` 的地方（包括上传脚本的 `--md-file`、最终展示的「📄 本地文件」）都使用**重命名后**的新路径。

### 第四步：用户同意后执行自动上传（线上中转 API）

**仅在第三步用户明确选择「立即上传」时进入本步。**

#### 第四步 A：自动上传前的 token 预检

调用 `upload_session_rpc.py` 之前，**先在本地探测 `chats_token` 是否已配置**（只判存在性，不读内容、不验证有效性），避免脚本因 token 缺失硬失败后才回退。

> ⚠️ **强约束（必读）**：下面这段探测代码的候选路径**必须**与 `upload_session_rpc.py` 中 `resolve_token_file()` 的逻辑**一一对应**——`$CHATS_TOKEN` → `$CHATS_HOME/chats_token` → `~/.chats/chats_token`，**仅此三条**。
> **严禁**在此列表之外私自添加其它探测路径（例如 `~/.claude/chats_token`、`~/.cursor/chats_token`、`~/.codebuddy/chats_token` 等 IDE 配置目录）。如果未来确需支持新位置，先去改脚本的 `resolve_token_file()`，再同步修改本文档；**绝不能只在预检处加路径**——否则预检 `HAS_TOKEN=1`，脚本却找不到文件，会出现"预检通过但上传报 token 不存在"的中间错误。

```bash
# 1) 环境变量 $CHATS_TOKEN 优先
if [ -n "$CHATS_TOKEN" ]; then
  HAS_TOKEN=1
  TOKEN_SOURCE=env_token
# 2) $CHATS_HOME/chats_token
elif [ -n "$CHATS_HOME" ] && [ -s "$CHATS_HOME/chats_token" ]; then
  HAS_TOKEN=1
  TOKEN_SOURCE=env_home
# 3) 默认 ~/.chats/chats_token
elif [ -s "$HOME/.chats/chats_token" ]; then
  HAS_TOKEN=1
  TOKEN_SOURCE=default
else
  HAS_TOKEN=0
  TOKEN_SOURCE=none
fi
echo "HAS_TOKEN=$HAS_TOKEN TOKEN_SOURCE=$TOKEN_SOURCE"
```

> `-s` 表示文件存在且非空。任一命中即视为"已配置"。  
> `TOKEN_SOURCE` 必须输出，第四步 C 会根据它决定是否需要透传 `CHATS_HOME` 给脚本。

- **`HAS_TOKEN=1`** → 直接进入第四步 C 执行上传脚本（按 `TOKEN_SOURCE` 决定是否透传环境变量）
- **`HAS_TOKEN=0`** → 进入第四步 B，让用户在"自动跳浏览器申请 / 转手动上传"之间显式选择

#### 第四步 B：未配置 token 时的二次询问（仅在 `HAS_TOKEN=0` 时执行）

**绝不直接静默打开浏览器，也不要硬退回手动上传**，而是把选择权交给用户。

> ⚠️ **必读：在调用下面任意一端的询问工具之前，先输出以下统一导语**，把两个关键 URL 显式呈现给用户，作为浏览器调起失败（远程开发 / SSH / 终端环境 / 权限拦截等）时的可复制兜底。**不要**把 URL 仅藏在按钮 label 里，也不要在没给出链接的情况下直接弹问。

```
未检测到 ChatSpark API Key，无法自动上传。
为防止后续浏览器调起失败，先把可手动访问的链接列在这里：

🔑 申请 API Key：<https://chats.woa.com/session/api-keys>
📤 手动上传页：  <https://chats.woa.com/session/upload>

若选择「打开浏览器申请 API Key」/「转为手动上传」时浏览器未自动打开，
请直接复制上方对应链接到浏览器访问。
```

输出导语后，再调用对应 IDE 的询问工具（**options 项数与语义保持不变**）：

**【Claude Code】** 调用 `AskUserQuestion`：

```
AskUserQuestion({
  questions: [{
    header: "缺少 API Key",
    question: "未检测到 ChatSpark API Key，无法自动上传。请选择处理方式：",
    multiSelect: false,
    options: [
      { label: "打开浏览器申请 API Key", description: "自动跳转到申请页，复制 Key 粘贴回来后继续自动上传" },
      { label: "转为手动上传", description: "直接打开 ChatSpark 上传页，自己在网页上传（无需配置 Key）" },
      { label: "取消", description: "仅保留本地文件，不做任何上传操作" }
    ]
  }]
})
```

**【Cursor】** 调用 `ask_followup_question`：

```
ask_followup_question({
  question: "未检测到 ChatSpark API Key，无法自动上传。请选择处理方式：",
  suggestions: ["打开浏览器申请 API Key", "转为手动上传", "取消"]
})
```

**【CodeBuddy】** 调用 `ask_followup_question`（不可用时退回文字交互）：

```
ask_followup_question({
  question: "未检测到 ChatSpark API Key，无法自动上传。请选择处理方式：",
  options: ["打开浏览器申请 API Key", "转为手动上传", "取消"]
})
```

**【WorkBuddy】** 调用 `AskUserQuestion`：

```
AskUserQuestion({
  questions: [{
    header: "缺少 API Key",
    question: "未检测到 ChatSpark API Key，无法自动上传。请选择处理方式：",
    multiSelect: false,
    options: [
      { label: "打开浏览器申请 API Key", description: "自动跳转到申请页，复制 Key 粘贴回来后继续自动上传" },
      { label: "转为手动上传", description: "直接打开 ChatSpark 上传页，自己在网页上传（无需配置 Key）" },
      { label: "取消", description: "仅保留本地文件，不做任何上传操作" }
    ]
  }]
})
```

**【Box AI】** 调用 `ask_followup_question`（与 CodeBuddy 一致）：

```
ask_followup_question({
  question: "未检测到 ChatSpark API Key，无法自动上传。请选择处理方式：",
  options: ["打开浏览器申请 API Key", "转为手动上传", "取消"]
})
```

路由：
- 「打开浏览器申请 API Key」→ 第四步 B.1（申请并配置后回到第四步 C）
- 「转为手动上传」→ 第五步 A（打开 `/session/upload`）
- 「取消」→ 第五步 B（仅展示本地路径，不打开任何浏览器）

##### 第四步 B.1：自动跳转申请 API Key 并本地配置

仅当用户在第四步 B 中点选「打开浏览器申请 API Key」时执行：

1. **打开 API Key 申请页**：

   ```bash
   # macOS
   open "https://chats.woa.com/session/api-keys"
   # Linux
   xdg-open "https://chats.woa.com/session/api-keys"
   # Windows
   start "" "https://chats.woa.com/session/api-keys"
   ```

2. **向用户说明操作步骤**（务必把 URL 明文列出，便于 `open` 失败时手动访问）：

   ```
   🌐 已尝试为你打开 API Key 申请页：<https://chats.woa.com/session/api-keys>
   若浏览器未自动打开，请手动复制上方链接到浏览器访问。

   请在网页上：
     1. 点击「生成」按钮，创建一个 Key
     2. 复制 Key 内容（形如 cb_sk_xxxxxxxxxxxx）

   复制完成后，请直接把 API Key 粘贴到下方对话框并发送给我即可，
   我会帮你写入本地配置（不会在对话里回显原文）。
   ```

3. **等待用户粘贴 API Key（纯话术，不调任何询问工具）**：

   - **不要**调用 `ask_followup_question` / `AskUserQuestion` 等带 options/suggestions 的工具——API Key 是自由文本，options 用不上，反而会绕一道交互。
   - 直接以上一步的话术作为提示，等待用户在对话框中输入并发送 API Key。
   - 收到用户消息后，把消息正文当作 `{USER_PASTED_API_KEY}` 用于第 4 步的写入。
   - **严禁**把 Key 原样复述、回显、或写入任何会被 git 跟踪的位置。

4. **拿到 API Key 后，写入默认路径**（权限收紧到 600）：

   ```bash
   mkdir -p "$HOME/.chats"
   printf '%s' "{USER_PASTED_API_KEY}" > "$HOME/.chats/chats_token"
   chmod 600 "$HOME/.chats/chats_token"
   ```

   > 严禁把 API Key 原样回显到对话中；不要写入项目目录或任何会被 git 跟踪的位置。

5. 写入成功后**回到第四步 C** 继续上传。

#### 第四步 C：调用上传脚本

**调用前必须按第四步 A 中的 `TOKEN_SOURCE` 决定是否透传环境变量**，不要"看到 `HAS_TOKEN=1` 就直接裸跑脚本"——否则 `TOKEN_SOURCE=env_home` 的场景下脚本会去默认路径 `~/.chats/chats_token` 找文件，命不中直接报 `❌ 配置错误: token 文件不存在`。

| `TOKEN_SOURCE` | 命中位置 | 调用脚本时的环境前缀 |
|---|---|---|
| `env_token` | `$CHATS_TOKEN` 环境变量 | 直接调用，无需额外前缀（脚本自身会读 `$CHATS_TOKEN`） |
| `env_home`  | `$CHATS_HOME/chats_token` | **必须**带上 `CHATS_HOME="$CHATS_HOME"` 前缀 |
| `default`   | `~/.chats/chats_token` | 直接调用，无需额外前缀 |

标准调用形态（按上表选择是否加前缀）：

```bash
# 通用骨架；env_home 场景请在 python3 前加 CHATS_HOME="$CHATS_HOME"
python3 "{SKILL_DIR}/scripts/upload_session_rpc.py" \
  --project-dir "{PROJECT_DIR}" \
  --md-file "{exported_md_file_path}" \
  --title "{title}" \
  --summary "{summary}" \
  --visibility "{visibility}"
# 标签由 AI 自动生成（总数 1-4 个，且至少包含一个平台预设标签）
```

`env_home` 场景的实际形态示例：

```bash
CHATS_HOME="$CHATS_HOME" python3 "{SKILL_DIR}/scripts/upload_session_rpc.py" \
  --project-dir "{PROJECT_DIR}" \
  --md-file "{exported_md_file_path}" \
  --title "{title}" \
  --summary "{summary}" \
  --visibility "{visibility}"
# 标签由 AI 自动生成（总数 1-4 个，且至少包含一个平台预设标签）
```

> `{visibility}` 取值由第三步 / 第三步半得出：未修改时填 `public`；用户在第三步半选「私密」时填 `private`。**不要传 `team`**，脚本只接受 `public` / `private` 两个值。

该脚本会自动：
1. 读取 `~/.chats/chats_token`（或 `CHATS_TOKEN` 环境变量，或 `$CHATS_HOME/chats_token`）
2. 优先调用线上中转 API（默认 `upload-session-relay`）完成：
   - `verify_api_key`
   - 上传完整 Markdown 到 Storage（`session-files`）
   - 调用 `upload_session_with_api_key` 仅写入短预览元数据
3. 若线上中转失败，自动回退到直连模式（此时才需要 `{PROJECT_DIR}/.supabase.json` 或 `SUPABASE_URL/SUPABASE_ANON_KEY`）
4. 校验关键请求返回 **HTTP 200** 且业务成功（`success=true`）

自动上传成功后，向用户展示：

```
✅ 对话已导出并自动上传成功！

📄 本地文件：{exported_md_file_path}
📊 消息统计：X 条用户消息，Y 条助手回复
📝 标题：{title}
🔒 可见范围：{public|private}
🆔 Session ID：{session_id}
```

> 🚫 **绝对禁止删除本地导出的 Markdown 文件**（无论上传成功还是失败）：
> - 本地 `.md` 文件**不是临时文件**，是用户的**永久产物**——可作为本地备份、转发素材、二次手动上传的源
> - **禁止**调用 `rm` / `os.remove` / `pathlib.Path.unlink` / `shutil.rmtree` 等任何删除该文件的操作
> - **禁止**输出"已清理 / 已删除 / cleanup / 临时文件已清理"之类的话术——这会误导用户以为文件已不存在
> - 上传成功的标志是返回 `session_id`，**不需要**做任何"收尾清理"动作；展示完上述成功信息就停止本轮，等待用户后续指令

**自动上传失败的回退**：若脚本返回非 0，按错误类型分别处理：

1. **`❌ 配置错误: token 文件不存在: ...`**（典型于 AI 在第四步 A 误判 `HAS_TOKEN=1`，或用户把 token 放在了 IDE 自己的目录里）：
   - **不要**立刻回退第五步 A。先做一次"二次探测"，扫一遍常见的 IDE 配置目录（**仅用于诊断和询问，不擅自使用**）：

     ```bash
     for d in "$HOME/.claude" "$HOME/.cursor" "$HOME/.codebuddy"; do
       if [ -s "$d/chats_token" ]; then
         echo "FOUND_TOKEN_AT=$d/chats_token"
       fi
     done
     ```

   - 若命中某个目录（设为 `$FOUND_DIR`），通过 `ask_followup_question` / `AskUserQuestion` 询问用户：「检测到 token 在 `$FOUND_DIR/chats_token`（非默认位置），是否用它重试上传？」，选项：`["用该位置重试", "转为手动上传"]`。
   - 用户同意 → 用 `CHATS_HOME="$FOUND_DIR" python3 ...` 重跑一次脚本；仍失败再走第五步 A。
   - 用户拒绝或二次探测未命中 → 直接走第五步 A。
2. **SSL 证书验证失败**（脚本输出含 `ssl_error: true` 或 response 含 `CERTIFICATE_VERIFY_FAILED` / `SSL_ERROR`）：
   - 告知用户：脚本已自动尝试系统 CA bundle 重试，仍失败说明当前机器的常见 CA bundle 路径（`/etc/ssl/certs/ca-certificates.crt` 等）均无法验证 ChatSpark 证书。
   - 建议用户提供企业根证书路径后设置以下环境变量重试（**只用 `SSL_CERT_FILE`，`REQUESTS_CA_BUNDLE` 对本脚本无效**）：
     ```bash
     export SSL_CERT_FILE=/path/to/company-ca.crt
     ```
   - 通过 `ask_followup_question` / `AskUserQuestion` 询问：「请提供企业 CA 证书路径，设置后可重试；或转为手动上传」，选项 `["设置好了，重试上传", "转为手动上传"]`。
   - 用户重试 → 重跑脚本；仍失败 → 走第五步 A。
   - 用户选手动 → 走第五步 A。
3. **其它错误**（token 无效、网络异常、接口非 200 等）：
   - **不要**重复打开 `/session/api-keys`（即便刚通过第四步 B.1 写过 token），直接回退到第五步 A（手动上传页），并附带脚本返回的错误摘要，提示用户检查 API Key 是否正确。

### 第五步：手动上传 / 暂不上传 分支

根据用户的点选或自动上传失败的情况，走以下两个子分支之一：

#### 第五步 A：手动上传（打开浏览器上传页）

触发条件：
- 第四步 B 用户点选「转为手动上传」
- 第四步 C 自动上传脚本返回非 0（token 无效、网络异常、接口非 200 等），自动回退到此分支

向用户展示（若是自动上传失败回退，附上脚本返回的错误摘要）：

```
ℹ️ 已保留本地导出文件，将尝试为你打开浏览器手动上传。

📄 本地文件：{exported_md_file_path}
👉 上传页面：<https://chats.woa.com/session/upload>

若浏览器未自动打开，请直接复制上方链接到浏览器访问。
```

并尝试自动打开上传页面（仅作为便捷兜底，不阻塞——`open`/`xdg-open`/`start` 在远程开发、SSH、终端环境下可能失败，此时请用户手动复制上面的 URL）：

```bash
# macOS
open "https://chats.woa.com/session/upload"
# Linux
xdg-open "https://chats.woa.com/session/upload"
# Windows
start "" "https://chats.woa.com/session/upload"
```

#### 第五步 B：暂不上传（仅保留本地）

触发条件：
- 第三步用户点选「暂不上传」
- 第四步 B 用户点选「取消」

**不要打开任何浏览器页面**，仅向用户展示本地文件路径：

```
ℹ️ 已保留本地导出文件，未做任何上传操作。

📄 本地文件：{exported_md_file_path}
👉 需要时可稍后手动上传：<https://chats.woa.com/session/upload>
```

> **注意**：
> - macOS 使用 `open` 命令，Linux 使用 `xdg-open`，Windows 使用 `start`
> - Skill Hub 使用 Next.js 路由，上传页路径为 `/session/upload`
> - 自动上传依赖本地 token（默认 `~/.chats/chats_token`，可用 `CHATS_HOME` 自定义目录）
> - 支持通过 `CHATS_UPLOAD_RELAY_URL` 覆盖默认中转地址（推荐配置为你方后端 `/api/chats/upload`）

### 错误处理

- 如果 Python 不可用 → 直接告诉用户本地文件路径，并以明文形式给出上传页 URL `<https://chats.woa.com/session/upload>`（不要假设浏览器一定能被调起）
- 如果 `read_chat_history.py` 找不到对话存档 → 回退到 AI 上下文记忆方式（手动整理 Markdown 并保存）
- 若用户在第三步点选「暂不上传」→ 不执行上传脚本、**不打开任何浏览器页面**，仅展示本地路径（第五步 B）
- 若第四步 A 检测到未配置 token → **不静默跳浏览器**，必须先经第四步 B 让用户在「申请 Key」/「转手动」/「取消」之间显式选择，且**导语中必须显式列出申请页 / 上传页 URL** 作为浏览器调起失败的兜底
- 若用户在第四步 B 点选「转为手动上传」→ 跳过上传脚本，直接打开上传页（第五步 A）
- 若用户在第四步 B 点选「取消」→ 走第五步 B，不打开任何浏览器
- 若用户在第四步 B.1 第 3 步未粘贴 key（直接发了空消息或无关内容）→ 重复一次纯话术提示「请把刚申请到的 API Key 直接粘贴到对话框发送」，**不要**改用 `ask_followup_question` 加 options
- 若用户在第四步 B.1 粘贴的 API Key 在第四步 C 中仍上传失败 → **不要**重复打开 `/session/api-keys`，转走第五步 A 并提示「API Key 可能无效，请检查后重试」
- 若第四步 C 脚本回 `❌ 配置错误: token 文件不存在: ...` → 先做"二次探测"（扫 `~/.claude` / `~/.cursor` / `~/.codebuddy` 下的 `chats_token`），命中则询问用户是否带 `CHATS_HOME=...` 重试；用户拒绝或未命中再回退第五步 A
- 如果自动上传脚本返回其它非 0 错误 → 展示错误摘要并回退到第五步 A（手动上传页）

## 本地存档读取原理

脚本采用**「AI 主动探测 → CLI 显式传入 → 环境变量 → 内置候选兜底」**的多级路径解析，确保在任意安装位置都能找到对话存档。

### 路径解析优先级（从高到低）

1. **CLI 显式传入**（第零步探测结果传入，推荐）
   - `--data-root <path>` —— CodeBuddy 系数据根，可重复传入多个
   - `--claudecode-projects-dir <path>` —— Claude Code projects 根，可重复传入多个
   - `--cursor-projects-dir <path>` —— Cursor projects 根，可重复传入多个
2. **环境变量**
   - `CODEBUDDY_DATA_ROOT`（多个用 `:` 分隔）
   - `CLAUDECODE_PROJECTS_DIR`（多个用 `:` 分隔）
3. **内置候选**（兜底，覆盖常见默认安装位置；如果第零步的探测已经覆盖到，这一级通常不会被触发）

> 脚本会按顺序遍历所有候选路径，自动跳过不存在的目录，命中即用。因此**传入多余或错误的路径不会导致失败**，只是多一次跳过。

在每个数据目录下，脚本会自动尝试多种目录层级结构：

```
{数据目录}/
  └── {userId 或 default}/CodeBuddyIDE/
      ├── {userId}/history/         # 结构1：macOS（userId 重复）
      ├── history/                  # 结构2：default 目录（无子层）
      └── {sessionId}/history/      # 结构3：Linux（sessionId ≠ userId）
          └── {workspaceHash}/           # workspaceHash = MD5(工作区绝对路径)
              └── {conversationId}/
                  ├── index.json         # 消息顺序索引 + 请求记录
                  └── messages/          # 每条消息一个 JSON 文件
                      └── {messageId}.json
```

**消息文件结构**：
- `role`: user / assistant / tool
- `message`: 嵌套 JSON，包含 `content` 数组（text / tool_use 块）
- `extra`: 包含 `sourceContentBlocks`（用户原始输入）、`requestId`、`modelId` 等元数据

**脚本智能提取逻辑**：
1. 用户消息 → 优先从 `extra.sourceContentBlocks` 获取干净的原始输入
2. 助手消息 → 提取 `content` 中 `type=text` 的文本块作为正文；同时单独抽取 `type=thinking/reasoning/redacted_thinking` 块作为模型思考；Box AI 直接消费 `messages.reasoning_content` 列
3. 跳过 `role=tool` 的工具响应消息
4. 按 `index.json` 顺序还原消息时序

**输出格式特性（v2.x+）**：

assistant 消息按以下结构输出，**前端契约清晰**：

```markdown
## Assistant <!-- MODEL:模型名 -->

<details>                               ← 思考过程（可选，仅当存档含 thinking/reasoning 时存在）
<summary></summary>
[模型思考全文]
</details>

<!-- TIMELINE_START -->                 ← 工具调用 + 过程叙述时间线（按原始时序穿插）
<!-- NARRATION:|让我先看看入口文件…| -->     ← 第一次工具调用之前的开场叙述
<!-- TOOL:read_file|📄 读取文件|main.py|ok|{结果预览JSON} -->
<!-- TOOL:list_dir|📂 浏览目录|src|ok|... -->
<!-- NARRATION:|找到了，接下来…| -->          ← 工具调用之间的过程文字
<!-- TOOL:search_content|🔍 搜索内容|main|ok|... -->
<!-- TIMELINE_END -->

最终结论文字 / 代码块 / 列表（普通 markdown 段落，留在 timeline 外）
```

- **模型思考（thinking / reasoning）**：原始数据里 thinking 块永远在 assistant 消息开头单次出现，**不与工具调用穿插**（已查证 Anthropic 协议契约）。因此独立为 `<details><summary></summary>...</details>` 块放在 timeline 之前，summary 留空让前端 CSS 接管标题。
- **过程叙述（NARRATION）**：助手在工具调用之前（含第一次工具调用之前）的所有文字段一律视为过程叙述，进入 timeline，与工具调用按 `position` 字段所示的原始时序穿插。叙述文本经 `\n` 编码（前端解码时还原为换行），用 `<!-- NARRATION:|...| -->` 单行注释承载。
- **工具调用（TOOL）**：进入 timeline，按 `position`（在原 content blocks 中前置非空 text 块的数量）穿插在 NARRATION 行之间。每行 `<!-- TOOL:name|displayName|argsSummary|ok|error|resultPreview -->`，状态为 `ok` 或 `error`。
- **最终结论**：**最后一个工具调用之后**的所有文字段，留在 timeline 外作为正常 markdown 段落输出（含代码块、列表、表格等）。切分规则纯确定性：text 段索引 `i <= max(tool_call.position)` 即 narration，反之即正文；无工具调用时整段都是正文，不生成 timeline 容器。
- **退化路径**：若存档里 tool_calls 缺失 position 信息（如 Box AI 等），所有工具调用按原顺序排在 timeline 最前，所有文字段视为正文。
- **前端集成**：识别 `<!-- TIMELINE_START --> ... <!-- TIMELINE_END -->` 这对标记，把内部所有 `<!-- TOOL: -->` 和 `<!-- NARRATION: -->` 行解析成时间线条目，渲染成可折叠组件；timeline 外的内容按普通 markdown 渲染。HTML 注释在所有 markdown 渲染器中默认不可见，因此**前端不实现 timeline 也只会看到"最终结论"部分，不会出现错乱标签**——这是安全的退化。

## 注意事项

- **🚫 本地导出文件是用户永久产物，禁止删除**：上传成功后**绝对不允许**对 `{exported_md_file_path}` 调用任何删除操作（`rm` / `os.remove` / `unlink` / `shutil.rmtree` 等）。本地 `.md` 是用户的备份与转发素材，不是临时文件。也**禁止**输出"已清理临时文件 / 已删除 / cleanup"之类的话术。
- **完整性保证**：通过本地文件读取，不受 AI 上下文窗口限制，可导出数百条消息的完整对话
- **敏感信息自动脱敏**：导出时默认对高置信度敏感信息（API Token、密钥、Auth Header 等）自动脱敏，保留前缀以便识别类型（如 `cb_sk_***REDACTED***`）。使用 `--no-redact` 可关闭脱敏
- **思考过程与过程叙述折叠**：导出的 Markdown 会把模型思考、工具调用之间的过渡叙述用 `<details>` 折叠包裹，渲染时默认收起，避免长会话出现"流水账"段落；展开后原文完整保留
- 导出时会自动过滤掉工具调用的技术细节（如 read_file、search 等），只保留核心对话内容
- 代码块会完整保留
- 图片引用会保留为 Markdown 链接格式
- **自动上传需要 API Key**：将 token 保存到 `~/.chats/chats_token`（或通过 `CHATS_TOKEN` 环境变量传入）；若用户选择立即上传但未配置 token，第四步会再次询问用户「打开浏览器申请 API Key」/「转为手动上传」/「取消」，不会静默跳浏览器
- **浏览器调起未必成功**：在 SSH、远程开发、终端环境或权限受限场景下，`open` / `xdg-open` / `start` 可能静默失败。所有可能调起浏览器的步骤（第四步 B 导语、第四步 B.1 第 2 步、第五步 A/B）都**必须**以明文形式显式给出 URL（推荐 Markdown autolink `<https://...>`），让用户随时可手动复制访问
- **粘贴 API Key 用纯话术**：第四步 B.1 第 3 步是自由文本输入场景，**禁止**调用 `ask_followup_question` / `AskUserQuestion` 等带 options/suggestions 的工具，直接以话术等待用户在对话框中粘贴并发送即可；且不要把 Key 原样回显

## 常见反模式与故障排查

### 🐛 症状：导出的 Markdown 中工具调用只显示「🔧 调用了工具」，没有工具名/参数/结果

**根因**：AI 没有调用 `read_chat_history.py` 脚本，而是自行编写了内联 Python 代码（如 `python3 -c "..."`）做简化导出。

**诊断**：检查导出的 `.md` 文件：
- ✅ 正确输出应包含 `<!-- TOOL:execute_command|⚡ 执行命令|...|ok|... -->` 格式的 HTML 注释
- ❌ 错误输出则是 `> 🔧 调用了工具` 这样的 Markdown blockquote，且所有工具调用内容完全一致

**修复**：重新执行本 skill，确保第一步使用的是：
```bash
python3 {SKILL_DIR}/scripts/read_chat_history.py --source boxai --conversation-id ... --workspace-dir ...
```
而不是任何形式的 `python3 -c "..."` 或 `python3 << 'EOF'`。

### 🐛 症状：AI 在执行第一步前花了大量时间「探索数据库结构」

**典型行为**：
- 先跑 `sqlite3 ... ".schema"` 看表结构
- 再跑 `sqlite3 ... "SELECT * FROM messages LIMIT 1"` 看数据格式
- 然后说「好的，我了解了数据结构」接着开始写 `python3 -c "..."`

**根因**：AI 陷入了「先理解再行动」的模式，但 Box AI 的 SQLite 结构对脚本是透明的——
脚本不需要 AI 预先理解数据结构，它自己会处理。

**正确做法**：第零步探测到 `sessions.db` 存在后，**直接跳到第一步运行脚本**。
不需要查看表结构、不需要预览数据、不需要验证格式——脚本会做所有这些。

### 🐛 症状：脚本执行报错后 AI 尝试用内联代码「补救」

**典型行为**：`read_chat_history.py` 返回非 0 → AI 说「脚本失败了，让我直接用 Python 读数据库导出」

**根因**：AI 把「脚本报错」理解为「需要用其他方式完成导出」，但脚本报错的正确语义是「参数或环境有问题，需要修正」。

**正确做法**：阅读 stderr 错误信息 → 按本文档「错误处理」章节指引操作 → 修正参数重试。
绝不把脚本失败当作「授权自行编写替代方案」的信号。
