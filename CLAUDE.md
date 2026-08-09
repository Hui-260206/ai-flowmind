# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

注意：
- 为了学习开发，开发需求时先给建议、教我设计可维护、符合现代软件工程思想的架构。
- 尽量让我手动改代码；未明确要求前不要直接修改。

## 项目概述

`flow-mind` 由两部分组成：

- **客户端**（`mobile/`）：基于 **Kuikly**（腾讯 Kotlin Multiplatform 框架），UI 在 `mobile/shared` 用 Kotlin / Kuikly Compose 编写一次，在 **Android / iOS** 原生渲染。**HarmonyOS（OHOS）脚手架保留但不维护、不在构建范围内。**
- **后端**（`services/`）：**Go API 服务 + Python AI 服务** 双服务架构，目前均为空目录、待搭建。
  - `go-api/`：Go 实现的 REST/gRPC 服务，移动端唯一对外 API，负责会话/消息业务、匿名设备隔离、MySQL 持久化、上下文控制、Redis（幂等/锁/限流）、调用 Python gRPC。
  - `ai-service/`：Python + FastAPI 的 AI 服务，负责模型调用、Provider 适配、参数校验、AI 错误转换，经 gRPC 被 Go 调用；模型密钥仅此侧保存、不对公网暴露。
  - 依赖 **MySQL**（事实来源）、**Redis**（幂等/锁/限流）；MVP 不引入消息队列，RabbitMQ 仅作为未来异步扩展选项。
  - 完整设计见 `docs/phase-0/MVP_REQUIREMENTS.md`、`docs/roadmap/MVP_EXECUTION_PLAN.md` 和 [`docs/README.md`](docs/README.md)。

Kotlin 包名 `com.heli.flowmind`；Kuikly 运行时 `2.7.0-2.1.21`，定义在 `mobile/buildSrc/.../KotlinBuildVar.kt`（`Version`/`BuildPlugin`），改版本只改这里。

## 跨平台流水线

- `mobile/shared` 是唯一的 KMP 模块，代码全在 `src/commonMain`（无平台特定 source set）；目标为 `androidTarget`、`ios*`、`js(IR)`。
- **页面** = `@Page("page_name")` 注解、继承 `base.BasePage` 的类，在 `willInit()` 调 `setContent { ... }`。默认页面 `flowmind_session_page`（`page/SessionPage.kt`）。
- **KSP** 处理 `@Page` 生成页面注册表；`-PpageName=<name>` 可分包构建单一页面（`shared/build.gradle.kts` 的 `getPageName()`）。
- 各端壳工程嵌入 Kuikly 渲染视图并指向页面名：Android `KuiklyRenderActivity`、iOS `KuiklyRenderViewPage(pageName:data:)`（见 `ContentView.swift`）。

## 原生桥（BridgeModule）

`base/BridgeModule.kt`（`HRBridgeModule`，`MODULE_NAME="HRBridgeModule"`）是 common→native 桥，经 `callNativeMethod`（异步）/ `syncCallNativeMethod`（同步）调用原生能力（日志、toast、开关页面、SSO、缓存、离线包、上报等）。各端实现对应方法：
- Android：`androidApp/.../module/KRBridgeModule.kt`（在 `KuiklyRenderActivity.registerExternalModule` 注册）。
- iOS：iOS 侧原生模块。

非 Composable 代码调用时用 `base/IPagerIdKtx.kt` 的 `IPagerId.bridgeModule`；`base/Utils.kt` 另提供 `currentBridgeModule()` / `logToNative(content)`。图片/日志/字体/路由/线程/崩溃等适配器在 Android `KuiklyRenderActivity.initKuiklyAdapter()` 注册。

## 构建 / 运行（均在 `mobile/` 用 `./gradlew`；Gradle 8.5 / AGP 7.4.2 / Kotlin 2.1.21）

