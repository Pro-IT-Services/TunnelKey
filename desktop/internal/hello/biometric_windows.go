package hello

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winbio                       = windows.NewLazySystemDLL("winbio.dll")
	procWinBioGetEnrolledFactors = winbio.NewProc("WinBioGetEnrolledFactors")
)

// WINBIO_IDENTITY with an account SID (winbio_types.h).
type winbioIdentity struct {
	Type uint32 // WINBIO_ID_TYPE_SID = 3
	Size uint32
	Data [68]byte // SECURITY_MAX_SID_SIZE
}

const (
	winbioTypeFacialFeatures = 0x02
	winbioTypeFingerprint    = 0x08
	winbioTypeIris           = 0x10
)

// BiometricsEnrolled reports whether the current user has a fingerprint, face
// or iris enrolled for Windows Hello. A Hello PIN alone does not count.
func BiometricsEnrolled() bool {
	if procWinBioGetEnrolledFactors.Find() != nil {
		return false
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false
	}
	sid := user.User.Sid
	n := windows.GetLengthSid(sid)
	id := winbioIdentity{Type: 3}
	if n == 0 || int(n) > len(id.Data) {
		return false
	}
	id.Size = n
	copy(id.Data[:], unsafe.Slice((*byte)(unsafe.Pointer(sid)), n))
	var factors uint32
	hr, _, _ := procWinBioGetEnrolledFactors.Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&factors)))
	if failed(hr) {
		return false
	}
	return factors&(winbioTypeFacialFeatures|winbioTypeFingerprint|winbioTypeIris) != 0
}

// BiometricAvailable: Windows Hello is usable and has a fingerprint or face.
func BiometricAvailable() bool { return Available() && BiometricsEnrolled() }
