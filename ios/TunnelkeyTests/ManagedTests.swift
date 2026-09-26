import XCTest
@testable import Tunnelkey

final class SetupCodeTests: XCTestCase {
    private var serverCode: String {
        let url = Bundle(for: Self.self).url(forResource: "setup_code_single", withExtension: "txt")!
        return try! String(contentsOf: url, encoding: .utf8)
    }

    func testDecodesServerOutput() throws {
        var asm = SetupCodeAssembler()
        let part = try XCTUnwrap(SetupPart.parse(serverCode))
        XCTAssertTrue(asm.add(part))
        let p = try asm.payload()
        XCTAssertEqual(p.name, "Office VPN")
        XCTAssertEqual(p.username, "marko")
        XCTAssertEqual(p.password, "test-password")
        XCTAssertEqual(p.totp?.secret, "JBSWY3DPEHPK3PXP")
        XCTAssertTrue(p.ovpn.contains("<tls-crypt>"))
        XCTAssertEqual(p.links.count, 2)
        XCTAssertTrue(p.links[1].uri.hasPrefix("rdp://full%20address=s:10.0.0.25:3389"))
    }

    func testReassemblesPartsInAnyOrder() throws {
        let data = serverCode.split(separator: ":", maxSplits: 4).map(String.init)[4]
        let chars = Array(data)
        let a = String(chars[0..<900]), b = String(chars[900..<1800]), c = String(chars[1800...])
        let texts = [
            "TK1:3/3:ABC123:\(c.count):\(c)",
            "TK1:1/3:ABC123:\(a.count):\(a)",
            "TK1:2/3:ABC123:\(b.count):\(b)",
        ]
        var asm = SetupCodeAssembler()
        for t in texts { XCTAssertTrue(asm.add(try XCTUnwrap(SetupPart.parse(t)))) }
        XCTAssertFalse(asm.add(try XCTUnwrap(SetupPart.parse(texts[0]))))
        XCTAssertEqual(try asm.payload().name, "Office VPN")
    }

    func testBase45Vectors() throws {
        XCTAssertEqual(String(data: try SetupCodes.base45Decode("BB8"), encoding: .utf8), "AB")
        XCTAssertEqual(String(data: try SetupCodes.base45Decode("%69 VD92EX0"), encoding: .utf8), "Hello!!")
        XCTAssertNil(SetupPart.parse("https://example.com"))
    }
}

final class TotpTests: XCTestCase {
    func testRFC6238Vectors() {
        let seed20 = Data("12345678901234567890".utf8)
        let seed32 = Data("12345678901234567890123456789012".utf8)
        let seed64 = Data("1234567890123456789012345678901234567890123456789012345678901234".utf8)
        XCTAssertEqual(Totp(secret: seed20, digits: 8).code(at: Date(timeIntervalSince1970: 59)), "94287082")
        XCTAssertEqual(Totp(secret: seed32, digits: 8, algorithm: "SHA256").code(at: Date(timeIntervalSince1970: 59)), "46119246")
        XCTAssertEqual(Totp(secret: seed64, digits: 8, algorithm: "SHA512").code(at: Date(timeIntervalSince1970: 59)), "90693936")
        XCTAssertEqual(Totp(secret: seed20, digits: 8).code(at: Date(timeIntervalSince1970: 1111111109)), "07081804")
        XCTAssertEqual(Totp.base32Decode("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"), seed20)
    }
}

final class PinPolicyTests: XCTestCase {
    func testRejectsWeakPins() {
        let weak: [String: PinPolicy.Problem] = [
            "11111111": .tooFewDigits, "11223344": .pairs, "12345678": .sequence, "11112222": .tooFewDigits,
            "87654321": .sequence, "13579135": .progression, "13571357": .repeatedBlock,
            "37922973": .mirror, "17031985": .date, "19850317": .date, "14725836": .common, "1234567": .length,
        ]
        for (pin, problem) in weak { XCTAssertEqual(PinPolicy.check(pin), problem, pin) }
    }

    func testAcceptsReasonablePins() {
        for pin in ["39174826", "40271953", "58302917", "71946053", "26093718"] { XCTAssertNil(PinPolicy.check(pin), pin) }
    }
}
