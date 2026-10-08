package app.tunnelkey

import app.tunnelkey.provision.SetupFileException
import app.tunnelkey.provision.SetupFiles
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/** Decrypts a file made by the setup page's JavaScript (shared with the server tests). */
class SetupFileTest {
    private val file = javaClass.classLoader!!.getResource("setup_v1.tunnelkey")!!.readText()

    @Test
    fun decryptsBrowserFile() {
        assertTrue(SetupFiles.looksLike(file))
        val p = SetupFiles.decrypt(file, "correct-horse-battery-staple-07")
        assertEquals("Office VPN", p.name)
        assertEquals(2, p.links.size)
        assertNotNull(p.totp)
    }

    @Test
    fun rejectsWrongPasswordAndTampering() {
        expect(SetupFileException.Kind.WrongPassword) { SetupFiles.decrypt(file, "correct-horse-battery-staple-08") }
        val tampered = file.replace("\"iter\": 600000", "\"iter\": 100000")
        expect(SetupFileException.Kind.WrongPassword) { SetupFiles.decrypt(tampered, "correct-horse-battery-staple-07") }
        expect(SetupFileException.Kind.NotSetupFile) { SetupFiles.decrypt("client\nremote x\n", "x") }
        assertFalse(SetupFiles.looksLike("client\nremote vpn.example.com\n"))
    }

    private fun expect(kind: SetupFileException.Kind, block: () -> Unit) {
        try {
            block()
            fail("expected $kind")
        } catch (e: SetupFileException) {
            assertEquals(kind, e.kind)
        }
    }
}
