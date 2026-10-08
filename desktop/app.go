package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/hello"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/helperclient"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/ipc"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/links"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/ovpn"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/setupfile"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/store"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/totp"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/vault"
)

// version is set by the build.
var version = "1.0.0"

const (
	windowTitle    = "Tunnelkey"
	maxAutoRetries = 2
	maxImportBytes = 1 << 20
)

// App is bound to the frontend; exported methods are callable from JS.
type App struct {
	ctx     context.Context
	store   *store.Store
	vault   *vault.Vault
	helper  *helperclient.Client
	initErr error

	mu             sync.Mutex
	status         ipc.Status
	logs           []string
	helperUp       bool
	helperVersion  string
	secrets        *vault.Secrets // unlocked provisioned secrets
	drafts         map[string]*draft
	pendingSetup   *setupfile.Payload
	active         *activeSession // what the app last asked the helper to run
	autoRetries    int
	retryAt        time.Time
	retryTimer     *time.Timer
	pendingOpen    []string // files to open once the UI is ready
	needCreds      bool     // the helper waits for a reconnect sign-in
	helloAvailable bool
}

type draft struct {
	config string
	file   string
}

type activeSession struct {
	profileID string
	managed   bool
	username  string
	password  string
	twoFactor bool
	codeAfter bool
	static    bool
}

// NewApp opens the data directory.
func NewApp() *App {
	a := &App{drafts: map[string]*draft{}, status: ipc.Status{Phase: ipc.Disconnected}}
	dir, err := store.DefaultDir()
	if err == nil {
		a.store, err = store.Open(dir)
	}
	if err != nil {
		a.initErr = err
		return a
	}
	a.vault = vault.Open(dir, a.store.Sealer())
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Codes are only generated automatically behind Windows Hello biometrics.
	a.helloAvailable = hello.BiometricAvailable()
	a.helper = &helperclient.Client{OnEvent: a.onHelperEvent, OnConnection: a.onHelperConnection}
	go a.helper.Run(ctx)
	a.startTray()
	wruntime.OnFileDrop(ctx, func(_, _ int, paths []string) {
		for _, p := range paths {
			a.emitOpen(p)
		}
	})
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, "-") {
			a.mu.Lock()
			a.pendingOpen = append(a.pendingOpen, arg)
			a.mu.Unlock()
		}
	}
}

// onSecondInstance handles a file opened while the app is already running.
func (a *App) onSecondInstance(args []string) {
	if a.ctx == nil {
		return
	}
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			a.emitOpen(arg)
		}
	}
}

// onFileOpen is macOS' "open document" callback.
func (a *App) onFileOpen(path string) {
	if a.ctx == nil {
		a.mu.Lock()
		a.pendingOpen = append(a.pendingOpen, path)
		a.mu.Unlock()
		return
	}
	a.emitOpen(path)
}

func (a *App) emitOpen(path string) {
	wruntime.EventsEmit(a.ctx, "open-file", path)
}

// TakePendingFiles returns files passed on the command line (once).
func (a *App) TakePendingFiles() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.pendingOpen
	a.pendingOpen = nil
	return out
}

// ---- State ----------------------------------------------------------------

// ProfileView is a profile as the UI sees it.
type ProfileView struct {
	store.Profile
	HasSavedPassword bool `json:"hasSavedPassword"`
}

// ManagedView is the provisioned configuration as the UI sees it.
type ManagedView struct {
	Name             string           `json:"name"`
	ProfileID        string           `json:"profileId"`
	Remote           string           `json:"remote"`
	Links            []setupfile.Link `json:"links"`
	HasTOTP          bool             `json:"hasTotp"`
	HasPassword      bool             `json:"hasPassword"`
	ManualCode       bool             `json:"manualCode"`
	CodeLength       int              `json:"codeLength"`
	NeedsUser        bool             `json:"needsUser"` // no username provisioned
	NeedsCredentials bool             `json:"needsCredentials"`
	LockMethod       string           `json:"lockMethod"`
}

