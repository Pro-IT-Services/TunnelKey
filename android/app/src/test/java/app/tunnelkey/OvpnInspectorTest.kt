package app.tunnelkey

import app.tunnelkey.data.OvpnInspector
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class OvpnInspectorTest {
    private val profile = """
        # comment
        client
        dev tun
        proto udp
        remote vpn.example.com 1194 udp
        remote backup.example.com 443 tcp
        auth-user-pass
        <ca>
        -----BEGIN CERTIFICATE-----
        remote not-a-directive 1
        -----END CERTIFICATE-----
        </ca>
    """.trimIndent()

    @Test fun readsFirstRemoteAndAuth() {
        val s = OvpnInspector.inspect(profile)
        assertEquals("vpn.example.com:1194/udp", s.remote)
        assertTrue(s.needsCredentials)
        assertTrue(s.isValid)
        assertNull(s.staticChallenge)
        assertTrue(s.externalFiles.isEmpty())
    }

    @Test fun detectsStaticChallengeAndExternalFiles() {
        val s = OvpnInspector.inspect("client\nremote h 1194\nca ca.crt\nstatic-challenge \"Enter code\" 1\n")
        assertEquals("Enter code", s.staticChallenge)
        assertEquals(listOf("ca.crt"), s.externalFiles)
        assertEquals("h:1194", s.remote)
    }

    @Test fun certificateOnlyProfile() {
        val s = OvpnInspector.inspect("client\nremote h\n<cert>\nx\n</cert>")
        assertFalse(s.needsCredentials)
    }

    @Test fun readsRemoteInsideConnectionBlock() {
        val s = OvpnInspector.inspect("client\n<connection>\nremote c.example 443 tcp\n</connection>\n")
        assertEquals("c.example:443/tcp", s.remote)
    }

    @Test fun rejectsNonProfiles() {
        assertFalse(OvpnInspector.inspect("hello world").isValid)
    }
}
