import SwiftUI
import UIKit

/// "Ink & brass": a quiet graphite canvas, brass for the key action, and a
/// single teal signal reserved for "you are protected". Same palette as Android.
enum Palette {
    static let canvas = Color(light: 0xF5F3EE, dark: 0x0B0F14)
    static let raised = Color(light: 0xFFFFFF, dark: 0x131A22)
    static let high = Color(light: 0xECE8E0, dark: 0x1B2430)
    static let line = Color(light: 0xD8D3C8, dark: 0x2A3542)
    static let ink = Color(light: 0x161A1F, dark: 0xE6EDF3)
    static let muted = Color(light: 0x55606B, dark: 0x93A1B0)

    static let brass = Color(light: 0x8A5A00, dark: 0xE3A93B)
    static let onBrass = Color(light: 0xFFFFFF, dark: 0x1E1404)
    static let brassContainer = Color(light: 0xFBE3B3, dark: 0x3A2C10)
    static let onBrassContainer = Color(light: 0x2C1C00, dark: 0xFFDFA6)

    static let secure = Color(light: 0x0E7C5A, dark: 0x3CCB9B)
    static let idle = Color(light: 0xBDB6A8, dark: 0x3A4654)
    static let danger = Color(light: 0xB3261E, dark: 0xFF7A6B)
    static let dangerContainer = Color(light: 0xF9DEDC, dark: 0x4A1510)
    static let onDangerContainer = Color(light: 0x410E0B, dark: 0xFFD9D3)
}

extension Color {
    init(light: UInt32, dark: UInt32) {
        self.init(UIColor { traits in
            UIColor(hex: traits.userInterfaceStyle == .dark ? dark : light)
        })
    }
}

private extension UIColor {
    convenience init(hex: UInt32) {
        self.init(
            red: CGFloat((hex >> 16) & 0xFF) / 255,
            green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255,
            alpha: 1
        )
    }
}

/// Primary full-width action, sized for the thumb zone.
struct PrimaryButtonStyle: ButtonStyle {
    var prominent = true
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .frame(maxWidth: .infinity, minHeight: 56)
            .foregroundStyle(prominent ? Palette.onBrass : Palette.ink)
            .background(
                RoundedRectangle(cornerRadius: 16, style: .continuous)
                    .fill(prominent ? Palette.brass : Color.clear)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 16, style: .continuous)
                    .strokeBorder(prominent ? Color.clear : Palette.line, lineWidth: 1)
            )
            .opacity(isEnabled ? (configuration.isPressed ? 0.85 : 1) : 0.4)
            .contentShape(Rectangle())
            .animation(.easeOut(duration: 0.15), value: configuration.isPressed)
    }
}

func formatBytes(_ bytes: Int) -> String {
    ByteCountFormatter.string(fromByteCount: Int64(bytes), countStyle: .binary)
}