// AppState is everything the UI renders.
type AppState struct {
	Version        string        `json:"version"`
	Platform       string        `json:"platform"`
	Language       string        `json:"language"`
	InitError      string        `json:"initError,omitempty"`
	HelperUp       bool          `json:"helperUp"`
	HelperVersion  string        `json:"helperVersion"`
	Status         ipc.Status    `json:"status"`
	Profiles       []ProfileView `json:"profiles"`
	Managed        *ManagedView  `json:"managed"`
	Locked         bool          `json:"locked"`
	HelloAvailable bool          `json:"helloAvailable"`
	WeakKeyStorage bool          `json:"weakKeyStorage"`
	RetryIn        int           `json:"retryIn"` // seconds until an automatic retry, 0 = none
}

// State returns the current app state.
func (a *App) State() AppState {
	st := AppState{Version: version, Platform: runtime.GOOS, Profiles: []ProfileView{}}
	if a.initErr != nil {
		st.InitError = a.initErr.Error()
		return st
	}
	st.Language = a.store.Settings().Language
	st.WeakKeyStorage = a.store.WeakKeyStorage
	a.mu.Lock()
	st.HelperUp, st.HelperVersion, st.Status = a.helperUp, a.helperVersion, a.status
	st.HelloAvailable = a.helloAvailable
	unlocked := a.secrets != nil
	if !a.retryAt.IsZero() {
		st.RetryIn = max(1, int(time.Until(a.retryAt).Seconds()+0.99))
	}
	a.mu.Unlock()

	profiles, _ := a.store.Profiles()
	for _, p := range profiles {
		if p.Managed {
			continue
		}
		st.Profiles = append(st.Profiles, ProfileView{Profile: p, HasSavedPassword: p.RememberPassword && a.store.Password(p.ID) != ""})
	}
	if m := a.store.Managed(); m != nil {
		mv := &ManagedView{Name: m.Name, ProfileID: m.ProfileID, Links: m.Links, HasTOTP: m.HasTOTP,
			HasPassword: m.HasPassword, ManualCode: m.ManualCode, CodeLength: 6, LockMethod: string(a.vault.Method())}
		if m.Links == nil {
			mv.Links = []setupfile.Link{}
		}
		if p, err := a.store.Profile(m.ProfileID); err == nil {
			mv.Remote = p.Remote
			mv.CodeLength = p.CodeLength
			mv.NeedsUser = p.NeedsCredentials && p.Username == ""
			mv.NeedsCredentials = p.NeedsCredentials
		}
		if m.TOTPDigits > 0 {
			mv.CodeLength = m.TOTPDigits
		}
		st.Managed = mv
		method := a.vault.Method()
		st.Locked = !unlocked && (method == vault.Pin || method == vault.Hello)
	}
	return st
}

// SetLanguage stores "", "en" or "sk".
func (a *App) SetLanguage(lang string) error {
	st := a.store.Settings()
	st.Language = lang
	if err := a.store.SaveSettings(st); err != nil {
		return err
	}
	a.trayRelabel()
	return nil
}

func (a *App) changed() {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "state")
	}
}

// ---- Helper events ---------------------------------------------------------

func (a *App) onHelperConnection(up bool, v string) {
	a.mu.Lock()
	changed := a.helperUp != up
	a.helperUp, a.helperVersion = up, v
	st := a.status
	a.mu.Unlock()
	a.trayUpdate(st, up)
	if changed {
		a.changed()
	}
}

func (a *App) onHelperEvent(ev ipc.Event) {
	switch ev.Type {
	case "status":
		if ev.Status == nil {
			return
		}
		a.mu.Lock()
		prev := a.status.Phase
		a.status = *ev.Status
		up := a.helperUp
		a.mu.Unlock()
		a.trayUpdate(*ev.Status, up)
		wruntime.EventsEmit(a.ctx, "status", ev.Status)
		if ev.Status.Phase == ipc.Failed && prev != ipc.Failed {
			a.maybeRetry(*ev.Status)
		}
		if ev.Status.Phase != ipc.Reconnecting {
			a.mu.Lock()
			a.needCreds = false
			if ev.Status.Phase == ipc.Connected {
				a.autoRetries = 0
			}
			a.mu.Unlock()
		}
	case "log":
		a.mu.Lock()
		a.logs = append(a.logs, ev.Lines...)
		if len(a.logs) > 2000 {
			a.logs = a.logs[len(a.logs)-2000:]
		}
		a.mu.Unlock()
		wruntime.EventsEmit(a.ctx, "log", ev.Lines)
	case "need-creds":
		answered := a.answerAutomatically()
		a.mu.Lock()
		a.needCreds = !answered
		a.mu.Unlock()
		if !answered {
			wruntime.EventsEmit(a.ctx, "need-creds")
		}
	}
}

