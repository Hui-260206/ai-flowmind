# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

`flow-mind` 由两部分组成：

- **客户端**（`mobile/`）：基于 **Kuikly**（腾讯的 Kotlin Multiplatform 跨平台框架）开发，UI 在
  `mobile/shared` 模块中用 Kotlin / Kuikly Compose 编写一次，在 **Android 与 iOS** 上原生渲染。
  **HarmonyOS（OHOS）暂不实现**——仓库里虽保留了 `ohosApp/` 及相关 `.ohos.gradle.kts`、`runOhosApp.sh` 等脚手架，
  但当前不维护、不在构建范围内，改动客户端时无需顾及。
- **后端**（`backend/`）：采用 **Python + FastAPI**（目前为空目录，待搭建）。

Kotlin 包名：`com.heli.flowmind`。Kuikly 运行时版本为 `2.7.0-2.1.21`，定义在
`mobile/buildSrc/src/main/java/KotlinBuildVar.kt`（`Version` / `BuildPlugin` 对象）——如需改版本只改这里，
不要去各个 `build.gradle.kts` 里改。

## 跨平台流水线如何运转

- `mobile/shared` 是唯一的 KMP 模块，也是 UI / 业务逻辑唯一的存放处。它面向
  `androidTarget`、`iosX64/iosArm64/iosSimulatorArm64`、`js(IR)`（browser）三个目标。所有真正的代码都在
  `src/commonMain` 下——没有任何带逻辑的平台特定 source set。
- **页面（page）** 是一个用 `@Page("page_name", ...)` 注解、继承 `base.BasePage`
  （继承自 Kuikly 的 `ComposeContainer`）的类。页面在 `willInit()` 中调用 `setContent { ... }` 来承载其 Compose UI。
  参见 `page/SessionPage.kt`——它的页面名 `flowmind_session_page` 是各端壳工程默认加载的页面。
- **KSP** 会处理 `@Page` 注解（通过 `core-ksp` 产物）并生成页面注册表。Gradle 属性 `-PpageName=<name>`
  可将一次 JS 构建限定到指定页面（"分包构建"）；不传则构建全部页面。详见 `shared/build.gradle.kts` 中的
  `getPageName()` 与 `ksp { arg(...) }`。
- 各端壳工程都嵌入一个 Kuikly 渲染视图，并指向某个页面名：
  - **Android** `androidApp`：`KuiklyRenderActivity` 承载 `KuiklyRenderViewBaseDelegator`，默认加载 `flowmind_session_page`。
  - **iOS** `iosApp`：SwiftUI 中的 `KuiklyRenderViewPage(pageName:data:)`（见 `ContentView.swift`）。
  - ~~HarmonyOS `ohosApp`~~：暂不实现，参见"项目概述"。

## 原生桥（Kuikly Module 模式）

`base/BridgeModule.kt`（`MODULE_NAME = "HRBridgeModule"`）是 common→native 的桥。它通过
`callNativeMethod`（异步）和 `syncCallNativeMethod`（同步）调用原生方法——这些是对原生能力的*声明*
（日志、toast、开关页面、SSO 请求、缓存读写、离线包更新、上报等）。各端实现对应方法：
- Android：`androidApp/.../module/KRBridgeModule.kt`（在 `KuiklyRenderActivity.registerExternalModule` 中注册）。
- iOS：iOS 侧的原生模块。

`BasePage.createExternalModules()` 会为每个页面注册 `BridgeModule`。在非 Composable 代码里调用它时，
用 `base/IPagerIdKtx.kt` 中的 `IPagerId.bridgeModule` 扩展（底层是 `Utils.bridgeModule(pagerId)`）。
`base/Utils.kt` 还提供 `currentBridgeModule()` / `logToNative(content)`。

各类适配器（图片加载、日志、字体、颜色解析、路由、线程、崩溃处理）在 Android 的
`KuiklyRenderActivity.initKuiklyAdapter()` 中注册。（OHOS 的 `ohosApp/.../kuikly/adapter/` 随鸿蒙端一并搁置。）

## 常用构建 / 运行命令

所有 Gradle 命令都在 `mobile/` 目录下用 wrapper 执行（`./gradlew`）。
Gradle 8.5，AGP 7.4.2，Kotlin 2.1.21。

