import SwiftUI
import shared

struct ContentView: View {

    private static let apiBaseURLInfoKey = "FlowMindChatApiBaseURL"

    private var pageData: [String: Any] {
        #if DEBUG
        let environment = ProcessInfo.processInfo.environment
        let environmentURL = environment["FLOWMIND_CHAT_API_BASE_URL"]?.trimmingCharacters(in: .whitespacesAndNewlines)
        let bundledURL = Bundle.main.object(forInfoDictionaryKey: Self.apiBaseURLInfoKey) as? String
        #if targetEnvironment(simulator)
        let defaultURL = "http://127.0.0.1:8080"
        #else
        let defaultURL = ""
        #endif
        let configuredURL = environmentURL?.isEmpty == false
            ? environmentURL
            : bundledURL?.trimmingCharacters(in: .whitespacesAndNewlines)
        let baseURL = configuredURL?.isEmpty == false ? configuredURL! : defaultURL
        guard !baseURL.isEmpty else { return [:] }
        return [
            "chatApiBaseUrl": baseURL,
            "chatApiProduction": false,
        ]
        #else
        // A release host must supply an explicit HTTPS endpoint before enabling remote chat.
        return [:]
        #endif
    }

    var body: some View {
        KuiklyRenderViewPage(pageName: "flowmind_session_page", data: pageData).ignoresSafeArea()
    }
}

struct ContentView_Previews: PreviewProvider {
    static var previews: some View {
        ContentView()
    }
}