// Logs returns the connection log.
func (a *App) Logs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.logs...)
}

// ---- Import ----------------------------------------------------------------

// ImportResult tells the UI what a chosen file is.
type ImportResult struct {
	Kind  string `json:"kind"` // ovpn, setup, cancelled, error
	Path  string `json:"path,omitempty"`
	Draft *Draft `json:"draft,omitempty"`
	Error string `json:"error,omitempty"`
}

// Draft is an imported .ovpn waiting for the user to save it.
type Draft struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Remote           string `json:"remote"`
	NeedsCredentials bool   `json:"needsCredentials"`
	StaticChallenge  bool   `json:"staticChallenge"`
}

// ChooseFile opens the system file picker.
func (a *App) ChooseFile() ImportResult {
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Import",
		Filters: []wruntime.FileFilter{
			{DisplayName: "Tunnelkey and OpenVPN files (*.tunnelkey, *.ovpn, *.conf)", Pattern: "*.tunnelkey;*.ovpn;*.conf"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return ImportResult{Kind: "error", Error: err.Error()}
	}
	if path == "" {
		return ImportResult{Kind: "cancelled"}
	}
	return a.ImportPath(path)
}

// ImportPath inspects a dropped, opened or chosen file.
func (a *App) ImportPath(path string) ImportResult {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return ImportResult{Kind: "error", Error: "file_unreadable"}
	}
	if fi.Size() > maxImportBytes {
		return ImportResult{Kind: "error", Error: "file_too_large"}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ImportResult{Kind: "error", Error: "file_unreadable"}
	}
	if looksLikeSetupFile(b) {
		return ImportResult{Kind: "setup", Path: path}
	}
	if looksLikePlainPackage(b) {
		return ImportResult{Kind: "error", Error: "plain_package"}
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return a.importText(string(b), name)
}

// ImportText imports a pasted profile.
func (a *App) ImportText(text string) ImportResult {
	if looksLikeSetupFile([]byte(text)) {
		return ImportResult{Kind: "error", Error: "paste_setup_file"}
	}
	if looksLikePlainPackage([]byte(text)) {
		return ImportResult{Kind: "error", Error: "plain_package"}
	}
	return a.importText(text, "")
}

func looksLikeSetupFile(b []byte) bool {
	s := strings.TrimSpace(strings.TrimPrefix(string(b[:min(len(b), 4096)]), "\xef\xbb\xbf"))
	return strings.HasPrefix(s, "{") && strings.Contains(s, `"tunnelkey"`)
}

// looksLikePlainPackage spots the unencrypted package files that earlier
// versions of the setup page saved; they must be saved again encrypted.
func looksLikePlainPackage(b []byte) bool {
	var v map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &v) != nil {
		return false
	}
	_, hasOVPN := v["ovpn"]
	_, isSetup := v["tunnelkey"]
	return hasOVPN && !isSetup
}

func (a *App) importText(text, name string) ImportResult {
	text = ovpn.Normalize(text)
	if _, err := ovpn.Sanitize(text); err != nil {
		return ImportResult{Kind: "error", Error: err.Error()}
	}
	sum := ovpn.Inspect(text)
	if name == "" {
		name = strings.Split(sum.Remote, ":")[0]
	}
	id := store.NewID()
	a.mu.Lock()
	a.drafts[id] = &draft{config: text}
	a.mu.Unlock()
	return ImportResult{Kind: "ovpn", Draft: &Draft{ID: id, Name: name, Remote: sum.Remote,
		NeedsCredentials: sum.NeedsCredentials, StaticChallenge: sum.HasStaticChallenge}}
}

// ProfileInput is what the editor saves.
type ProfileInput struct {
	Name       string `json:"name"`
	Username   string `json:"username"`
	TwoFactor  bool   `json:"twoFactor"`
	CodeAfter  bool   `json:"codeAfter"`
	CodeLength int    `json:"codeLength"`
}

func (in ProfileInput) apply(p *store.Profile) error {
	p.Name = strings.TrimSpace(in.Name)
	if p.Name == "" {
		return errors.New("name_required")
	}
	p.Username = strings.TrimSpace(in.Username)
	p.TwoFactor, p.CodeAfter = in.TwoFactor, in.CodeAfter
	p.CodeLength = 6
	if in.CodeLength == 8 {
		p.CodeLength = 8
	}
	return nil
}

