# FlowMind 移动端

Kuikly 共享层已经完成 MVP 会话体验：初始化时恢复上次选择，会话为空时自动创建；支持新建、切换和删除会话，加载服务端历史，在同一会话中连续多轮发送，以及失败后的历史对账和幂等重试。会话抽屉、标题、加载/空/错误状态、长消息与自动滚动均由 `shared/src/commonMain` 实现，Android/iOS 壳只负责渲染和注入运行配置。

## 网络边界

共享层只通过 Go API 的 `/api/v1/sessions` 访问服务端。页面从 Kuikly
`NetworkModule` 获取请求能力，并通过 `SharedPreferencesModule` 持久化匿名
UUID `X-Client-ID`；Python、gRPC、Redis 与 MySQL 不会暴露给移动端。

每个请求均带 `X-Client-ID` 与新的 `X-Request-ID`。JSON POST 带
`Content-Type: application/json`；删除会话使用 NetworkModule 底层
`httpRequest` 的显式 `"DELETE"` 方法，并接受 `204 No Content`。

NetworkModule 的平台原始连接失败文本不会直接显示给用户；无 HTTP 状态的
传输失败统一显示为“网络连接失败，请检查网络后重试”。Go API 的标准 JSON
错误包络仍保留服务端错误码、提示和 `request_id`。

## 开发地址

| 目标 | 默认或配置方式 |
| --- | --- |
| Android USB 真机（Debug 默认） | 执行 `adb reverse tcp:8080 tcp:8080`；Debug 默认使用 `http://127.0.0.1:8080`，也可用 Gradle 属性覆盖 |
| Android 无线真机 | 使用电脑当前局域网 IPv4 构建，例如 `./gradlew :androidApp:assembleDebug -Pflowmind.chatApiBaseUrl=http://30.27.133.17:8080`；手机和电脑必须处于可互通网络，电脑换网后需用新 IP 重新构建 |
| Android Emulator | 可同样使用 `adb reverse`，或通过 `-Pflowmind.chatApiBaseUrl=http://10.0.2.2:8080` 覆盖 |
| iOS Simulator（Debug 默认） | 默认使用 `http://127.0.0.1:8080`，可在 Debug Build Settings 以 `FLOWMIND_CHAT_API_BASE_URL` 覆盖 |
| iOS 无线真机 | iPhone 与 Mac 必须连接同一个允许客户端互访的 Wi-Fi；先用 `ipconfig getifaddr en0` 获取 Mac 当前 IPv4，再将 Debug Build Setting `FLOWMIND_CHAT_API_BASE_URL` 设置为 `http://<Mac IPv4>:8080` 并重新安装。不要沿用上一次网络的固定 IP |
| OpenHarmony / 真机 | 启动页面时在 `pageData.chatApiBaseUrl` 提供开发机可访问的局域网 URL |
| 生产 | 宿主必须提供 `pageData.chatApiBaseUrl`，且它必须是 HTTPS |

Android 已声明 Internet 权限。iOS 只在 Debug 专用的 `Info-Debug.plist` 中允许
本地 HTTP，并声明本地网络访问用途；Release 的 `Info.plist` 不含 HTTP 例外，生产
API 必须为 HTTPS。iOS 真机上的 `localhost` 是手机自身，不能用作开发机 API 地址。

共享页面仍允许宿主不传 `chatApiBaseUrl` 时使用 Mock Repository，便于独立预览；Android/iOS Debug 壳默认会注入本机开发服务地址。

## 构建与测试

```sh
cd mobile

# 共享层 common tests
./gradlew :shared:test

# Android Debug
./gradlew :androidApp:assembleDebug

# iOS 首次准备；随后用 Xcode 打开 iosApp/iosApp.xcworkspace
./gradlew :shared:generateDummyFramework
cd iosApp && pod install && cd ..
```

共享层测试覆盖 Repository 协议解析、会话选择持久化、初始化/历史状态、多轮发送、防重复点击、过期响应抑制、会话增删切换、失败对账与同 ID 重试。OpenHarmony 脚手架保留，但不属于 MVP 构建和验收范围。
