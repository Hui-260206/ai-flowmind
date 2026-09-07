import SwiftUI
import shared

struct ContentView: View {

    private static let apiBaseURLInfoKey = "FlowMindChatApiBaseURL"

    private var pageData: [String: Any] {
        #if DEBUG
        let environment = ProcessInfo.processInfo.environment
        let environmentURL = environment["FLOWMIND_CHAT_API_BASE_URL"]?.trimmingCharacters(in: .whitespacesAndNewlines)
        let bundledURL = Bundle.main.object(forInfoDictionaryKey: Self.apiBaseURLInfoKey) as? String
        let baseURL = (environmentURL?.isEmpty == false ? environmentURL : bundledURL?.trimmingCharacters(in: .whitespacesAndNewlines)) ?? ""

        guard !baseURL.isEmpty else {
            // An unconfigured Debug host deliberately keeps the page on Mock data.
            return [:]
        }
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