```sh
# Android debug APK
./gradlew :androidApp:assembleDebug
# 通过 Android Studio 在连接的设备上运行/安装，或：
./gradlew :androidApp:installDebug

# iOS —— shared 框架通过 CocoaPods 构建。首次准备：
./gradlew :shared:generateDummyFramework      # 生成占位框架，pod install 才能成功
cd iosApp && pod install && cd ..             # 之后用 Xcode 打开 iosApp/iosApp.xcworkspace
# 后续 shared 改动通过 pod 的 "Build shared" 脚本阶段重新构建框架。

# 单页面 JS 产物（输出：shared/build/js/packages/.../nativevue2.js，文件名由 webpackTask 指定）
./gradlew :shared:jsBrowserProductionWebpack
# 分包构建 —— 只构建指定页面：
./gradlew :shared:jsBrowserProductionWebpack -PpageName=flowmind_session_page
```

### 本地开发 / 热更新：静态服务器 + whistle

Kuikly 调试时通过 HTTP 拉取 JS（`nv_js`）和原生 `.so`（`nv_so`）。仓库自带一个 Koa 服务器，
负责托管 bundle 并启动 whistle 代理：

```sh
cd mobile && npm run serve        # 在 :8017 提供静态服务，在 :8083 启动 whistle（UI 在 :8017）
```

代理规则在 `mobile/.whistle.js` 中，把 `.../debug/nv_js/...` 和 `.../debug/nv_so/...`
转发到 `127.0.0.1:8017`。把 debug 包指向这个 host，即可在不全量重编译的情况下迭代 JS bundle。
托管的静态路径配置在 `static_server/serve/config/serve.conf.js`（`../../static`）。

> 鸿蒙（OHOS）原有一套独立的 Gradle 体系（`settings.ohos.gradle.kts` / `build.ohos.gradle.kts` /
> `ohosApp/runOhosApp.sh`，依赖专用 Kotlin 工具链 `2.0.21-KBA-010` 与 DevEco Studio SDK）。**当前暂不实现**，
> 上述脚手架保留但不维护，无需运行。

## 后端（backend/）

后端采用 **Python + FastAPI**，目录目前为空、待搭建。后续在此目录内组织 FastAPI 应用
（如 `app/main.py` 入口、路由/模型/依赖按需分层），通过 HTTP 向客户端提供会话相关接口；
客户端侧经由 `BridgeModule`（见上）或独立的网络模块与之通信。搭建时建议使用 `uv` / `pip` +
`requirements.txt` 或 `pyproject.toml` 管理依赖，并以 `uvicorn` 启动开发服务器。

## 测试

```sh
./gradlew :shared:allTests      # 所有 KMP 目标
./gradlew :shared:test          # 仅 JVM（commonTest）—— 使用 kotlin("test")
```

`commonTest` 是唯一的测试 source set；目前没有任何平台特定的测试。

## 编辑代码时的架构要点

- **Compose 原语来自 `com.tencent.kuikly.compose.*`，而不是 `androidx.compose.*`。**
  `BasicWidget.kt` 在 Kuikly Compose 之上重新实现了常用辅助函数（`TextField`、`Button`、`Modal`、
  `margin`、`borderRadius`、`touchListener`、`willAppear` 等）——优先复用这些，不要重造。注意
  `padding`/`margin`/`height`/`width` 都有 `Float` 重载版本，内部通过 `.dp` 转换。
- **状态管理：** `state/SessionViewModel.kt` 是一个纯 Compose state 持有者
  （`mutableStateOf` / `mutableStateListOf`），已通过 `remember { SessionViewModel() }` 接入 `SessionScreen`，
  输入栏的 `value`/`onValueChange`/`onSend` 全部走 viewModel。新增会话相关状态时优先扩展这个 ViewModel。
- **模型** 放在 `model/`（`Message`、`MessageRole`、`MessageStatus`）；**组件** 放在 `component/`；
  **页面** 放在 `page/`。保持分层：`page` → `component` → `base`/`state`/`model`。
- `BasePage` 处理深色模式（`isNightMode`，以 `isNightMode` 页面参数为键）并关闭了调试 UI 检查器
  （`debugUIInspector() = false`）——继承 `BasePage` 而非直接继承 `ComposeContainer`，以保留这些行为。
- iOS 部署目标 14.1；`shared` 以**静态**框架发布（`isStatic = true`），资源来自 `src/commonMain/assets/**`。

## 仓库注意事项

- `settings.gradle.kts` include 了 `:h5App` 和 `:miniApp`，但仓库里这两个目录并不存在。
  若 Gradle sync 因此报错，把它们注释掉或创建对应模块即可。
- 解析 Kuikly 产物必须能访问腾讯 Maven 镜像（`mirrors.tencent.com/nexus/repository/maven-tencent/`），
  请确保网络可达。
- `*.js` 和 `*.so` 在仓库根被 gitignore——构建出的 JS bundle 和原生库不会提交。dev server 托管的
  `static/` 目录同样是生成物，不入库。