// SaveDraft turns a draft into a profile.
func (a *App) SaveDraft(id string, in ProfileInput) (string, error) {
	a.mu.Lock()
	d := a.drafts[id]
	delete(a.drafts, id)
	a.mu.Unlock()
	if d == nil {
		return "", errors.New("draft_expired")
	}
	sum := ovpn.Inspect(d.config)
	p := store.Profile{ID: store.NewID(), Remote: sum.Remote, NeedsCredentials: sum.NeedsCredentials,
		StaticChallenge: sum.HasStaticChallenge}
	if err := in.apply(&p); err != nil {
		return "", err
	}
	if err := a.store.SaveProfile(p, d.config); err != nil {
		return "", err
	}
	a.changed()
	return p.ID, nil
}

// UpdateProfile edits a profile's settings.
func (a *App) UpdateProfile(id string, in ProfileInput) error {
	p, err := a.store.Profile(id)
	if err != nil {
		return err
	}
	if err := in.apply(p); err != nil {
		return err
	}
	if err := a.store.SaveProfile(*p, ""); err != nil {
		return err
	}
	a.changed()
	return nil
}

// DeleteProfile removes a profile.
func (a *App) DeleteProfile(id string) error {
	a.mu.Lock()
	running := a.active != nil && a.active.profileID == id && a.status.Phase != ipc.Disconnected && a.status.Phase != ipc.Failed
	a.mu.Unlock()
	if running {
		a.Disconnect()
	}
	err := a.store.DeleteProfile(id)
	a.changed()
	return err
}

// ForgetPassword drops a saved password.
func (a *App) ForgetPassword(id string) error {
	p, err := a.store.Profile(id)
	if err != nil {
		return err
	}
	p.RememberPassword = false
	a.store.SaveProfile(*p, "")
	err = a.store.SetPassword(id, "")
	a.changed()
	return err
}

// ---- Setup files ---------------------------------------------------------------

// SetupSummary describes an opened setup file before it is installed.
type SetupSummary struct {
	Name        string           `json:"name"`
	Remote      string           `json:"remote"`
	HasTOTP     bool             `json:"hasTotp"`
	HasPassword bool             `json:"hasPassword"`
	ManualCode  bool             `json:"manualCode"`
	Links       []setupfile.Link `json:"links"`
	// CanUseHello: the 2FA secret can be kept behind Windows Hello
	// fingerprint/face so codes are filled in automatically.
	CanUseHello bool   `json:"canUseHello"`
	Replaces    string `json:"replaces,omitempty"`
	// TOTPNeedsHello: the file has a 2FA secret, but without Windows Hello
	// fingerprint/face it is not stored and the user types the codes.
	TOTPNeedsHello bool `json:"totpNeedsHello"`
}

// OpenSetupFile decrypts a setup file. Errors: not_setup_file, newer_version,
// wrong_password, damaged, profile:<reason>.
func (a *App) OpenSetupFile(path, password string) (*SetupSummary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("file_unreadable")
	}
	p, err := setupfile.Decrypt(b, password)
	switch {
	case errors.Is(err, setupfile.ErrWrongPassword):
		return nil, errors.New("wrong_password")
	case errors.Is(err, setupfile.ErrNewerVersion):
		return nil, errors.New("newer_version")
	case errors.Is(err, setupfile.ErrNotSetupFile):
		return nil, errors.New("not_setup_file")
	case err != nil:
		return nil, errors.New("damaged")
	}
	if _, err := ovpn.Sanitize(p.OVPN); err != nil {
		return nil, errors.New("profile:" + err.Error())
	}
	if p.TOTP != nil {
		if _, err := totp.New(p.TOTP.Secret, p.TOTP.Digits, p.TOTP.Period, p.TOTP.Algorithm); err != nil {
			return nil, errors.New("damaged")
		}
	}
	a.mu.Lock()
	a.pendingSetup = p
	a.mu.Unlock()
	return a.summaryFor(p), nil
}

