# FlowMind 移动端网络配置

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
| Android USB 真机（推荐） | 先执行 `adb reverse tcp:8080 tcp:8080`，构建时传入 `-Pflowmind.chatApiBaseUrl=http://127.0.0.1:8080` |
| Android 无线真机 | 使用电脑当前局域网 IPv4 构建，例如 `./gradlew :androidApp:assembleDebug -Pflowmind.chatApiBaseUrl=http://30.27.133.17:8080`；手机和电脑必须处于可互通网络，电脑换网后需用新 IP 重新构建 |
| Android Emulator | 使用 `-Pflowmind.chatApiBaseUrl=http://10.0.2.2:8080` 覆盖 |
| iOS Simulator | 在 Debug Build Settings 以 `FLOWMIND_CHAT_API_BASE_URL=http://localhost:8080` 覆盖即可 |
| iOS 无线真机 | 在 Debug Build Settings 或启动环境中设置 `FLOWMIND_CHAT_API_BASE_URL=http://<开发机 IPv4>:8080`，然后重新安装 |
| OpenHarmony / 真机 | 启动页面时在 `pageData.chatApiBaseUrl` 提供开发机可访问的局域网 URL |
| 生产 | 宿主必须提供 `pageData.chatApiBaseUrl`，且它必须是 HTTPS |

Android 已声明 Internet 权限。iOS 只在 Debug 专用的 `Info-Debug.plist` 中允许
本地 HTTP，并声明本地网络访问用途；Release 的 `Info.plist` 不含 HTTP 例外，生产
API 必须为 HTTPS。iOS 真机上的 `localhost` 是手机自身，不能用作开发机 API 地址。

没有 `chatApiBaseUrl` 时页面会保留 Mock Repository，便于离线 UI 开发。
