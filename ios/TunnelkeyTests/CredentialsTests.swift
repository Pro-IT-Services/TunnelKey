import XCTest
@testable import Tunnelkey

final class CredentialsTests: XCTestCase {
    func testAppendsCodeAfterPassword() {
        XCTAssertEqual(Credentials.combine(password: "hunter2", code: "123456", position: .afterPassword), "hunter2123456")
    }

    func testPrependsCodeBeforePassword() {
        XCTAssertEqual(Credentials.combine(password: "hunter2", code: "123456", position: .beforePassword), "123456hunter2")
    }

    func testValidatesCodes() {
        XCTAssertTrue(Credentials.isValidCode("012345", length: 6))
        XCTAssertTrue(Credentials.isValidCode("01234567", length: 8))
        XCTAssertFalse(Credentials.isValidCode("12345", length: 6))
        XCTAssertFalse(Credentials.isValidCode("12345a", length: 6))
        XCTAssertFalse(Credentials.isValidCode("１２３４５６", length: 6))
    }
}

final class OvpnInspectorTests: XCTestCase {
    func testReadsFirstRemoteAndAuth() {
        let s = OvpnInspector.inspect("""
        client
        remote vpn.example.com 1194 udp
        remote backup.example.com 443 tcp
        auth-user-pass
        <ca>
        remote not-a-directive 1
        </ca>
        """)
        XCTAssertEqual(s.remote, "vpn.example.com:1194/udp")
        XCTAssertTrue(s.needsCredentials)
        XCTAssertTrue(s.isValid)
        XCTAssertNil(s.staticChallenge)
    }

    func testDetectsStaticChallengeAndExternalFiles() {
        let s = OvpnInspector.inspect("client\nremote h 1194\nca ca.crt\nstatic-challenge \"Enter code\" 1\n")
        XCTAssertEqual(s.staticChallenge, "Enter code")
        XCTAssertEqual(s.externalFiles, ["ca.crt"])
    }

    func testReadsRemoteInsideConnectionBlock() {
        let s = OvpnInspector.inspect("client\n<connection>\nremote c.example 443 tcp\n</connection>\n")
        XCTAssertEqual(s.remote, "c.example:443/tcp")
    }

    func testRejectsNonProfiles() {
        XCTAssertFalse(OvpnInspector.inspect("hello world").isValid)
    }
}
