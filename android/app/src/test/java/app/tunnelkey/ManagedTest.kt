package app.tunnelkey

import app.tunnelkey.provision.SetupCodeAssembler
import app.tunnelkey.provision.SetupCodes
import app.tunnelkey.provision.SetupPart
import app.tunnelkey.security.PinPolicy
import app.tunnelkey.security.PinPolicy.Problem
import app.tunnelkey.security.Totp
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SetupCodeTest {
    /** A real code produced by the Go server (throwaway test keys). */
    private val serverCode = javaClass.classLoader!!.getResource("setup_code_single.txt")!!.readText()

    @Test fun decodesServerOutput() {
        val part = SetupPart.parse(serverCode)
        assertNotNull(part)
        val asm = SetupCodeAssembler()
        asm.add(part!!)
        assertTrue(asm.isComplete)
        val p = asm.payload()
        assertEquals("Office VPN", p.name)
        assertEquals("marko", p.username)
        assertEquals("test-password", p.password)
        assertEquals("JBSWY3DPEHPK3PXP", p.totp?.secret)
        assertEquals(6, p.totp?.digits)
        assertTrue(p.ovpn.contains("<tls-crypt>"))
        assertEquals(2, p.links.size)
        assertEquals("rdp", p.links[1].kind)
        assertTrue(p.links[1].uri.startsWith("rdp://full%20address=s:10.0.0.25:3389"))
    }

    @Test fun reassemblesPartsInAnyOrderAndRestoresTrimmedSpaces() {
        val data = serverCode.substringAfter(':').substringAfter(':').substringAfter(':').substringAfter(':')
        val a = data.substring(0, 900)
        val b = data.substring(900, 1800)
        val c = data.substring(1800) + ""
        val parts = listOf(
            "TK1:3/3:ABC123:${c.length}:$c",
            "TK1:1/3:ABC123:${a.length}:${a.trimEnd()}",
            "TK1:2/3:ABC123:${b.length}:${b.trimEnd()}",
        )
        val asm = SetupCodeAssembler()
        parts.forEachIndexed { i, text ->
            assertTrue(asm.add(SetupPart.parse(text)!!))
            assertEquals(i == 2, asm.isComplete)
        }
        assertFalse("duplicate part is not new", asm.add(SetupPart.parse(parts[0])!!))
        assertEquals("Office VPN", asm.payload().name)
    }

    @Test fun ignoresForeignCodes() {
        assertNull(SetupPart.parse("https://example.com"))
        assertNull(SetupPart.parse("TK1:0/1:X:1:A"))
        assertNull(SetupPart.parse("TK1:2/1:X:1:A"))
    }

    @Test fun base45RfcVectors() {
        assertEquals("AB", String(SetupCodes.base45Decode("BB8")))
        assertEquals("Hello!!", String(SetupCodes.base45Decode("%69 VD92EX0")))
        assertEquals("base-45", String(SetupCodes.base45Decode("UJCLQE7W581")))
    }
}

class TotpTest {
    // RFC 6238 appendix B
    private val seed20 = "12345678901234567890".toByteArray()
    private val seed32 = "12345678901234567890123456789012".toByteArray()
    private val seed64 = "1234567890123456789012345678901234567890123456789012345678901234".toByteArray()

    @Test fun rfcVectors() {
        assertEquals("94287082", Totp(seed20, 8, 30, "SHA1").code(59))
        assertEquals("46119246", Totp(seed32, 8, 30, "SHA256").code(59))
        assertEquals("90693936", Totp(seed64, 8, 30, "SHA512").code(59))
        assertEquals("07081804", Totp(seed20, 8, 30, "SHA1").code(1111111109))
        assertEquals("65353130", Totp(seed20, 8, 30, "SHA1").code(20000000000))
        assertEquals("081804", Totp(seed20, 6, 30, "SHA1").code(1111111109))
    }

    @Test fun base32() {
        assertEquals("Hello!Þ­¾ï", String(Totp.base32Decode("JBSWY3DPEHPK3PXP"), Charsets.ISO_8859_1))
        assertTrue(seed20.contentEquals(Totp.base32Decode("gezd gnbv gy3t qojq gezd gnbv gy3t qojq")))
    }

    @Test fun secondsLeft() {
        assertEquals(30, Totp(seed20).secondsLeft(60))
        assertEquals(1, Totp(seed20).secondsLeft(89))
    }
}

class PinPolicyTest {
    @Test fun rejectsUserExamplesAndCommonPatterns() {
        val weak = mapOf(
            "11111111" to Problem.TooFewDigits,
            "11223344" to Problem.Pairs,
            "12345678" to Problem.Sequence,
            "11112222" to Problem.TooFewDigits,
            "87654321" to Problem.Sequence,
            "90123456" to Problem.Sequence,
            "13579135" to Problem.Progression,
            "12341234" to Problem.Sequence,
            "12121212" to Problem.TooFewDigits,
            "13571357" to Problem.RepeatedBlock,
            "12344321" to Problem.Sequence,
            "37922973" to Problem.Mirror,
            "17031985" to Problem.Date,
            "19850317" to Problem.Date,
            "14725836" to Problem.Common,
            "1234567" to Problem.Length,
            "1234567a" to Problem.Length,
        )
        for ((pin, expected) in weak) {
            assertEquals("PIN $pin", expected, PinPolicy.check(pin))
        }
    }

    @Test fun acceptsReasonablePins() {
        for (pin in listOf("39174826", "40271953", "58302917", "71946053", "26093718")) {
            assertNull("PIN $pin", PinPolicy.check(pin))
        }
    }
}
