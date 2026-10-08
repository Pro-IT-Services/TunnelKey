package pinpolicy

import "testing"

func TestRefusesWeakPins(t *testing.T) {
	for pin, want := range map[string]Problem{
		"11111111": TooFewDigits, "11223344": Pairs, "12345678": Sequence, "11112222": TooFewDigits,
		"87654321": Sequence, "13579135": Progression, "12121212": TooFewDigits, "12341234": Sequence,
		"12344321": Sequence, "19850317": Date, "17031985": Date, "14725836": Common, "1234567": BadLength,
		"1234567a": BadLength, "48159263": OK, "73920516": OK,
	} {
		if got := Check(pin); got != want {
			t.Errorf("%s: got %q want %q", pin, got, want)
		}
	}
}
