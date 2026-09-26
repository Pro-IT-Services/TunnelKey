import Foundation

/// Rules for the 8-digit app PIN. Same logic as the Android app (PinPolicy.kt):
/// repeats, runs, pairs, mirrors, dates and keypad shapes are refused.
enum PinPolicy {
    static let length = 8

    enum Problem: String {
        case length, tooFewDigits, repeatedDigit, sequence, progression, repeatedBlock, pairs, mirror, date, common

        var message: String {
            switch self {
            case .length: return "The PIN must be 8 digits."
            case .tooFewDigits: return "Use at least 4 different digits."
            case .repeatedDigit: return "Don't use the same digit 4 or more times."
            case .sequence: return "Avoid runs like 1234 or 9876."
            case .progression: return "Avoid evenly stepped numbers like 13579."
            case .repeatedBlock: return "Avoid repeating blocks like 12121212 or 12341234."
            case .pairs: return "Avoid pairs like 11223344."
            case .mirror: return "Avoid mirrored PINs like 12344321."
            case .date: return "That looks like a date. Pick something less guessable."
            case .common: return "That PIN is too common."
            }
        }
    }

    private static let common: Set<String> = [
        "14725836", "96385274", "25802580", "14789632", "12369874", "78963214", "15975346",
        "15935728", "75395128", "11235813", "31415926", "27182818", "13572468", "24681357",
        "01234567", "98765432", "12345679", "87654320", "20252026", "20262027",
    ]

    /// Returns the first problem found, or nil when the PIN is acceptable.
    static func check(_ pin: String) -> Problem? {
        guard pin.count == length, pin.allSatisfy({ ("0"..."9").contains($0) }) else { return .length }
        let d = pin.map { Int(String($0))! }

        if Set(d).count < 4 { return .tooFewDigits }
        if Dictionary(grouping: d, by: { $0 }).values.contains(where: { $0.count >= 4 }) { return .repeatedDigit }
        if longestRun(d, step: 1) >= 4 || longestRun(d, step: -1) >= 4 { return .sequence }
        for step in 2...8 where longestRun(d, step: step) >= 5 { return .progression }

        let s = Array(pin)
        if String(repeating: String(s[0..<2]), count: 4) == pin || String(repeating: String(s[0..<4]), count: 2) == pin {
            return .repeatedBlock
        }
        if stride(from: 0, to: length, by: 2).allSatisfy({ d[$0] == d[$0 + 1] }) { return .pairs }
        if String(s.reversed()) == pin || String(s[4...]) == String(s[0..<4].reversed()) { return .mirror }
        if looksLikeDate(d) { return .date }
        if common.contains(pin) { return .common }
        return nil
    }

    /// Longest run where each digit is the previous plus `step` (mod 10).
    private static func longestRun(_ d: [Int], step: Int) -> Int {
        var best = 1
        var run = 1
        for i in 1..<d.count {
            run = d[i] == ((d[i - 1] + step) % 10 + 10) % 10 ? run + 1 : 1
            best = max(best, run)
        }
        return best
    }

    private static func looksLikeDate(_ d: [Int]) -> Bool {
        func num(_ r: Range<Int>) -> Int { r.reduce(0) { $0 * 10 + d[$1] } }
        func valid(_ y: Int, _ m: Int, _ day: Int) -> Bool { (1900...2099).contains(y) && (1...12).contains(m) && (1...31).contains(day) }
        let a = num(0..<2), b = num(2..<4), c = num(4..<8)
        let y4 = num(0..<4), m2 = num(4..<6), d2 = num(6..<8)
        return valid(c, b, a) || valid(c, a, b) || valid(y4, m2, d2)
    }
}