func (a *App) summaryFor(p *setupfile.Payload) *SetupSummary {
	sum := &SetupSummary{Name: p.Name, Remote: ovpn.Inspect(p.OVPN).Remote, HasTOTP: p.TOTP != nil,
		HasPassword: p.Password != "", ManualCode: p.TOTP == nil && p.ManualCode, Links: p.Links,
		CanUseHello: p.TOTP != nil && a.helloAvailable, TOTPNeedsHello: p.TOTP != nil && !a.helloAvailable}
	if sum.Links == nil {
		sum.Links = []setupfile.Link{}
	}
	if m := a.store.Managed(); m != nil {
		sum.Replaces = m.Name
	}
	return sum
}

// InstallSetup stores the opened setup file. method "hello" keeps the 2FA
// secret behind Windows Hello fingerprint/face (automatic codes); "none"
// drops it and the user types codes. A provisioned password is always kept,
// sealed with the user's key like a remembered password. The desktop app has
// no PIN lock: a lock only exists to protect automatic codes.
func (a *App) InstallSetup(method string) error {
	a.mu.Lock()
	p := a.pendingSetup
	a.mu.Unlock()
	if p == nil {
		return errors.New("setup_expired")
	}
	m := vault.Method(method)
	if m == vault.Hello && p.TOTP == nil {
		m = vault.None // nothing that needs Windows Hello
	}
	keepTOTP := m == vault.Hello
	switch {
	case m == vault.Hello && !a.helloAvailable:
		return errors.New("hello_unavailable")
	case m == vault.Hello:
		if err := hello.Verify(windowTitle, "Fill in 2FA codes for "+p.Name); err != nil {
			return helloError(err)
		}
	case m != vault.None:
		return errors.New("lock_unsupported")
	}

	// Replace any earlier provisioned configuration.
	if old := a.store.Managed(); old != nil {
		a.store.DeleteProfile(old.ProfileID)
	}
	sum := ovpn.Inspect(p.OVPN)
	prof := store.Profile{
		ID: store.NewID(), Name: p.Name, Remote: sum.Remote, Username: p.Username,
		NeedsCredentials: sum.NeedsCredentials, StaticChallenge: sum.HasStaticChallenge,
		TwoFactor: p.TOTP != nil || p.ManualCode, CodeAfter: p.CodeAfter(), CodeLength: 6, Managed: true,
	}
	if p.TOTP != nil {
		prof.CodeLength = p.TOTP.Digits
	}
	secrets := vault.Secrets{Password: p.Password}
	if keepTOTP {
		secrets.TOTPSecret = p.TOTP.Secret
	}
	if err := a.vault.Store(m, "", secrets); err != nil {
		return err
	}
	if err := a.store.SaveProfile(prof, ovpn.Normalize(p.OVPN)); err != nil {
		return err
	}
	mg := &store.Managed{ProfileID: prof.ID, Name: p.Name, Links: p.Links, HasTOTP: keepTOTP,
		HasPassword: p.Password != "", ManualCode: p.ManualCode || (p.TOTP != nil && !keepTOTP), InstalledAt: time.Now().UnixMilli()}
	if p.TOTP != nil {
		mg.TOTPDigits, mg.TOTPPeriod, mg.TOTPAlgorithm = p.TOTP.Digits, p.TOTP.Period, p.TOTP.Algorithm
	}
	if err := a.store.SetManaged(mg); err != nil {
		return err
	}
	a.mu.Lock()
	a.pendingSetup = nil
	a.secrets = &secrets
	a.mu.Unlock()
	a.changed()
	return nil
}

// RemoveManaged leaves single-config mode and erases the provisioned data.
func (a *App) RemoveManaged() error {
	m := a.store.Managed()
	if m == nil {
		return nil
	}
	a.mu.Lock()
	running := a.active != nil && a.active.managed
	a.mu.Unlock()
	if running {
		a.Disconnect()
	}
	a.store.DeleteProfile(m.ProfileID)
	a.vault.Wipe()
	err := a.store.ClearManaged()
	a.mu.Lock()
	a.secrets = nil
	a.mu.Unlock()
	a.changed()
	return err
}

// ---- Lock -----------------------------------------------------------------------

// UnlockResult of a PIN attempt.
type UnlockResult struct {
	OK           bool  `json:"ok"`
	AttemptsLeft int   `json:"attemptsLeft"`
	LockedUntil  int64 `json:"lockedUntil"` // unix ms, 0 = none
	Wiped        bool  `json:"wiped"`
}

