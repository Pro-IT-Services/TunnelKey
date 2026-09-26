import SwiftUI

/// The tunnel: three nested arches. Grey when idle, brass shimmer while
/// connecting, and a slow teal flow through the arches once protected.
struct StatusHeroView: View {
    let phase: TunnelPhase
    let profileName: String?
    let connectedAt: Date?
    let vpnAddress: String
    let bytesIn: Int
    let bytesOut: Int
    let errorMessage: String?

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var tint: Color {
        switch phase {
        case .connected: return Palette.secure
        case .connecting, .reconnecting, .disconnecting: return Palette.brass
        case .failed: return Palette.danger
        case .disconnected: return Palette.idle
        }
    }

    private var title: String {
        switch phase {
        case .disconnected: return "Not connected"
        case .connecting: return "Connecting…"
        case .connected: return "Protected"
        case .reconnecting: return "Reconnecting…"
        case .disconnecting: return "Disconnecting…"
        case .failed: return "Connection failed"
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            arches
                .frame(width: 220, height: 170)
                .accessibilityHidden(true)

            Text(title)
                .font(.title2.weight(.semibold))
                .foregroundStyle(phase == .connected ? Palette.secure : Palette.ink)
                .padding(.top, 20)
                .accessibilityAddTraits(.updatesFrequently)

            HStack(spacing: 6) {
                if let profileName {
                    Text(profileName).foregroundStyle(Palette.muted)
                }
                if phase == .connected, let connectedAt {
                    Text("·").foregroundStyle(Palette.muted)
                    Text(connectedAt, style: .timer)
                        .monospacedDigit()
                        .foregroundStyle(Palette.muted)
                }
            }
            .font(.subheadline)
            .padding(.top, 4)

            if phase == .connected {
                HStack {
                    stat("VPN address", vpnAddress.isEmpty ? "—" : vpnAddress)
                    stat("Down", formatBytes(bytesIn))
                    stat("Up", formatBytes(bytesOut))
                }
                .padding(.top, 20)
            }

            if phase == .failed, let errorMessage, !errorMessage.isEmpty {
                Label(errorMessage, systemImage: "exclamationmark.circle")
                    .font(.subheadline)
                    .foregroundStyle(Palette.onDangerContainer)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(14)
                    .background(Palette.dangerContainer, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                    .padding(.top, 16)
            }
        }
        .frame(maxWidth: .infinity)
        .animation(.easeInOut(duration: 0.4), value: phase)
    }

    private func stat(_ label: String, _ value: String) -> some View {
        VStack(spacing: 4) {
            Text(label.uppercased())
                .font(.caption2.weight(.semibold))
                .tracking(1.2)
                .foregroundStyle(Palette.muted)
            Text(value)
                .font(.subheadline.monospaced())
                .foregroundStyle(Palette.ink)
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var arches: some View {
        let animated = !reduceMotion && phase != .disconnected && phase != .failed
        TimelineView(.animation(paused: !animated)) { context in
            let t = context.date.timeIntervalSinceReferenceDate
            Canvas { ctx, size in
                let stroke: CGFloat = 7
                let gap: CGFloat = 20
                for i in 0..<3 {
                    let inset = CGFloat(i) * gap + stroke / 2
                    let w = size.width - inset * 2
                    let radius = w / 2
                    let bottom = size.height - stroke / 2
                    var path = Path()
                    path.move(to: CGPoint(x: inset, y: bottom))
                    path.addLine(to: CGPoint(x: inset, y: inset + radius))
                    path.addArc(
                        center: CGPoint(x: size.width / 2, y: inset + radius),
                        radius: radius,
                        startAngle: .degrees(180), endAngle: .degrees(0), clockwise: false
                    )
                    path.addLine(to: CGPoint(x: size.width - inset, y: bottom))

                    let alpha: Double
                    var style = StrokeStyle(lineWidth: stroke, lineCap: .round)
                    if !animated {
                        alpha = 1 - Double(i) * 0.28
                    } else if phase == .connected {
                        alpha = 1 - Double(i) * 0.22
                        let dash: CGFloat = 14
                        style.dash = [dash * 3, dash]
                        style.dashPhase = -CGFloat((t / 2.6).truncatingRemainder(dividingBy: 1)) * dash * 4 * CGFloat(i + 1)
                    } else {
                        // Brass shimmer travelling inwards while connecting.
                        let p = (t / 0.9 + Double(i) * 0.33).truncatingRemainder(dividingBy: 1)
                        alpha = 0.3 + 0.7 * abs(sin(p * .pi))
                    }
                    ctx.stroke(path, with: .color(tint.opacity(alpha)), style: style)
                }
                var ground = Path()
                ground.move(to: CGPoint(x: 0, y: size.height))
                ground.addLine(to: CGPoint(x: size.width, y: size.height))
                ctx.stroke(ground, with: .color(Palette.idle.opacity(0.5)), lineWidth: 1)
            }
        }
    }
}
