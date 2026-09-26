// Command tunnelkey-server is the admin web UI that turns an .ovpn profile,
// credentials, a TOTP secret and a list of links into setup QR codes for the
// Tunnelkey mobile apps.
package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "admin" {
		adminCommand(os.Args[2:])
		return
	}

	listen := flag.String("listen", "127.0.0.1:8080", "address to listen on")
	dataDir := flag.String("data", "data", "directory for the database and encryption key")
	flag.Parse()

	store, err := openStore(*dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	bootstrapAdmin(store)

	static, _ := fs.Sub(webFS, "web")
	srv := &Server{store: store, limiter: &loginLimiter{failures: map[string][]time.Time{}}}
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           srv.routes(http.FileServerFS(static)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	log.Printf("Tunnelkey server listening on http://%s (data in %s)", *listen, *dataDir)
	log.Fatal(httpServer.ListenAndServe())
}

// bootstrapAdmin creates the first admin from TUNNELKEY_ADMIN_USER /
// TUNNELKEY_ADMIN_PASSWORD when the database has none.
func bootstrapAdmin(store *Store) {
	n, err := store.adminCount()
	if err != nil || n > 0 {
		return
	}
	user, pass := os.Getenv("TUNNELKEY_ADMIN_USER"), os.Getenv("TUNNELKEY_ADMIN_PASSWORD")
	if user == "" || pass == "" {
		log.Printf("No admin account yet. Create one with:  tunnelkey-server admin <username>")
		return
	}
	if len(pass) < 12 {
		log.Fatalf("TUNNELKEY_ADMIN_PASSWORD must be at least 12 characters")
	}
	hash, _ := hashPassword(pass)
	if err := store.upsertAdmin(user, hash); err != nil {
		log.Fatalf("create admin: %v", err)
	}
	log.Printf("Created admin %q", user)
}

// adminCommand: tunnelkey-server admin [-data dir] <username>
// Creates the admin or resets its password. The password is read from stdin.
func adminCommand(args []string) {
	fset := flag.NewFlagSet("admin", flag.ExitOnError)
	dataDir := fset.String("data", "data", "directory for the database and encryption key")
	fset.Parse(args)
	if fset.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: tunnelkey-server admin [-data dir] <username>")
		os.Exit(2)
	}
	store, err := openStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprint(os.Stderr, "Password (min. 12 characters): ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	pass := strings.TrimRight(line, "\r\n")
	if len(pass) < 12 {
		log.Fatal("password must be at least 12 characters")
	}
	hash, _ := hashPassword(pass)
	if err := store.upsertAdmin(fset.Arg(0), hash); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "Admin %q saved.\n", fset.Arg(0))
}
