import SwiftUI

/// App name, version, what the app does with data, and where the policy and source live.
struct AboutView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL

    private let privacyPolicyURL = URL(string: "https://pro-it-services.github.io/TunnelKey/privacy-policy.html")!
    private let sourceCodeURL = URL(string: "https://github.com/Pro-IT-Services/TunnelKey")!

    private var version: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 0) {
                    Image(systemName: "lock.shield")
                        .font(.system(size: 44))
                        .foregroundStyle(Palette.brass)
                        .accessibilityHidden(true)
                    Text("Tunnelkey")
                        .font(.title2.weight(.semibold))
                        .foregroundStyle(Palette.ink)
                        .padding(.top, 14)
                    if !version.isEmpty {
                        Text("Version \(version)")
                            .font(.subheadline.monospacedDigit())
                            .foregroundStyle(Palette.muted)
                            .padding(.top, 4)
                    }
                    Text("Open-source OpenVPN client with authenticator-code support, by ProIT services. No analytics, no ads, no accounts: profiles, passwords and 2FA secrets stay on this device. Licensed under the GNU AGPL v3.")
                        .font(.body)
                        .foregroundStyle(Palette.ink)
                        .multilineTextAlignment(.center)
                        .padding(.top, 20)
                    Text("Built on OpenVPNAdapter and the OpenVPN 3 core.")
                        .font(.footnote)
                        .foregroundStyle(Palette.muted)
                        .multilineTextAlignment(.center)
                        .padding(.top, 10)
                }
                .frame(maxWidth: .infinity)
                .padding(.horizontal, 24)
                .padding(.top, 24)
            }
            .safeAreaInset(edge: .bottom) {
                VStack(spacing: 12) {
                    Button {
                        openURL(privacyPolicyURL)
                    } label: {
                        Label("Privacy policy", systemImage: "hand.raised")
                    }
                    Button {
                        openURL(sourceCodeURL)
                    } label: {
                        Label("Source code", systemImage: "chevron.left.forwardslash.chevron.right")
                    }
                }
                .buttonStyle(PrimaryButtonStyle(prominent: false))
                .padding(.horizontal, 24)
                .padding(.vertical, 12)
                .background(Palette.canvas)
            }
            .background(Palette.canvas)
            .navigationTitle("About")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
        }
    }
}