// UnlockWithPin opens the vault with the PIN.
func (a *App) UnlockWithPin(pin string) (UnlockResult, error) {
	r, err := a.vault.UnlockWithPin(pin)
	if r.Wiped {
		// Ten wrong PINs: erase the provisioned configuration like the phone apps.
		if m := a.store.Managed(); m != nil {
			a.store.DeleteProfile(m.ProfileID)
			a.store.ClearManaged()
		}
		a.changed()
		return UnlockResult{Wiped: true}, nil
	}
	if errors.Is(err, vault.ErrWrongPin) {
		out := UnlockResult{AttemptsLeft: r.AttemptsLeft}
		if !r.LockedUntil.IsZero() && r.LockedUntil.After(time.Now()) {
			out.LockedUntil = r.LockedUntil.UnixMilli()
		}
		return out, nil
	}
	if err != nil {
		return UnlockResult{}, err
	}
	// Earlier versions locked with a PIN; the desktop app no longer does.
	// After this last PIN unlock the lock and the 2FA secret are gone.
	a.unlockForGood(r.Secrets)
	a.mu.Lock()
	a.secrets = r.Secrets
	a.mu.Unlock()
	a.answerPending()
	a.changed()
	return UnlockResult{OK: true}, nil
}

// PinStatus reports the current lock-out for the PIN screen.
func (a *App) PinStatus() UnlockResult {
	f, until := a.vault.Status()
	out := UnlockResult{AttemptsLeft: vault.MaxAttempts - f}
	if until.After(time.Now()) {
		out.LockedUntil = until.UnixMilli()
	}
	return out
}

// UnlockWithHello asks Windows Hello, then opens the vault.
func (a *App) UnlockWithHello() error {
	name := "Tunnelkey"
	if m := a.store.Managed(); m != nil {
		name = m.Name
	}
	if err := hello.Verify(windowTitle, "Unlock "+name); err != nil {
		return helloError(err)
	}
	s, err := a.vault.Open()
	if err != nil {
		return err
	}
	if !a.helloAvailable {
		// Fingerprint/face was removed: stop generating codes, drop the lock.
		a.unlockForGood(s)
	}
	a.mu.Lock()
	a.secrets = s
	a.mu.Unlock()
	a.answerPending()
	a.changed()
	return nil
}

// answerPending answers a reconnect sign-in that arrived while locked.
func (a *App) answerPending() {
	a.mu.Lock()
	pending := a.needCreds
	a.mu.Unlock()
	if pending && a.answerAutomatically() {
		a.mu.Lock()
		a.needCreds = false
		a.mu.Unlock()
	}
}

func helloError(err error) error {
	switch {
	case errors.Is(err, hello.ErrCanceled):
		return errors.New("hello_cancelled")
	case errors.Is(err, hello.ErrUnavailable):
		return errors.New("hello_unavailable")
	default:
		return errors.New("hello_failed")
	}
}

// Lock forgets the unlocked secrets (window hidden for a while, or by hand).
func (a *App) Lock() {
	method := a.vault.Method()
	if method != vault.Pin && method != vault.Hello {
		return
	}
	a.mu.Lock()
	was := a.secrets != nil
	a.secrets = nil
	a.mu.Unlock()
	if was {
		a.changed()
	}
}

// TurnOffHello stops filling in 2FA codes: the 2FA secret is erased, the
// lock removed, and the user types codes from now on.
func (a *App) TurnOffHello() error {
	a.mu.Lock()
	sec := a.secrets
	a.mu.Unlock()
	if sec == nil {
		return errors.New("locked")
	}
	return a.unlockForGood(sec)
}

// unlockForGood erases a stored 2FA secret and keeps the remaining secrets
// (the password) without a lock, sealed with the user's key.
func (a *App) unlockForGood(sec *vault.Secrets) error {
	if sec == nil {
		return nil
	}
	hadTOTP := sec.TOTPSecret != ""
	sec.TOTPSecret = ""
	if err := a.vault.Store(vault.None, "", *sec); err != nil {
		return err
	}
	if m := a.store.Managed(); m != nil && hadTOTP {
		m.HasTOTP, m.ManualCode = false, true
		if err := a.store.SetManaged(m); err != nil {
			return err
		}
	}
	a.changed()
	return nil
}

// ---- Connect -----------------------------------------------------------------------

// ConnectInput is what the sign-in sheet collects.
type ConnectInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
	Remember bool   `json:"remember"`
}

