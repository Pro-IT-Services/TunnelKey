import Compression
import Foundation

// Reader for setup codes produced by the Tunnelkey server.
// Format: docs/provisioning-format.md

struct SetupTotp: Decodable {
    let secret: String
    let digits: Int
    let period: Int
    let algorithm: String

    enum CodingKeys: String, CodingKey { case secret = "s", digits = "d", period = "p", algorithm = "a" }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        secret = try c.decode(String.self, forKey: .secret)
        digits = try c.decodeIfPresent(Int.self, forKey: .digits) ?? 6
        period = try c.decodeIfPresent(Int.self, forKey: .period) ?? 30
        algorithm = (try c.decodeIfPresent(String.self, forKey: .algorithm) ?? "SHA1").uppercased()
    }
}

struct SetupLink: Codable, Hashable {
    let title: String
    let kind: String
    let uri: String

    enum CodingKeys: String, CodingKey { case title = "t", kind = "k", uri = "u" }
}

struct SetupPayload: Decodable {
    let version: Int
    let name: String
    let ovpn: String
    let username: String
    let password: String?
    let totp: SetupTotp?
    let manualCode: Bool
    let codePosition: String
    let links: [SetupLink]

    enum CodingKeys: String, CodingKey {
        case version = "v", name = "n", ovpn = "o", username = "u", password = "p"
        case totp = "t", manualCode = "f", codePosition = "c", links = "l"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        version = try c.decode(Int.self, forKey: .version)
        name = try c.decode(String.self, forKey: .name)
        ovpn = try c.decode(String.self, forKey: .ovpn)
        username = try c.decodeIfPresent(String.self, forKey: .username) ?? ""
        password = try c.decodeIfPresent(String.self, forKey: .password)
        totp = try c.decodeIfPresent(SetupTotp.self, forKey: .totp)
        manualCode = try c.decodeIfPresent(Bool.self, forKey: .manualCode) ?? false
        codePosition = try c.decodeIfPresent(String.self, forKey: .codePosition) ?? "a"
        links = try c.decodeIfPresent([SetupLink].self, forKey: .links) ?? []
    }

    var hasSecrets: Bool { !(password ?? "").isEmpty || totp != nil }
}

enum SetupCodeError: LocalizedError {
    case damaged, tooLarge, newerVersion, incomplete

    var errorDescription: String? {
        switch self {
        case .damaged: return "The setup code is damaged."
        case .tooLarge: return "The setup code is too large."
        case .newerVersion: return "This setup code needs a newer version of Tunnelkey."
        case .incomplete: return "The setup code is incomplete."
        }
    }
}

/// One scanned QR code of a set.
struct SetupPart: Equatable {
    let index: Int
    let total: Int
    let id: String
    let data: String

    /// Returns nil for QR codes that aren't Tunnelkey setup codes.
    static func parse(_ text: String) -> SetupPart? {
        guard text.hasPrefix("TK1:") else { return nil }
        let f = text.split(separator: ":", maxSplits: 4, omittingEmptySubsequences: false).map(String.init)
        guard f.count == 5 else { return nil }
        let nums = f[1].split(separator: "/").compactMap { Int($0) }
        guard nums.count == 2, let len = Int(f[3]) else { return nil }
        let (i, n) = (nums[0], nums[1])
        guard i >= 1, i <= n, n <= 64, !f[2].isEmpty else { return nil }
        // Some scanners trim trailing spaces, which are valid Base45.
        let data = f[4].count < len ? f[4] + String(repeating: " ", count: len - f[4].count) : f[4]
        return SetupPart(index: i, total: n, id: f[2], data: data)
    }
}

/// Collects the parts of one code set, in any order.
struct SetupCodeAssembler {
    private(set) var parts: [Int: String] = [:]
    private(set) var id: String?
    private(set) var total = 0

    var isComplete: Bool { total > 0 && parts.count == total }

    /// Returns true when the part was new. A part from another set restarts collection.
    mutating func add(_ part: SetupPart) -> Bool {
        if id != part.id || total != part.total {
            parts = [:]
            id = part.id
            total = part.total
        }
        return parts.updateValue(part.data, forKey: part.index) == nil
    }

    func payload() throws -> SetupPayload {
        guard isComplete else { throw SetupCodeError.incomplete }
        let text = (1...total).compactMap { parts[$0] }.joined()
        return try SetupCodes.decode(text)
    }
}

enum SetupCodes {
    private static let alphabet = Array("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:")
    private static let maxInflated = 512 * 1024

    static func decode(_ base45: String) throws -> SetupPayload {
        let raw = try zlibInflate(try base45Decode(base45))
        let payload: SetupPayload
        do {
            payload = try JSONDecoder().decode(SetupPayload.self, from: raw)
        } catch {
            throw SetupCodeError.damaged
        }
        guard payload.version == 1 else { throw SetupCodeError.newerVersion }
        guard !payload.name.isEmpty, !payload.ovpn.isEmpty else { throw SetupCodeError.incomplete }
        return payload
    }

    static func base45Decode(_ s: String) throws -> Data {
        var values: [Int] = []
        values.reserveCapacity(s.count)
        for ch in s {
            guard let v = alphabet.firstIndex(of: ch) else { throw SetupCodeError.damaged }
            values.append(v)
        }
        guard values.count % 3 != 1 else { throw SetupCodeError.damaged }
        var out = Data(capacity: values.count * 2 / 3)
        var i = 0
        while i < values.count {
            if i + 2 < values.count {
                let v = values[i] + values[i + 1] * 45 + values[i + 2] * 2025
                guard v <= 0xFFFF else { throw SetupCodeError.damaged }
                out.append(UInt8(v >> 8))
                out.append(UInt8(v & 0xFF))
            } else {
                let v = values[i] + values[i + 1] * 45
                guard v <= 0xFF else { throw SetupCodeError.damaged }
                out.append(UInt8(v))
            }
            i += 3
        }
        return out
    }

    /// zlib (RFC 1950): 2-byte header, raw deflate, Adler-32 trailer.
    static func zlibInflate(_ data: Data) throws -> Data {
        let bytes = [UInt8](data)
        guard bytes.count > 6, bytes[0] & 0x0F == 8, (Int(bytes[0]) << 8 | Int(bytes[1])) % 31 == 0 else {
            throw SetupCodeError.damaged
        }
        let deflate = Array(bytes[2..<(bytes.count - 4)])
        var output = [UInt8](repeating: 0, count: maxInflated)
        let written = deflate.withUnsafeBufferPointer { src in
            output.withUnsafeMutableBufferPointer { dst in
                compression_decode_buffer(dst.baseAddress!, dst.count, src.baseAddress!, src.count, nil, COMPRESSION_ZLIB)
            }
        }
        guard written > 0 else { throw SetupCodeError.damaged }
        guard written < maxInflated else { throw SetupCodeError.tooLarge }
        let result = Array(output[0..<written])

        let t = bytes.suffix(4)
        let expected = t.reduce(UInt32(0)) { $0 << 8 | UInt32($1) }
        guard adler32(result) == expected else { throw SetupCodeError.damaged }
        return Data(result)
    }

    private static func adler32(_ data: [UInt8]) -> UInt32 {
        var a: UInt32 = 1
        var b: UInt32 = 0
        for byte in data {
            a = (a + UInt32(byte)) % 65521
            b = (b + a) % 65521
        }
        return b << 16 | a
    }
}
