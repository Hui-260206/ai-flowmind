import SwiftUI
import shared

struct ContentView: View {

    var body: some View {
        KuiklyRenderViewPage(pageName: "flowmind_session_page", data: [:]).ignoresSafeArea()
    }
}

struct ContentView_Previews: PreviewProvider {
    static var previews: some View {
        ContentView()
    }
}