```sh
./gradlew :androidApp:assembleDebug          # Android debug APK
./gradlew :androidApp:installDebug            # 安装到设备
./gradlew :shared:generateDummyFramework      # iOS 首次：生成占位框架后 pod install 才能成功
cd iosApp && pod install && cd ..             # Xcode 打开 iosApp/iosApp.xcworkspace
./gradlew :shared:jsBrowserProductionWebpack                      # 全量 JS 产物
./gradlew :shared:jsBrowserProductionWebpack -PpageName=flowmind_session_page  # 分包
cd mobile && npm run serve                    # 本地热更新：:8017 静态服务 + :8083 whistle
```

调试经 HTTP 拉 JS（`nv_js`）与 `.so`（`nv_so`）；代理规则 `mobile/.whistle.js` 转发到 `127.0.0.1:8017`，静态路径见 `static_server/serve/config/serve.conf.js`。

## 后端（services/）要点

```text
Kuikly 移动端 ──HTTPS JSON──▶ Go API ──gRPC──▶ Python AI ──▶ 大模型 Provider
                  ├ MySQL(事实来源) └ Redis(幂等/锁/限流)
```

- Go API 是移动端唯一业务 API；Python AI 不暴露公网、不持会话所有权与业务库。
- MySQL 是聊天记录唯一事实来源，Redis/进程内存不替代它；MVP 普通聊天采用同步 HTTP + gRPC，不依赖消息队列。
- 对外 REST（MVP）：`GET /healthz`、`/readyz`；`POST/GET /api/v1/sessions`；`GET/POST/DELETE /api/v1/sessions/{id}/messages`。业务请求带 `X-Client-ID`（匿名 UUID）与 `X-Request-ID`；错误格式见需求文档第 8 节。
- 当前 `go-api`/`ai-service` 为空：先定接口/边界（proto、OpenAPI、数据模型、Redis Key），再做 Go 工程、数据层、聊天 API，接 Python gRPC，补 Redis 能力，最后联调达标再接入 `data/RemoteChatRepository.kt`（阶段 9）。Outbox/MQ 不属于 MVP 主链路，未来按需单独引入。

## 测试

```sh
./gradlew :shared:allTests   # 所有 KMP 目标
./gradlew :shared:test        # 仅 JVM（commonTest），使用 kotlin("test")
```

`commonTest` 是唯一测试 source set。

## 编辑代码架构要点

- Compose 原语来自 `com.tencent.kuikly.compose.*`（非 `androidx.compose.*`）。`base/BasicWidget.kt` 已封装 `TextField`/`Button`/`Modal`/`margin`/`borderRadius` 等，优先复用；`padding`/`margin`/`height`/`width` 有 `Float` 重载（内部 `.dp`）。
- 状态：`state/SessionViewModel.kt` 是纯 Compose state 持有者（`mutableStateOf`/`mutableStateListOf`），经 `remember` 接入 `SessionScreen`；输入栏 `value`/`onValueChange`/`onSend` 走此 ViewModel，新增状态优先扩展它。
- 分层：`page` → `component` → `base`/`state`/`model`/`data`；模型在 `model/`（`Message`/`MessageRole`/`MessageStatus`），组件在 `component/`，页面在 `page/`。
- 继承 `BasePage`（非直接 `ComposeContainer`）以保留深色模式（`isNightMode`）与关闭调试检查器等行为。
- iOS 部署目标 14.1；`shared` 以静态框架发布（`isStatic = true`），资源来自 `src/commonMain/assets/**`。

## 仓库注意事项

- `settings.gradle.kts` 含不存在的 `:h5App`/`:miniApp`，Gradle sync 报错时注释掉或创建对应模块。
- 解析 Kuikly 产物需访问腾讯 Maven 镜像（`mirrors.tencent.com/.../maven-tencent/`）。
- `*.js`/`*.so` 及 `static/` 被 gitignore，构建产物不入库。
- OHOS Gradle 体系（`settings.ohos.gradle.kts` 等）保留但不维护、无需运行。
- 禁止提交真实密码与 API Key。
