// Package store keeps profiles and app data in the user's config directory.
// Profiles and their metadata are JSON; OpenVPN configs (which carry private
// keys) and saved passwords are sealed with the user's master key.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/protect"
	"github.com/Pro-IT-Services/TunnelKey/desktop/internal/setupfile"
)

// Profile mirrors the phone app's profile.
type Profile struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Remote           string `json:"remote"`
	Username         string `json:"username"`
	NeedsCredentials bool   `json:"needsCredentials"`
	TwoFactor        bool   `json:"twoFactor"`
	CodeAfter        bool   `json:"codeAfter"` // code goes after the password
	CodeLength       int    `json:"codeLength"`
	StaticChallenge  bool   `json:"staticChallenge"`
	RememberPassword bool   `json:"rememberPassword"`
	ImportedAt       int64  `json:"importedAt"`
	Managed          bool   `json:"managed"` // installed from a setup file; hidden from the list
}

// Managed describes the provisioned configuration (single-config mode).
type Managed struct {
	ProfileID     string           `json:"profileId"`
	Name          string           `json:"name"`
	Links         []setupfile.Link `json:"links"`
	HasTOTP       bool             `json:"hasTotp"`
	TOTPDigits    int              `json:"totpDigits"`
	TOTPPeriod    int              `json:"totpPeriod"`
	TOTPAlgorithm string           `json:"totpAlgorithm"`
	HasPassword   bool             `json:"hasPassword"`
	ManualCode    bool             `json:"manualCode"`
	InstalledAt   int64            `json:"installedAt"`
}

// Store is safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	dir    string
	sealer *protect.Sealer
	// WeakKeyStorage: no OS key store was available.
	WeakKeyStorage bool
}

// Open prepares dir (created 0700).
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0o700); err != nil {
		return nil, err
	}
	s, weak, err := protect.OpenSealer(dir)
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, sealer: s, WeakKeyStorage: weak}, nil
}

// DefaultDir is the per-user data directory.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Tunnelkey"), nil
}

// Dir returns the data directory.
func (s *Store) Dir() string { return s.dir }

// Sealer exposes the master-key sealer (used by the vault).
func (s *Store) Sealer() *protect.Sealer { return s.sealer }

// NewID returns a random profile id.
func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- Profiles -----------------------------------------------------------

func (s *Store) profilesPath() string { return filepath.Join(s.dir, "profiles.json") }

// Profiles returns all profiles, newest first.
func (s *Store) Profiles() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadProfiles()
}

func (s *Store) loadProfiles() ([]Profile, error) {
	var list []Profile
	if err := readJSON(s.profilesPath(), &list); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ImportedAt > list[j].ImportedAt })
	return list, nil
}

// Profile returns one profile.
func (s *Store) Profile(id string) (*Profile, error) {
	list, err := s.Profiles()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, os.ErrNotExist
}

// SaveProfile adds or replaces p; config is written when non-empty.
func (s *Store) SaveProfile(p Profile, config string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if config != "" {
		sealed := s.sealer.Seal([]byte(config), []byte("config:"+p.ID))
		if err := writeFile(filepath.Join(s.dir, "configs", p.ID+".bin"), sealed); err != nil {
			return err
		}
	}
	list, err := s.loadProfiles()
	if err != nil {
		return err
	}
	if p.ImportedAt == 0 {
		p.ImportedAt = time.Now().UnixMilli()
	}
	replaced := false
	for i := range list {
		if list[i].ID == p.ID {
			list[i] = p
			replaced = true
		}
	}
	if !replaced {
		list = append(list, p)
	}
	return writeJSON(s.profilesPath(), list)
}

// DeleteProfile removes a profile, its config and saved password.
func (s *Store) DeleteProfile(id string) error {
	s.mu.Lock()
	list, err := s.loadProfiles()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	out := list[:0]
	for _, p := range list {
		if p.ID != id {
			out = append(out, p)
		}
	}
	os.Remove(filepath.Join(s.dir, "configs", id+".bin"))
	err = writeJSON(s.profilesPath(), out)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.SetPassword(id, "")
}

// Config returns the decrypted OpenVPN config of a profile.
func (s *Store) Config(id string) (string, error) {
	sealed, err := os.ReadFile(filepath.Join(s.dir, "configs", id+".bin"))
	if err != nil {
		return "", err
	}
	plain, err := s.sealer.Open(sealed, []byte("config:"+id))
	return string(plain), err
}

// ---- Saved passwords ----------------------------------------------------

func (s *Store) passwords() (map[string]string, error) {
	m := map[string]string{}
	sealed, err := os.ReadFile(filepath.Join(s.dir, "passwords.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := s.sealer.Open(sealed, []byte("passwords"))
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(plain, &m)
}

// Password returns the saved password of a profile ("" when none).
func (s *Store) Password(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.passwords()
	if err != nil {
		return ""
	}
	return m[id]
}

// SetPassword saves (or with "" forgets) a profile's password.
func (s *Store) SetPassword(id, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.passwords()
	if err != nil {
		m = map[string]string{}
	}
	if password == "" {
		if _, ok := m[id]; !ok {
			return nil
		}
		delete(m, id)
	} else {
		m[id] = password
	}
	plain, _ := json.Marshal(m)
	return writeFile(filepath.Join(s.dir, "passwords.bin"), s.sealer.Seal(plain, []byte("passwords")))
}

// ---- Managed configuration ---------------------------------------------

func (s *Store) managedPath() string { return filepath.Join(s.dir, "managed.json") }

// Managed returns the provisioned configuration, or nil.
func (s *Store) Managed() *Managed {
	var m Managed
	if err := readJSON(s.managedPath(), &m); err != nil || m.ProfileID == "" {
		return nil
	}
	return &m
}

// SetManaged records the provisioned configuration.
func (s *Store) SetManaged(m *Managed) error { return writeJSON(s.managedPath(), m) }

// ClearManaged leaves single-config mode.
func (s *Store) ClearManaged() error {
	err := os.Remove(s.managedPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ---- Settings -------------------------------------------------------------

// Settings are small per-user preferences.
type Settings struct {
	Language string `json:"language,omitempty"` // "", "en", "sk"
}

// Settings returns the stored settings.
func (s *Store) Settings() Settings {
	var st Settings
	readJSON(filepath.Join(s.dir, "settings.json"), &st)
	return st
}

// SaveSettings stores settings.
func (s *Store) SaveSettings(st Settings) error {
	return writeJSON(filepath.Join(s.dir, "settings.json"), st)
}

// ---- Files ----------------------------------------------------------------

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, b)
}

// writeFile replaces path atomically.
func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
