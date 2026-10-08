import XCTest
@testable import Tunnelkey

/// Shared vector: server/testdata/setup_v1.tunnelkey (same file the server and desktop tests use).
final class SetupFileTests: XCTestCase {
    private let password = "correct-horse-battery-staple-07"

    private var vector: Data {
        let url = Bundle(for: Self.self).url(forResource: "setup_v1", withExtension: "tunnelkey")!
        return try! Data(contentsOf: url)
    }

    /// The vector with one envelope field changed.
    private func mutated(_ change: (inout [String: Any]) -> Void) throws -> Data {
        var obj = try XCTUnwrap(JSONSerialization.jsonObject(with: vector) as? [String: Any])
        change(&obj)
        return try JSONSerialization.data(withJSONObject: obj)
    }

    private func kdf(_ key: String, _ value: Any) -> (inout [String: Any]) -> Void {
        { obj in
            var kdf = obj["kdf"] as? [String: Any] ?? [:]
            kdf[key] = value
            obj["kdf"] = kdf
        }
    }

    private func assertFails(_ data: Data, with expected: SetupFileError, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertThrowsError(try SetupFile(data: data), file: file, line: line) { error in
            XCTAssertEqual(error as? SetupFileError, expected, file: file, line: line)
        }
    }

    func testDecryptsSharedVector() throws {
        let p = try SetupFile(data: vector).decrypt(password: password)
        XCTAssertEqual(p.name, "Office VPN")
        XCTAssertEqual(p.username, "marko")
        XCTAssertEqual(p.password, "p\u{E4}ssw\u{F6}rd")
        XCTAssertNotNil(p.totp)
        XCTAssertEqual(p.totp?.secret, "JBSWY3DPEHPK3PXP")
        XCTAssertFalse(p.ovpn.isEmpty)
        XCTAssertEqual(p.links.count, 2)
    }

    func testWrongPasswordFails() throws {
        let file = try SetupFile(data: vector)
        XCTAssertThrowsError(try file.decrypt(password: "correct-horse-battery-staple-08")) { error in
            XCTAssertEqual(error as? SetupFileError, .wrongPassword)
        }
        XCTAssertThrowsError(try file.decrypt(password: "")) { error in
            XCTAssertEqual(error as? SetupFileError, .wrongPassword)
        }
    }

    func testChangedParametersFailAuthentication() throws {
        // The AAD binds iter, so a changed (still valid) count can't silently weaken the KDF.
        let data = try mutated(kdf("iter", 600_001))
        XCTAssertThrowsError(try SetupFile(data: data).decrypt(password: password)) { error in
            XCTAssertEqual(error as? SetupFileError, .wrongPassword)
        }
    }

    func testRejectsMalformedEnvelopes() throws {
        assertFails(Data("client\nremote vpn.example.com 1194\n".utf8), with: .notASetupFile)
        assertFails(Data(#"{"hello":"world"}"#.utf8), with: .notASetupFile)
        assertFails(try mutated { $0["tunnelkey"] = "something-else" }, with: .notASetupFile)
        assertFails(try mutated { $0["v"] = 2 }, with: .newerVersion)
        assertFails(try mutated(kdf("alg", "PBKDF2-SHA1")), with: .damaged)
        assertFails(try mutated(kdf("iter", 99_999)), with: .damaged)
        assertFails(try mutated(kdf("iter", 10_000_001)), with: .damaged)
        assertFails(try mutated(kdf("salt", "AAAAAAAAAAAAAAAAAAAA")), with: .damaged) // 15 bytes
        assertFails(try mutated { obj in
            var enc = obj["enc"] as? [String: Any] ?? [:]
            enc["iv"] = "AAAAAAAAAAAAAAAAAAAAAA" // 16 bytes
            obj["enc"] = enc
        }, with: .damaged)
        assertFails(try mutated { $0["data"] = "AAAAAAAAAAAAAAAAAAAA" }, with: .damaged) // 15 bytes
        assertFails(try mutated { _ = $0.removeValue(forKey: "data") }, with: .damaged)
    }

    func testAcceptsIterationBounds() throws {
        XCTAssertNoThrow(try SetupFile(data: try mutated(kdf("iter", 100_000))))
        XCTAssertNoThrow(try SetupFile(data: try mutated(kdf("iter", 10_000_000))))
    }

    func testBase64URLWithoutPadding() {
        XCTAssertEqual(SetupFile.base64URLDecode("-_8"), Data([0xFB, 0xFF]))
        XCTAssertEqual(SetupFile.base64URLDecode("AQID"), Data([1, 2, 3]))
        XCTAssertNil(SetupFile.base64URLDecode("AQ=="))   // padding isn't allowed
        XCTAssertNil(SetupFile.base64URLDecode("+/8"))    // standard alphabet isn't allowed
        XCTAssertNil(SetupFile.base64URLDecode("AQIDB"))  // impossible length
    }
}
