package app.tunnelkey.security

/**
 * Rules for the 8-digit app PIN. A PIN guards the stored TOTP secret, so the
 * patterns people reach for first (repeats, runs, pairs, dates, keypad shapes)
 * are refused. Identical logic lives in the iOS app (PinPolicy.swift).
 */
object PinPolicy {
    const val LENGTH = 8

    enum class Problem {
        Length,
        TooFewDigits,     // fewer than 4 different digits, e.g. 11112222
        RepeatedDigit,    // one digit used 4+ times
        Sequence,         // 4+ consecutive ascending/descending, e.g. 12345678, 87654321
        Progression,      // constant step over 5+ digits, e.g. 13579135
        RepeatedBlock,    // 12121212, 12341234
        Pairs,            // 11223344
        Mirror,           // 12344321
        Date,             // 19850317, 17031985
        Common,           // keypad shapes and other well-known PINs
    }

    private val common = setOf(
        "14725836", "96385274", "25802580", "14789632", "12369874", "78963214", "15975346",
        "15935728", "75395128", "11235813", "31415926", "27182818", "13572468", "24681357",
        "01234567", "98765432", "12345679", "87654320", "20252026", "20262027",
    )

    /** Returns the first problem found, or null when the PIN is acceptable. */
    fun check(pin: String): Problem? {
        if (pin.length != LENGTH || pin.any { it !in '0'..'9' }) return Problem.Length
        val d = pin.map { it - '0' }

        if (d.toSet().size < 4) return Problem.TooFewDigits
        if (d.groupingBy { it }.eachCount().values.any { it >= 4 }) return Problem.RepeatedDigit
        if (longestRun(d, 1) >= 4 || longestRun(d, -1) >= 4) return Problem.Sequence
        for (step in 2..8) {
            if (longestRun(d, step) >= 5) return Problem.Progression
        }
        if (pin.substring(0, 2).repeat(4) == pin || pin.substring(0, 4).repeat(2) == pin) return Problem.RepeatedBlock
        if ((0 until LENGTH step 2).all { d[it] == d[it + 1] }) return Problem.Pairs
        if (pin == pin.reversed() || pin.substring(4) == pin.substring(0, 4).reversed()) return Problem.Mirror
        if (looksLikeDate(pin)) return Problem.Date
        if (pin in common) return Problem.Common
        return null
    }

    /** Longest run where each digit is the previous plus [step] (mod 10). */
    private fun longestRun(d: List<Int>, step: Int): Int {
        var best = 1
        var run = 1
        for (i in 1 until d.size) {
            run = if (d[i] == Math.floorMod(d[i - 1] + step, 10)) run + 1 else 1
            best = maxOf(best, run)
        }
        return best
    }

    private fun looksLikeDate(pin: String): Boolean {
        fun valid(y: Int, m: Int, day: Int) =
            y in 1900..2099 && m in 1..12 && day in 1..31
        val a = pin.substring(0, 2).toInt()
        val b = pin.substring(2, 4).toInt()
        val c = pin.substring(4, 8).toInt()
        val y4 = pin.substring(0, 4).toInt()
        val m2 = pin.substring(4, 6).toInt()
        val d2 = pin.substring(6, 8).toInt()
        return valid(c, b, a) ||      // DDMMYYYY
            valid(c, a, b) ||         // MMDDYYYY
            valid(y4, m2, d2)         // YYYYMMDD
    }
}