// Connect starts a normal (non-provisioned) profile.
func (a *App) Connect(profileID string, in ConnectInput) error {
	p, err := a.store.Profile(profileID)
	if err != nil {
		return err
	}
	if p.Managed {
		return a.ConnectManaged(in)
	}
	username := p.Username
	if username == "" {
		username = strings.TrimSpace(in.Username)
	}
	password := in.Password
	if password == "" && p.RememberPassword {
		password = a.store.Password(p.ID)
	}
	if p.NeedsCredentials && p.TwoFactor && !validCode(in.Code, p.CodeLength) {
		return errors.New("code_invalid")
	}
	if in.Remember && in.Password != "" {
		a.store.SetPassword(p.ID, in.Password)
		p.RememberPassword = true
		a.store.SaveProfile(*p, "")
	}
	if in.Username != "" && p.Username == "" {
		p.Username = username
		a.store.SaveProfile(*p, "")
	}
	act := &activeSession{profileID: p.ID, username: username, password: password,
		twoFactor: p.TwoFactor, codeAfter: p.CodeAfter, static: p.StaticChallenge}
	a.cancelRetry()
	a.mu.Lock()
	a.autoRetries = 0
	a.mu.Unlock()
	return a.start(p, act, in.Code)
}

// ConnectManaged starts the provisioned configuration.
func (a *App) ConnectManaged(in ConnectInput) error {
	a.cancelRetry()
	a.mu.Lock()
	a.autoRetries = maxAutoRetries
	a.mu.Unlock()
	return a.connectManaged(in)
}

func (a *App) connectManaged(in ConnectInput) error {
	m := a.store.Managed()
	if m == nil {
		return errors.New("no_managed")
	}
	p, err := a.store.Profile(m.ProfileID)
	if err != nil {
		return err
	}
	a.mu.Lock()
	s := a.secrets
	a.mu.Unlock()
	method := a.vault.Method()
	if s == nil && (method == vault.Pin || method == vault.Hello) {
		return errors.New("locked")
	}
	if s == nil {
		s, err = a.vault.Open()
		if err != nil {
			s = &vault.Secrets{}
		}
	}
	username := p.Username
	if username == "" {
		username = strings.TrimSpace(in.Username)
	}
	password := s.Password
	if password == "" {
		password = in.Password
	}
	code := in.Code
	if s.TOTPSecret != "" {
		code, err = a.currentCode(m, s.TOTPSecret)
		if err != nil {
			return err
		}
	} else if p.TwoFactor && !validCode(code, p.CodeLength) {
		return errors.New("code_invalid")
	}
	act := &activeSession{profileID: p.ID, managed: true, username: username, password: password,
		twoFactor: p.TwoFactor, codeAfter: p.CodeAfter, static: p.StaticChallenge}
	return a.start(p, act, code)
}

func (a *App) currentCode(m *store.Managed, secret string) (string, error) {
	g, err := totp.New(secret, m.TOTPDigits, m.TOTPPeriod, m.TOTPAlgorithm)
	if err != nil {
		return "", err
	}
	return g.Code(time.Now()), nil
}

