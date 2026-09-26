package app.tunnelkey

import app.tunnelkey.data.CodePosition
import app.tunnelkey.data.Credentials
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CredentialsTest {
    @Test fun appendsCodeAfterPassword() {
        assertEquals("hunter2123456", Credentials.combine("hunter2", "123456", CodePosition.AFTER_PASSWORD))
    }

    @Test fun prependsCodeBeforePassword() {
        assertEquals("123456hunter2", Credentials.combine("hunter2", "123456", CodePosition.BEFORE_PASSWORD))
    }

    @Test fun keepsNonAsciiPasswordsIntact() {
        assertEquals("pässwörd🔑000111", Credentials.combine("pässwörd🔑", "000111", CodePosition.AFTER_PASSWORD))
    }

    @Test fun validatesCodes() {
        assertTrue(Credentials.isValidCode("012345", 6))
        assertTrue(Credentials.isValidCode("01234567", 8))
        assertFalse(Credentials.isValidCode("12345", 6))
        assertFalse(Credentials.isValidCode("12345a", 6))
        assertFalse(Credentials.isValidCode("１２３４５６", 6)) // full-width digits
    }
}
