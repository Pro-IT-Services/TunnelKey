import SwiftUI

struct LogView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var lines: [String] = []
    private let timer = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    var body: some View {
        NavigationStack {
            Group {
                if lines.isEmpty {
                    Text("Nothing logged yet. Connect to a profile to see what happens.")
                        .multilineTextAlignment(.center)
                        .foregroundStyle(Palette.muted)
                        .padding(32)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    ScrollViewReader { proxy in
                        ScrollView {
                            LazyVStack(alignment: .leading, spacing: 2) {
                                ForEach(Array(lines.enumerated()), id: \.offset) { index, line in
                                    Text(line)
                                        .font(.caption.monospaced())
                                        .foregroundStyle(Palette.ink)
                                        .frame(maxWidth: .infinity, alignment: .leading)
                                        .id(index)
                                }
                            }
                            .textSelection(.enabled)
                            .padding(16)
                        }
                        .onAppear { proxy.scrollTo(lines.count - 1, anchor: .bottom) }
                        .onChange(of: lines.count) { count in proxy.scrollTo(count - 1, anchor: .bottom) }
                    }
                }
            }
            .background(Palette.canvas)
            .navigationTitle("Connection Log")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
                ToolbarItemGroup(placement: .bottomBar) {
                    ShareLink(item: lines.joined(separator: "\n")) {
                        Label("Share", systemImage: "square.and.arrow.up")
                    }
                    .disabled(lines.isEmpty)
                    Spacer()
                    Button(role: .destructive) {
                        SharedLog.shared.clear()
                        lines = []
                    } label: {
                        Label("Clear", systemImage: "trash")
                    }
                    .disabled(lines.isEmpty)
                }
            }
        }
        .onAppear { lines = SharedLog.shared.read() }
        .onReceive(timer) { _ in lines = SharedLog.shared.read() }
    }
}
