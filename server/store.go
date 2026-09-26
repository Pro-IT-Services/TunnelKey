package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Link struct {
	Title    string `json:"title"`
	Kind     string `json:"kind"` // web | rdp | app
	URL      string `json:"url,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
}

// Package is one provisionable configuration.
type Package struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	OVPN         string      `json:"ovpn"`
	Username     string      `json:"username"`
	Password     string      `json:"password,omitempty"`
	TOTP         *TOTPConfig `json:"totp,omitempty"`
	ManualCode   bool        `json:"manualCode"`
	CodePosition string      `json:"codePosition"` // after | before
	Links        []Link      `json:"links"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

// PackageSummary is what the list view gets; it holds no secrets.
type PackageSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Remote      string    `json:"remote"`
	HasTOTP     bool      `json:"hasTotp"`
	HasPassword bool      `json:"hasPassword"`
	LinkCount   int       `json:"linkCount"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Store struct {
	db   *sql.DB
	aead cipher.AEAD
}

// openStore opens (or creates) the database and the at-rest encryption key in dir.
func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := loadOrCreateKey(filepath.Join(dir, "secret.key"))
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)

	db, err := sql.Open("sqlite", filepath.Join(dir, "tunnelkey.db")+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS admins (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS packages (
	id TEXT PRIMARY KEY,
	summary TEXT NOT NULL,
	data BLOB NOT NULL,
	updated_at INTEGER NOT NULL
);`)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, aead: aead}, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("%s: expected 64 hex characters", path)
		}
		return key, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Store) seal(plain []byte) []byte {
	nonce := make([]byte, s.aead.NonceSize())
	rand.Read(nonce)
	return s.aead.Seal(nonce, nonce, plain, nil)
}

func (s *Store) open(sealed []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("ciphertext too short")
	}
	return s.aead.Open(nil, sealed[:n], sealed[n:], nil)
}

// ---- Admins & sessions ---------------------------------------------------

func (s *Store) adminCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func (s *Store) upsertAdmin(username, hash string) error {
	_, err := s.db.Exec(`INSERT INTO admins (username, password_hash, created_at) VALUES (?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`,
		username, hash, time.Now().Unix())
	return err
}

func (s *Store) adminByName(username string) (id int64, hash string, err error) {
	err = s.db.QueryRow(`SELECT id, password_hash FROM admins WHERE username = ?`, username).Scan(&id, &hash)
	return
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) createSession(adminID int64, ttl time.Duration) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	_, err := s.db.Exec(`INSERT INTO sessions (token_hash, admin_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), adminID, time.Now().Add(ttl).Unix())
	return token, err
}

func (s *Store) sessionAdmin(token string) (string, error) {
	var name string
	err := s.db.QueryRow(`SELECT a.username FROM sessions s JOIN admins a ON a.id = s.admin_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, hashToken(token), time.Now().Unix()).Scan(&name)
	return name, err
}

func (s *Store) deleteSession(token string) {
	s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
}

// ---- Packages ------------------------------------------------------------

func summarize(p *Package) PackageSummary {
	return PackageSummary{
		ID:          p.ID,
		Name:        p.Name,
		Remote:      firstRemote(p.OVPN),
		HasTOTP:     p.TOTP != nil,
		HasPassword: p.Password != "",
		LinkCount:   len(p.Links),
		UpdatedAt:   p.UpdatedAt,
	}
}

func firstRemote(ovpn string) string {
	for _, line := range strings.Split(ovpn, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "remote" {
			if len(f) >= 3 {
				return f[1] + ":" + f[2]
			}
			return f[1]
		}
	}
	return ""
}

func (s *Store) savePackage(p *Package) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	summary, _ := json.Marshal(summarize(p))
	_, err = s.db.Exec(`INSERT INTO packages (id, summary, data, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET summary = excluded.summary, data = excluded.data, updated_at = excluded.updated_at`,
		p.ID, string(summary), s.seal(data), p.UpdatedAt.Unix())
	return err
}

func (s *Store) getPackage(id string) (*Package, error) {
	var sealed []byte
	if err := s.db.QueryRow(`SELECT data FROM packages WHERE id = ?`, id).Scan(&sealed); err != nil {
		return nil, err
	}
	plain, err := s.open(sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypt package: %w", err)
	}
	var p Package
	return &p, json.Unmarshal(plain, &p)
}

func (s *Store) listPackages() ([]PackageSummary, error) {
	rows, err := s.db.Query(`SELECT summary FROM packages ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PackageSummary{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var ps PackageSummary
		if json.Unmarshal([]byte(raw), &ps) == nil {
			out = append(out, ps)
		}
	}
	return out, rows.Err()
}

func (s *Store) deletePackage(id string) error {
	_, err := s.db.Exec(`DELETE FROM packages WHERE id = ?`, id)
	return err
}
