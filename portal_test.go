// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
)

func sampleConfig() config {
	c := config{URL: "https://portal.example.invalid", Issuer: "https://id.example.invalid", ClientID: "client", ClientSecret: "dummy", People: map[string]person{}}
	for _, id := range []string{"alice", "bob"} {
		c.People[id] = person{Issuer: c.Issuer, Subject: id + "-sub", Credentials: credentials{Host: "https://" + id + ".example.invalid", ManagementURL: "http://" + id + ":5984", User: "server-owner", Password: "dummy", Database: "notes-" + id, Passphrase: "dummy-stable-key", Recovery: "sls-id-v1:" + strings.Repeat("ab", 32)}}
	}
	if err := c.validate(); err != nil {
		panic(err)
	}
	return c
}

type fakeDB struct {
	calls []string
	fail  int
}

func (d *fakeDB) Change(_ context.Context, method string, c credentials, db string) error {
	d.calls = append(d.calls, method+" "+db)
	if d.fail > 0 {
		d.fail--
		return errors.New("injected")
	}
	return nil
}

type failingRepo struct {
	repository
	saves, fail int
}

func (r *failingRepo) Save(ctx context.Context, p string, n int, c catalog) error {
	r.saves++
	if r.saves == r.fail {
		return errors.New("injected")
	}
	return r.repository.Save(ctx, p, n, c)
}
func fixture(t *testing.T) (*manager, *sqliteRepository, *fakeDB, string) {
	t.Helper()
	c := sampleConfig()
	path := filepath.Join(t.TempDir(), "portal.db")
	r, err := openSQLite(path, c.People)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	d := &fakeDB{}
	return &manager{Repo: r, DB: d, People: c.People}, r, d, path
}
func TestSQLiteCASOwnershipAndLock(t *testing.T) {
	m, r, _, path := fixture(t)
	ctx := context.Background()
	if _, err := openSQLite(path, m.People); err == nil {
		t.Fatal("second process accepted")
	}
	_, err := m.run(ctx, "alice", "list", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c, n, err := r.Load(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Save(ctx, "alice", n-1, c); !errors.Is(err, errConflict) {
		t.Fatal(err)
	}
	if err = r.Save(ctx, "bob", 0, c); err == nil {
		t.Fatal("cross ownership accepted")
	}
	c.Issuer = "https://foreign.invalid"
	if err = r.Save(ctx, "alice", n, c); err == nil {
		t.Fatal("issuer remap")
	}
}
func TestRestartCRUDAndConcurrentBootstrap(t *testing.T) {
	c := sampleConfig()
	path := filepath.Join(t.TempDir(), "portal.db")
	r, err := openSQLite(path, c.People)
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDB{}
	m := &manager{Repo: r, DB: d, People: c.People}
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.run(ctx, "alice", "list", "", ""); err != nil {
				t.Error(err)
			}
		}()
		_ = i
	}
	wg.Wait()
	if len(d.calls) != 1 {
		t.Fatal("non-idempotent default", d.calls)
	}
	v, err := m.run(ctx, "alice", "create", "", "Work")
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, x := range v {
		if x.ID != "default" {
			id = x.ID
		}
	}
	before, err := m.setup(ctx, "alice", id)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	r, err = openSQLite(path, c.People)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	m = &manager{Repo: r, DB: d, People: c.People}
	after, err := m.setup(ctx, "alice", id)
	if err != nil || before != after {
		t.Fatal("restart lost keys", err)
	}
	if _, err = m.setup(ctx, "bob", id); err == nil {
		t.Fatal("cross user setup")
	}
	if _, err = m.run(ctx, "bob", "delete", id, ""); err == nil {
		t.Fatal("cross user delete")
	}
	if _, err = m.run(ctx, "alice", "delete", "default", ""); err == nil {
		t.Fatal("default delete")
	}
	if _, err = m.run(ctx, "alice", "delete", id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = m.setup(ctx, "alice", id); err == nil {
		t.Fatal("deleted setup")
	}
	cat, _, _ := r.Load(ctx, "alice")
	if cat.Vaults[id].State != "deleted" || cat.Vaults[id].Passphrase != "" {
		t.Fatal("bad tombstone")
	}
	p := m.People["alice"]
	p.Subject = "replacement"
	m.People["alice"] = p
	if _, err = m.view(ctx, "alice"); err == nil {
		t.Fatal("remapping inherited catalog")
	}
}
func TestPartialFailureRecovery(t *testing.T) {
	for _, phase := range []string{"reserve", "database", "activate", "delete", "tombstone"} {
		t.Run(phase, func(t *testing.T) {
			m, r, d, _ := fixture(t)
			f := &failingRepo{repository: r}
			m.Repo = f
			ctx := context.Background()
			switch phase {
			case "reserve":
				f.fail = 1
			case "database":
				d.fail = 1
			case "activate":
				f.fail = 2
			}
			_, err := m.run(ctx, "alice", "list", "", "")
			id := "default"
			if phase == "delete" || phase == "tombstone" {
				vs, e := m.run(ctx, "alice", "create", "", "Work")
				if e != nil {
					t.Fatal(e)
				}
				for _, v := range vs {
					if v.ID != "default" {
						id = v.ID
					}
				}
				if phase == "delete" {
					d.fail = 1
				} else {
					f.fail = f.saves + 2
				}
				_, err = m.run(ctx, "alice", "delete", id, "")
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			f.fail = 0
			if _, err = m.run(ctx, "alice", "list", "", ""); err != nil {
				t.Fatal(err)
			}
			c, _, _ := r.Load(ctx, "alice")
			want := "active"
			if id != "default" {
				want = "deleted"
			}
			if c.Vaults[id].State != want {
				t.Fatal(c)
			}
		})
	}
}
func TestConcurrentCreatesAndReadOnlyView(t *testing.T) {
	m, _, d, _ := fixture(t)
	ctx := context.Background()
	if _, err := m.view(ctx, "alice"); err != nil || len(d.calls) != 0 {
		t.Fatal("view mutates")
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := m.run(ctx, "alice", "create", "", fmt.Sprint(i)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	vs, err := m.view(ctx, "alice")
	if err != nil || len(vs) != 21 || len(d.calls) != 21 {
		t.Fatal("lost create", err)
	}
	if err = m.lock(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.unlock()
	cx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	if _, err = m.view(cx, "alice"); err == nil {
		t.Fatal("ignored timeout")
	}
}
func testApp(t *testing.T) *app {
	m, _, _, _ := fixture(t)
	return &app{Config: sampleConfig(), Manager: m, Sessions: map[string]session{"valid": {CSRF: "csrf", Expires: time.Now().Add(time.Hour), Person: "alice", Subject: "alice-sub", Issuer: sampleConfig().Issuer}}, Logins: map[string]login{}, Templates: template.Must(template.ParseFS(assets, "templates/*")), Generate: func(_ context.Context, c credentials) (setupResult, error) {
		return setupResult{URI: "obsidian://setuplivesync?dummy", Password: "dummy-uri-key", Recovery: c.Recovery}, nil
	}}
}
func call(a *app, path, method string, form url.Values, origin string, authenticated bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, a.Config.URL+path, strings.NewReader(form.Encode()))
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if authenticated {
		r.AddCookie(&http.Cookie{Name: "__Host-obsidian", Value: "valid"})
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	return w
}
func TestHTTPBoundaries(t *testing.T) {
	a := testApp(t)
	origin := a.Config.URL
	if w := call(a, "/", "POST", nil, origin, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, o := range []string{"", "https://foreign.invalid"} {
		if w := call(a, "/", "POST", url.Values{"csrf": {"csrf"}}, o, true); w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	if w := call(a, "/?person=bob", "GET", nil, "", true); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, action := range []string{"setup", "create", "delete", "list"} {
		f := url.Values{"person": {"bob"}, "action": {action}, "csrf": {"csrf"}}
		if w := call(a, "/", "POST", f, origin, true); w.Code != 403 {
			t.Fatal(action, w.Code)
		}
	}
	_, _ = a.Manager.run(context.Background(), "alice", "list", "", "")
	f := url.Values{"csrf": {"csrf"}, "action": {"setup"}, "id": {"default"}}
	w := call(a, "/", "POST", f, origin, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "dummy-uri-key") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body)
	}
	f.Set("action", "delete")
	f.Set("confirm", "DELETE")
	if w = call(a, "/", "POST", f, origin, true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = call(a, "/admin", "GET", nil, "", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	p := a.Config.People["alice"]
	p.Issuer = "https://replacement.invalid"
	a.Config.People["alice"] = p
	if w = call(a, "/", "GET", nil, "", true); w.Code != 403 {
		t.Fatal("session identity remap")
	}
}
func TestAdmissionAndConfiguration(t *testing.T) {
	c := sampleConfig()
	if !c.admit(map[string]any{}) {
		t.Fatal("optional groups required")
	}
	c.Groups = []string{"editors", "users"}
	c.GroupsClaim = "roles"
	c.GroupMatch = "any"
	if !c.admit(map[string]any{"roles": "editors"}) || c.admit(map[string]any{"roles": []any{true}}) {
		t.Fatal("scalar/invalid groups")
	}
	c.GroupMatch = "all"
	if c.admit(map[string]any{"roles": []any{"editors"}}) || !c.admit(map[string]any{"roles": []any{"editors", "users"}}) {
		t.Fatal("all groups")
	}
	c.Groups = nil
	c.RequireVerifiedEmail = true
	if c.admit(map[string]any{"email_verified": "true"}) || !c.admit(map[string]any{"email_verified": true}) {
		t.Fatal("verified claim")
	}
	c.RequireVerifiedEmail = false
	c.Claims = map[string]string{"tenant": "allowed"}
	if c.admit(map[string]any{}) || !c.admit(map[string]any{"tenant": "allowed"}) {
		t.Fatal("claim policy")
	}
	a := testApp(t)
	if _, ok := a.identity(a.Config.Issuer, "alice-sub"); !ok {
		t.Fatal("stable identity")
	}
	if _, ok := a.identity("https://foreign.invalid", "alice-sub"); ok {
		t.Fatal("issuer ignored")
	}
	for _, s := range []string{"http://portal.example.invalid", "https://user:pass@portal.invalid", "https://portal.invalid/", "https://portal.invalid/?x=1"} {
		c := sampleConfig()
		c.URL = s
		if c.validate() == nil {
			t.Fatal("invalid external url", s)
		}
	}
	c = sampleConfig()
	p := c.People["alice"]
	p.Credentials.Database = "../_users"
	c.People["alice"] = p
	if c.validate() == nil {
		t.Fatal("unsafe database")
	}
}
func TestOIDCMock(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "nonce", "subject", "issuer", "audience", "expired", "state", "signature", "groups", "verified", "optional-groups", "string-groups"} {
		t.Run(bad, func(t *testing.T) {
			a := testApp(t)
			a.Config.Groups = []string{"members"}
			a.Config.GroupsClaim = "groups"
			a.Config.RequireVerifiedEmail = true
			claims := map[string]any{"iss": a.Config.Issuer, "sub": "alice-sub", "aud": "client", "exp": time.Now().Add(time.Hour).Unix(), "nonce": "nonce", "groups": []string{"members"}, "email_verified": true}
			switch bad {
			case "nonce":
				claims["nonce"] = "wrong"
			case "subject":
				claims["sub"] = "foreign"
			case "issuer":
				claims["iss"] = "https://evil.invalid"
			case "audience":
				claims["aud"] = "foreign"
			case "expired":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			case "groups":
				claims["groups"] = []string{"foreign"}
			case "verified":
				claims["email_verified"] = false
			case "optional-groups":
				a.Config.Groups = nil
				a.Config.RequireVerifiedEmail = false
				delete(claims, "groups")
				delete(claims, "email_verified")
			case "string-groups":
				claims["groups"] = "members"
			}
			signed, err := jwt.Signed(signer).Claims(claims).Serialize()
			if err != nil {
				t.Fatal(err)
			}
			if bad == "signature" {
				parts := strings.Split(signed, ".")
				parts[2] = strings.Repeat("A", len(parts[2]))
				signed = strings.Join(parts, ".")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/keys" {
					json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Algorithm: "RS256", Use: "sig"}}})
					return
				}
				r.ParseForm()
				if r.Form.Get("code_verifier") != "verifier" {
					t.Error("missing PKCE")
				}
				json.NewEncoder(w).Encode(map[string]any{"access_token": "dummy", "token_type": "Bearer", "id_token": signed})
			}))
			defer server.Close()
			a.HTTP = server.Client()
			a.Verifier = oidc.NewVerifier(a.Config.Issuer, oidc.NewRemoteKeySet(context.Background(), server.URL+"/keys"), &oidc.Config{ClientID: "client"})
			a.OAuth = oauth2.Config{ClientID: "client", ClientSecret: "dummy", Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token"}, RedirectURL: a.Config.URL + "/callback"}
			a.Logins["state"] = login{Nonce: "nonce", Verifier: "verifier", Expires: time.Now().Add(time.Minute)}
			run := func() *httptest.ResponseRecorder {
				state := "state"
				if bad == "state" {
					state = "wrong"
				}
				r := httptest.NewRequest("GET", a.Config.URL+"/callback?state="+state+"&code=dummy", nil)
				r.AddCookie(&http.Cookie{Name: "__Host-obsidian-login", Value: "state"})
				w := httptest.NewRecorder()
				a.ServeHTTP(w, r)
				return w
			}
			w := run()
			want := bad == "" || bad == "optional-groups" || bad == "string-groups"
			if (w.Code == 303) != want {
				t.Fatal(w.Code, w.Body)
			}
			if want {
				secure := false
				for _, c := range w.Result().Cookies() {
					if c.Name == "__Host-obsidian" {
						secure = c.Secure && c.HttpOnly && c.SameSite == http.SameSiteLaxMode && c.Path == "/"
					}
				}
				if !secure {
					t.Fatal("insecure cookie")
				}
				if run().Code != 400 {
					t.Fatal("state replay")
				}
			}
		})
	}
}
func TestNativeApplicationSetup(t *testing.T) {
	c := sampleConfig().People["alice"].Credentials
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for range 2 {
		r, err := generate(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := DecryptPersistentSetup(r.URI, r.Password)
		if err != nil || raw["couchDB_PASSWORD"] != c.Password || raw["passphrase"] != c.Passphrase || r.Recovery != c.Recovery {
			t.Fatal("native app setup contract", err)
		}
	}
}
func TestVendoredHTMX(t *testing.T) {
	b, _ := assets.ReadFile("static/htmx.min.js")
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "fd25ba3fe5e44768756f23862a6daa4f928192199a121e825e882e719bc8aabd" {
		t.Fatal("htmx 4.0.2 pin changed")
	}
}

func TestLoginChallenge(t *testing.T) {
	a := testApp(t)
	a.OAuth = oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{AuthURL: "https://id.example.invalid/authorize"}, RedirectURL: a.Config.URL + "/callback", Scopes: []string{"openid"}}
	w := call(a, "/login", "GET", nil, "", false)
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatal("missing PKCE/nonce/state")
	}
	pending := a.Logins[q.Get("state")]
	if pending.Nonce != q.Get("nonce") || pending.Verifier == "" {
		t.Fatal("unbound login")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestConfiguredCouchDB(t *testing.T) {
	c := sampleConfig().People["alice"].Credentials
	for _, method := range []string{"PUT", "DELETE"} {
		for _, status := range []int{200, 201, 202, 404, 412, 401, 500} {
			h := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != c.ManagementURL+"/"+c.Database {
					t.Fatal(r.URL)
				}
				u, p, _ := r.BasicAuth()
				if u != c.User || p != c.Password {
					t.Fatal("credentials")
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			err := (couchDB{h}).Change(context.Background(), method, c, c.Database)
			want := method == "PUT" && (status == 201 || status == 202 || status == 412) || method == "DELETE" && (status == 200 || status == 202 || status == 404)
			if (err == nil) != want {
				t.Fatal(method, status, err)
			}
		}
	}
	if err := (couchDB{}).Change(context.Background(), "PUT", c, "../_users"); err == nil {
		t.Fatal("unsafe URL escaped")
	}
}
