import CryptoKit
import Foundation

/// RFC 6238 time-based one-time passwords.
struct Totp {
    let secret: Data
    var digits = 6
    var period = 30
    var algorithm = "SHA1"

    func code(at date: Date = Date()) -> String {
        let counter = UInt64(date.timeIntervalSince1970) / UInt64(period)
        let message = withUnsafeBytes(of: counter.bigEndian) { Data($0) }
        let key = SymmetricKey(data: secret)
        let mac: [UInt8]
        switch algorithm.uppercased() {
        case "SHA256": mac = Array(HMAC<SHA256>.authenticationCode(for: message, using: key))
        case "SHA512": mac = Array(HMAC<SHA512>.authenticationCode(for: message, using: key))
        default: mac = Array(HMAC<Insecure.SHA1>.authenticationCode(for: message, using: key))
        }
        let offset = Int(mac[mac.count - 1] & 0x0F)
        let binary = UInt32(mac[offset] & 0x7F) << 24
            | UInt32(mac[offset + 1]) << 16
            | UInt32(mac[offset + 2]) << 8
            | UInt32(mac[offset + 3])
        var modulus: UInt32 = 1
        for _ in 0..<digits { modulus *= 10 }
        let value = String(binary % modulus)
        return String(repeating: "0", count: max(0, digits - value.count)) + value
    }

    /// Seconds until the current code rolls over.
    func secondsLeft(at date: Date = Date()) -> Int {
        period - Int(UInt64(date.timeIntervalSince1970) % UInt64(period))
    }

    static func base32Decode(_ input: String) -> Data? {
        var buffer: UInt32 = 0
        var bits = 0
        var out = Data()
        for ch in input.uppercased() where ch != " " && ch != "-" && ch != "=" {
            let v: UInt32
            switch ch {
            case "A"..."Z": v = UInt32(ch.asciiValue! - Character("A").asciiValue!)
            case "2"..."7": v = UInt32(ch.asciiValue! - Character("2").asciiValue!) + 26
            default: return nil
            }
            buffer = (buffer << 5) | v
            bits += 5
            if bits >= 8 {
                out.append(UInt8((buffer >> UInt32(bits - 8)) & 0xFF))
                bits -= 8
            }
        }
        return out
    }
}
