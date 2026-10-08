// Package pinpolicy holds the rules for the 8-digit app PIN. Identical logic
// lives in the phone apps (PinPolicy.kt, PinPolicy.swift).
package pinpolicy

import "strconv"

// Length of every PIN.
const Length = 8

// Problem names the first rule a PIN breaks.
type Problem string

const (
	OK            Problem = ""
	BadLength     Problem = "length"
	TooFewDigits  Problem = "too_few_digits" // fewer than 4 different digits, e.g. 11112222
	RepeatedDigit Problem = "repeated_digit" // one digit used 4+ times
	Sequence      Problem = "sequence"       // 4+ consecutive ascending/descending
	Progression   Problem = "progression"    // constant step over 5+ digits, e.g. 13579135
	RepeatedBlock Problem = "repeated_block" // 12121212, 12341234
	Pairs         Problem = "pairs"          // 11223344
	Mirror        Problem = "mirror"         // 12344321
	Date          Problem = "date"           // 19850317, 17031985
	Common        Problem = "common"         // keypad shapes and other well-known PINs
)

var common = map[string]bool{
	"14725836": true, "96385274": true, "25802580": true, "14789632": true, "12369874": true,
	"78963214": true, "15975346": true, "15935728": true, "75395128": true, "11235813": true,
	"31415926": true, "27182818": true, "13572468": true, "24681357": true, "01234567": true,
	"98765432": true, "12345679": true, "87654320": true, "20252026": true, "20262027": true,
}

// Check returns the first problem found, or OK.
func Check(pin string) Problem {
	if len(pin) != Length {
		return BadLength
	}
	d := make([]int, Length)
	counts := map[int]int{}
	for i, c := range pin {
		if c < '0' || c > '9' {
			return BadLength
		}
		d[i] = int(c - '0')
		counts[d[i]]++
	}
	if len(counts) < 4 {
		return TooFewDigits
	}
	for _, n := range counts {
		if n >= 4 {
			return RepeatedDigit
		}
	}
	if longestRun(d, 1) >= 4 || longestRun(d, -1) >= 4 {
		return Sequence
	}
	for step := 2; step <= 8; step++ {
		if longestRun(d, step) >= 5 {
			return Progression
		}
	}
	if pin[:2]+pin[:2]+pin[:2]+pin[:2] == pin || pin[:4]+pin[:4] == pin {
		return RepeatedBlock
	}
	pairs := true
	for i := 0; i < Length; i += 2 {
		if d[i] != d[i+1] {
			pairs = false
		}
	}
	if pairs {
		return Pairs
	}
	if pin == reverse(pin) || pin[4:] == reverse(pin[:4]) {
		return Mirror
	}
	if looksLikeDate(pin) {
		return Date
	}
	if common[pin] {
		return Common
	}
	return OK
}

func longestRun(d []int, step int) int {
	best, run := 1, 1
	for i := 1; i < len(d); i++ {
		if d[i] == ((d[i-1]+step)%10+10)%10 {
			run++
		} else {
			run = 1
		}
		best = max(best, run)
	}
	return best
}

func reverse(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func looksLikeDate(pin string) bool {
	num := func(s string) int { n, _ := strconv.Atoi(s); return n }
	valid := func(y, m, day int) bool { return y >= 1900 && y <= 2099 && m >= 1 && m <= 12 && day >= 1 && day <= 31 }
	a, b, c := num(pin[0:2]), num(pin[2:4]), num(pin[4:8])
	y4, m2, d2 := num(pin[0:4]), num(pin[4:6]), num(pin[6:8])
	return valid(c, b, a) || valid(c, a, b) || valid(y4, m2, d2)
}
