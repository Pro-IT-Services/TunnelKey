import SwiftUI

/// One-time-code input drawn as digit cells. Underneath is a single text field
/// with `.oneTimeCode` content type, so iOS can offer codes from Messages or
/// from verification codes saved in the Passwords app.
struct CodeField: View {
    @Binding var code: String
    let length: Int
    var focused: FocusState<Bool>.Binding

    var body: some View {
        ZStack {
            TextField("", text: $code)
                .keyboardType(.numberPad)
                .textContentType(.oneTimeCode)
                .focused(focused)
                .foregroundStyle(.clear)
                .tint(.clear)
                .accessibilityLabel("Authenticator code")
                .onChange(of: code) { newValue in
                    let digits = String(newValue.filter(\.isASCIIDigit).prefix(length))
                    if digits != newValue { code = digits }
                }

            HStack(spacing: 6) {
                ForEach(0..<length, id: \.self) { i in
                    cell(i)
                    if i == length / 2 - 1 { Spacer().frame(width: 8) }
                }
            }
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }
        .frame(height: 58)
        .contentShape(Rectangle())
        .onTapGesture { focused.wrappedValue = true }
    }

    private func cell(_ i: Int) -> some View {
        let chars = Array(code)
        let char = i < chars.count ? String(chars[i]) : ""
        let active = focused.wrappedValue && (i == chars.count || (i == length - 1 && chars.count == length))
        return Text(char)
            .font(.title2.monospacedDigit().weight(.medium))
            .foregroundStyle(Palette.ink)
            .frame(width: length > 6 ? 32 : 44, height: 58)
            .background(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .strokeBorder(
                        active ? Palette.brass : (char.isEmpty ? Palette.line : Palette.muted),
                        lineWidth: active ? 2 : 1
                    )
            )
    }
}

private extension Character {
    var isASCIIDigit: Bool { ("0"..."9").contains(self) }
}
