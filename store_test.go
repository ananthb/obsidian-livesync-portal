// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultConfigurableTokenKubernetesAndCAS(t *testing.T) {
	for _, mode := range []string{"token", "kubernetes"} {
		t.Run(mode, func(t *testing.T) {
			logins := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/auth/ordinary-k8s/login" {
					logins++
					var body map[string]string
					json.NewDecoder(r.Body).Decode(&body)
					if body["role"] != "portal-role" || body["jwt"] != "dummy-jwt" {
						t.Error(body)
					}
					io.WriteString(w, `{"auth":{"client_token":"dummy-token","lease_duration":900}}`)
					return
				}
				if r.Header.Get("X-Vault-Token") != "dummy-token" {
					t.Error("missing token")
				}
				if !strings.HasPrefix(r.URL.Path, "/v1/custom-kv/data/team/catalogs/") {
					t.Error("wrong configurable path", r.URL)
				}
				if strings.HasSuffix(r.URL.Path, "missing") {
					w.WriteHeader(404)
					return
				}
				if r.Method == "GET" {
					io.WriteString(w, `{"data":{"data":{"test":"value"},"metadata":{"version":7}}}`)
					return
				}
				var body struct {
					Options struct {
						CAS int `json:"cas"`
					} `json:"options"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				if body.Options.CAS != 7 {
					w.WriteHeader(400)
					return
				}
				io.WriteString(w, `{}`)
			}))
			defer server.Close()
			v := &vaultClient{URL: server.URL, Mount: "custom-kv", Path: "team/catalogs", AuthMount: "ordinary-k8s", Role: "portal-role", HTTP: server.Client()}
			if mode == "token" {
				v.Token = "dummy-token"
			} else {
				v.JWTFile = filepath.Join(t.TempDir(), "jwt")
				os.WriteFile(v.JWTFile, []byte("dummy-jwt"), 0600)
			}
			ctx := context.Background()
			var data map[string]string
			n, err := v.read(ctx, "team/catalogs/alice", &data)
			if n != 7 || err != nil || data["test"] != "value" {
				t.Fatal(n, err)
			}
			if err = v.write(ctx, "team/catalogs/alice", 7, data); err != nil {
				t.Fatal(err)
			}
			if err = v.write(ctx, "team/catalogs/alice", 6, data); err != errConflict {
				t.Fatal(err)
			}
			if n, err = v.read(ctx, "team/catalogs/missing", &data); n != 0 || err != nil {
				t.Fatal(n, err)
			}
			want := 0
			if mode == "kubernetes" {
				want = 1
			}
			if logins != want {
				t.Fatal("token cache", logins)
			}
			repo := vaultRepository{vaultClient: v, People: sampleConfig().People}
			if _, _, err = repo.Load(ctx, "../alice"); err == nil {
				t.Fatal("unsafe user path")
			}
			if err = repo.Save(ctx, "bob", 0, catalog{Person: "alice"}); err == nil {
				t.Fatal("cross-user save")
			}
		})
	}
}

func TestSQLiteConfigurationPersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal.db")
	c := sampleConfig()
	r, err := openSQLite(path, c.People)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(c)
	if _, err = r.db.Exec("INSERT INTO configuration(id,body) VALUES(1,?)", body); err != nil {
		t.Fatal(err)
	}
	r.Close()
	got, err := loadConfig("sqlite:" + path)
	if err != nil || got.SQLite != path || got.People["alice"].Credentials.Password != "dummy" {
		t.Fatal("durable configuration", err)
	}
	file := filepath.Join(dir, "config.json")
	os.WriteFile(file, append(body, []byte(" {}")...), 0600)
	if _, err = loadConfig(file); err == nil {
		t.Fatal("trailing JSON")
	}
	os.WriteFile(file, []byte(`{"unexpected":true}`), 0600)
	if _, err = loadConfig(file); err == nil {
		t.Fatal("unknown configuration")
	}
}
