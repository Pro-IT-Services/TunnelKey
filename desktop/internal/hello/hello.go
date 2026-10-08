// Package hello asks Windows Hello (face, fingerprint or Hello PIN) to confirm
// the user before the app opens its stored 2FA secret. Other systems report
// it as unavailable and the app uses its 8-digit PIN instead.
package hello

import "errors"

var (
	ErrCanceled    = errors.New("verification was cancelled")
	ErrUnavailable = errors.New("Windows Hello is not available")
	ErrFailed      = errors.New("Windows Hello could not verify you")
)
