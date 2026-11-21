// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestInstalledImage(t *testing.T) {
	if _, err := os.Stat("/app/livesync-portal"); err != nil {
		t.Skip("run scripts/image-smoke.sh")
	}
	if os.Getuid() != 65532 || os.Getgid() != 65532 {
		t.Fatal("wrong image UID/GID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := sampleConfig().People["alice"].Credentials
	input, _ := json.Marshal(c)
	cmd := exec.CommandContext(ctx, "/app/livesync-portal", "setup-uri")
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal("offline installed image native setup", err)
	}
	var result setupResult
	if json.Unmarshal(out, &result) != nil || result.Recovery != c.Recovery {
		t.Fatal("image native setup response")
	}
	if _, err := DecryptPersistentSetup(result.URI, result.Password); err != nil {
		t.Fatal("image URI decryption", err)
	}
	if _, err := os.Stat("/app/setup-uri"); !os.IsNotExist(err) {
		t.Fatal("obsolete runtime helper present")
	}
	if _, err := os.Stat("/etc/ssl/certs/ca-certificates.crt"); err != nil {
		t.Fatal("system CA roots missing", err)
	}
	ca, err := os.ReadFile("/etc/ssl/certs/ca-certificates.crt")
	if err != nil {
		t.Fatal(err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(ca) {
		t.Fatal("invalid system trust bundle")
	}
	if _, err := os.Stat("/etc/resolv.conf"); err != nil {
		t.Fatal("container DNS configuration missing", err)
	}
	if _, err := os.Stat("/app/licenses/ca-bundle/debian-copyright"); err != nil {
		t.Fatal("CA bundle package notice missing", err)
	}
	data, err := os.CreateTemp("/data", "smoke-*")
	if err != nil {
		t.Fatal("SQLite data volume not writable", err)
	}
	data.Close()
	defer os.Remove(data.Name())
	path := filepath.Join(t.TempDir(), "catalog.db")
	r, err := openSQLite(path, sampleConfig().People)
	if err != nil {
		t.Fatal("image SQLite", err)
	}
	defer r.Close()
	testInstalledStartup(t, "/app/livesync-portal")
}

func testInstalledStartup(t *testing.T, binary string) {
	t.Helper()
	issuer := ""
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	defer provider.Close()
	issuer = provider.URL
	c := sampleConfig()
	c.Issuer = issuer
	for id, p := range c.People {
		p.Issuer = issuer
		c.People[id] = p
	}
	dir := t.TempDir()
	c.SQLite = filepath.Join(dir, "app.db")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.Listen = l.Addr().String()
	l.Close()
	base := "http://" + c.Listen
	cfg, _ := json.Marshal(c)
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw})
	certfile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(certfile, ca, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary)
	cmd.Env = []string{"PORTAL_CONFIG=" + file, "SSL_CERT_FILE=" + certfile}
	var logs bytes.Buffer
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cmd.Process.Kill(); <-done }()
	h := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := h.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("startup health timeout", logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, path := range []string{"/", "/login", "/static/htmx.min.js"} {
		resp, err := h.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("cache restriction missing")
		}
	}
}
