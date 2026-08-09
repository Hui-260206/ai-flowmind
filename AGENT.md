# AGENT.md

本文件为在本仓库工作的 AI 编程助手（Agent）提供操作指南。详细的架构说明见 [`CLAUDE.md`](./CLAUDE.md)，本文档侧重 **Agent 的工作方式** 与 **强制流程约束**。

## 1. 工作方式（强制）

- **以教学为目的**：用户正在学习开发。实现需求时，先解释设计思路、给出符合现代软件工程思想的可维护架构，再动手。
- **优先让用户手动修改**：除非用户明确要求你改代码，否则 **不要直接修改源文件**。探索、提案、示例代码优先放在对话里，而不是写进文件。
- **改动要最小且定向**：确需改代码时，用 `replace_in_file` 做局部修改，避免重写大文件。

## 2. OpenSpec + Superpowers 工作流（强制）

检测到以下命令前缀时，按对应阶段执行（严格禁止凭记忆跳步）：

| 命令 | 阶段 |
|------|------|
| `opsx:propose` / `openspec:proposal` | Phase 1（头脑风暴）+ 1.5（worktree 校验）+ 2（写提案） |
| `opsx:apply` / `openspec:apply` | Phase 3（执行） |

核心约束（违反立即停止并说明）：

- **Phase 1**：禁止创建文件、禁止跳过 brainstorming、禁止未经确认进入下阶段。
- **Phase 1.5**：必须验证 worktree 环境，严格按 1 → 1.5 → 2 顺序。
- **Phase 2**：必须在 `openspec/changes/` 下创建提案，禁止使用 `writing-plans`、禁止凭记忆生成文档。
- **Phase 3**：必须停等用户选择 `execution_mode`、必须按步骤执行、每个任务后必须做代码审查 + 验证。

项目配置（`execution_mode: agent` / `review_mode: per_task` / `review_action: confirm`，详见用户规则）。

## 3. 项目概览

`flow-mind` 是一个面向移动端的 AI 对话应用，分两部分：

- **客户端** `mobile/`：基于 **Kuikly**（腾讯 Kotlin Multiplatform 框架）。UI 与业务逻辑全部写在 `mobile/shared`（`src/commonMain`）里，用 Kotlin / Kuikly Compose 一次编写，在 **Android 与 iOS** 原生渲染。**HarmonyOS（OHOS）脚手架保留但不维护、不在构建范围内。**
- **服务端** `services/`：按 MVP 设计为 **Go API 服务 + Python AI 服务**，内部通过 gRPC 通信，外部对移动端暴露唯一 REST API。依赖 MySQL（事实来源）和 Redis（幂等/锁/限流）；MVP 不引入消息队列。开发阶段 MySQL/Redis 运行在 Mac 本机，后续部署再迁移到 Docker。

> 注意：`CLAUDE.md` 中"后端为 Python + FastAPI"是早期设想；当前 MVP 已演进为 Go + Python 双服务架构，以 [`docs/phase-0/MVP_REQUIREMENTS.md`](docs/phase-0/MVP_REQUIREMENTS.md) 和 [`docs/roadmap/MVP_EXECUTION_PLAN.md`](docs/roadmap/MVP_EXECUTION_PLAN.md) 为准。

Kotlin 包名：`com.heli.flowmind`。Kuikly 运行时版本 `2.7.0-2.1.21`（定义在 `mobile/buildSrc/.../KotlinBuildVar.kt` 的 `Version`/`BuildPlugin`，改版本只改这里）。

## 4. 移动端架构要点（编辑代码时）

- Compose 原语来自 `com.tencent.kuikly.compose.*`，**不是** `androidx.compose.*`。`base/BasicWidget.kt` 已封装常用辅助（`TextField`、`Button`、`Modal` 等），优先复用。
- 页面 = 带 `@Page("page_name")` 注解、继承 `base.BasePage` 的类，在 `willInit()` 里 `setContent { ... }`。默认页面为 `flowmind_session_page`（`page/SessionPage.kt`）。
- 状态管理：`state/SessionViewModel.kt` 是纯 Compose state 持有者（`mutableStateOf`/`mutableStateListOf`），通过 `remember` 接入 `SessionScreen`。
- 分层：`page` → `component` → `base`/`state`/`model`/`data`。模型在 `model/`，组件在 `component/`。
- 原生桥：`base/BridgeModule.kt`（`HRBridgeModule`）是 common→native 桥，经 `callNativeMethod`/`syncCallNativeMethod` 调用原生能力。各端壳工程嵌入 Kuikly 渲染视图并指向页面名。
- 继承 `BasePage` 而非 `ComposeContainer`，以保留深色模式与关闭调试检查器等行为。
- iOS 部署目标 14.1；`shared` 以静态框架发布；资源来自 `src/commonMain/assets/**`。

## 5. 常用命令（均在 `mobile/` 下用 `./gradlew`）

```sh
# Android
./gradlew :androidApp:assembleDebug
./gradlew :androidApp:installDebug

# iOS（CocoaPods）
./gradlew :shared:generateDummyFramework
cd iosApp && pod install && cd ..   # 用 Xcode 打开 iosApp/iosApp.xcworkspace

# 单页 JS 产物（分包：-PpageName=<name>）
./gradlew :shared:jsBrowserProductionWebpack
./gradlew :shared:jsBrowserProductionWebpack -PpageName=flowmind_session_page

# 测试
./gradlew :shared:allTests        # 所有 KMP 目标
./gradlew :shared:test            # 仅 JVM（commonTest）

# 本地热更新：静态服务器 + whistle
cd mobile && npm run serve        # :8017 静态服务，:8083 whistle
```

## 6. 仓库注意事项

- `settings.gradle.kts` include 了不存在的 `:h5App`/`:miniApp`，Gradle sync 报错时注释掉或创建对应模块。
- 解析 Kuikly 产物需访问腾讯 Maven 镜像（`mirrors.tencent.com/.../maven-tencent/`），确保网络可达。
- `*.js`/`*.so` 及 `static/` 被 gitignore，构建产物不入库。
- OHOS 脚手架（`settings.ohos.gradle.kts` 等）保留但不维护，无需运行。
- 禁止提交真实密码、API Key 等密钥。

补充说明（非常重要）：
我做这个项目是为了学习企业级的AI全栈开发。因此，每个需求（或者阶段任务）拆分成多个足够小的阶段。
每个小阶段，都需要停下来，都要告诉我为什么这样实现，我也需要review代码。
确保新手的我能绝对和完全掌控这个项目。