func validCode(code string, length int) bool {
	if len(code) != length {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (act *activeSession) credentials(code string) *ipc.Credentials {
	c := &ipc.Credentials{Username: act.username, Password: act.password}
	switch {
	case act.twoFactor && act.static:
		c.Response = code // sent as the static-challenge response
	case act.twoFactor && act.codeAfter:
		c.Password = act.password + code
	case act.twoFactor:
		c.Password = code + act.password
	}
	return c
}

func (a *App) start(p *store.Profile, act *activeSession, code string) error {
	config, err := a.store.Config(p.ID)
	if err != nil {
		return err
	}
	req := ipc.Request{Op: "connect", ProfileID: p.ID, ProfileName: p.Name, Config: config}
	if p.NeedsCredentials {
		req.Credentials = act.credentials(code)
	}
	if err := a.helper.Send(req); err != nil {
		return errors.New("helper_unavailable")
	}
	a.mu.Lock()
	a.active = act
	a.mu.Unlock()
	return nil
}

// Disconnect stops the tunnel and any pending automatic retry.
func (a *App) Disconnect() error {
	a.cancelRetry()
	a.mu.Lock()
	a.autoRetries = 0
	a.mu.Unlock()
	if err := a.helper.Send(ipc.Request{Op: "disconnect"}); err != nil {
		return errors.New("helper_unavailable")
	}
	return nil
}

// ProvideCredentials answers a "need-creds" request (reconnect sign-in).
func (a *App) ProvideCredentials(in ConnectInput) error {
	a.mu.Lock()
	act := a.active
	a.mu.Unlock()
	if act == nil {
		return errors.New("no_session")
	}
	copyAct := *act
	code := in.Code
	if act.managed && act.twoFactor && code == "" {
		// Provisioned TOTP: the app generates the code.
		a.mu.Lock()
		s := a.secrets
		a.mu.Unlock()
		m := a.store.Managed()
		if s == nil || s.TOTPSecret == "" || m == nil {
			return errors.New("locked")
		}
		var err error
		if code, err = a.currentCode(m, s.TOTPSecret); err != nil {
			return err
		}
	}
	if in.Password != "" {
		copyAct.password = in.Password
	}
	if in.Username != "" && copyAct.username == "" {
		copyAct.username = in.Username
	}
	a.mu.Lock()
	a.needCreds = false
	a.mu.Unlock()
	return a.helper.Send(ipc.Request{Op: "credentials", Credentials: copyAct.credentials(code)})
}

// answerAutomatically answers a reconnect sign-in when no typing is needed:
// provisioned TOTP, or a saved password without 2FA.
func (a *App) answerAutomatically() bool {
	a.mu.Lock()
	act, s := a.active, a.secrets
	a.mu.Unlock()
	if act == nil {
		return false
	}
	if act.managed {
		m := a.store.Managed()
		if m == nil || (act.twoFactor && (s == nil || s.TOTPSecret == "")) {
			return false
		}
		code := ""
		if act.twoFactor {
			var err error
			if code, err = a.currentCode(m, s.TOTPSecret); err != nil {
				return false
			}
		}
		return a.helper.Send(ipc.Request{Op: "credentials", Credentials: act.credentials(code)}) == nil
	}
	if !act.twoFactor && act.password != "" {
		return a.helper.Send(ipc.Request{Op: "credentials", Credentials: act.credentials("")}) == nil
	}
	return false
}

// maybeRetry: a provisioned TOTP sign-in that was rejected is retried with
// the next code, like the phone apps (the code may have just expired).
func (a *App) maybeRetry(st ipc.Status) {
	if st.Failure != ipc.AuthFailed {
		return
	}
	a.mu.Lock()
	act, s := a.active, a.secrets
	left := a.autoRetries
	a.mu.Unlock()
	m := a.store.Managed()
	if act == nil || !act.managed || left <= 0 || m == nil || s == nil || s.TOTPSecret == "" {
		return
	}
	g, err := totp.New(s.TOTPSecret, m.TOTPDigits, m.TOTPPeriod, m.TOTPAlgorithm)
	if err != nil {
		return
	}
	wait := time.Duration(g.SecondsLeft(time.Now())+2) * time.Second
	a.mu.Lock()
	a.autoRetries--
	a.retryAt = time.Now().Add(wait)
	if a.retryTimer != nil {
		a.retryTimer.Stop()
	}
	a.retryTimer = time.AfterFunc(wait, func() {
		a.mu.Lock()
		a.retryAt = time.Time{}
		a.retryTimer = nil
		a.mu.Unlock()
		if err := a.connectManaged(ConnectInput{}); err != nil {
			wruntime.EventsEmit(a.ctx, "error", err.Error())
		}
		a.changed()
	})
	a.mu.Unlock()
	a.changed()
}

func (a *App) cancelRetry() {
	a.mu.Lock()
	t := a.retryTimer
	a.retryTimer = nil
	had := !a.retryAt.IsZero()
	a.retryAt = time.Time{}
	a.mu.Unlock()
	if t != nil {
		t.Stop()
	}
	if had {
		a.changed()
	}
}

// ---- Links -------------------------------------------------------------------------

// OpenLink opens link i of the provisioned configuration.
func (a *App) OpenLink(i int) error {
	m := a.store.Managed()
	if m == nil || i < 0 || i >= len(m.Links) {
		return errors.New("no_link")
	}
	l := m.Links[i]
	if err := links.Open(l.Kind, l.URI, l.Title); err != nil {
		return errors.New("link_unsafe")
	}
	return nil
}

// OpenURL opens an http(s) link (About, privacy policy).
func (a *App) OpenURL(u string) error {
	return links.Open("web", u, "")
}
