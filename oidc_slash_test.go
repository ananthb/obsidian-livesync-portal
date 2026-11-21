// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
)

func TestOIDCDiscoveryAndLoginWithTrailingSlashes(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	for _, issuerPath := range []string{"/application/o/portal/", "/"} {
		t.Run(issuerPath, func(t *testing.T) {
			var issuer, base, nonce, challenge string
			var sawDiscovery, sawToken, sawKeys bool
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case strings.TrimSuffix(issuerPath, "/") + "/.well-known/openid-configuration":
					sawDiscovery = true
					json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": base + "/oauth/authorize/", "token_endpoint": base + "/oauth/token/", "jwks_uri": base + "/oauth/keys/", "id_token_signing_alg_values_supported": []string{"RS256"}})
				case "/oauth/authorize/":
					q := r.URL.Query()
					nonce = q.Get("nonce")
					challenge = q.Get("code_challenge")
					if q.Get("client_id") != "client" || q.Get("code_challenge_method") != "S256" || nonce == "" || challenge == "" {
						t.Error("missing authorization fields")
					}
					http.Redirect(w, r, q.Get("redirect_uri")+"?code=code&state="+url.QueryEscape(q.Get("state")), http.StatusSeeOther)
				case "/oauth/token/":
					sawToken = true
					r.ParseForm()
					sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
					if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.Form.Get("code") != "code" {
						t.Error("invalid exchange PKCE")
					}
					signed, e := jwt.Signed(signer).Claims(map[string]any{"iss": issuer, "sub": "alice-sub", "aud": "client", "exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce}).Serialize()
					if e != nil {
						t.Error(e)
					}
					json.NewEncoder(w).Encode(map[string]any{"access_token": "dummy", "token_type": "Bearer", "id_token": signed})
				case "/oauth/keys/":
					sawKeys = true
					json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Algorithm: "RS256", Use: "sig"}}})
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer provider.Close()
			base = provider.URL
			issuer = base + issuerPath
			h := provider.Client()
			h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			p, err := discoverProvider(context.Background(), h, issuer)
			if err != nil {
				t.Fatal(err)
			}
			a := testApp(t)
			a.Config.Issuer = issuer
			a.Config.People = a.Manager.People
			for id, person := range a.Config.People {
				person.Issuer = issuer
				a.Config.People[id] = person
			}
			if err = a.Config.validate(); err != nil {
				t.Fatal("valid slash issuer rejected", err)
			}
			a.HTTP = h
			a.OAuth = oauth2.Config{ClientID: "client", ClientSecret: "dummy", Endpoint: p.Endpoint(), RedirectURL: a.Config.URL + "/callback", Scopes: []string{"openid"}}
			a.Verifier = p.Verifier(&oidc.Config{ClientID: "client"})
			login := call(a, "/login", "GET", nil, "", false)
			response, err := h.Get(login.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			callback := httptest.NewRequest("GET", response.Header.Get("Location"), nil)
			for _, cookie := range login.Result().Cookies() {
				callback.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			a.ServeHTTP(w, callback)
			if w.Code != 303 || !sawDiscovery || !sawToken || !sawKeys {
				t.Fatal("full slash login failed", w.Code, w.Body.String())
			}
			found := false
			for _, session := range a.Sessions {
				if session.Issuer == issuer && session.Subject == "alice-sub" {
					found = true
				}
			}
			if !found {
				t.Fatal("issuer was normalized or lost")
			}
		})
	}
}

func TestEndpointAndOriginPolicies(t *testing.T) {
	for _, endpoint := range []string{"https://id.example.invalid/", "https://id.example.invalid/application/o/portal/", "https://id.example.invalid/token/?tenant=example"} {
		if !validEndpoint(endpoint, false) {
			t.Fatal(endpoint)
		}
	}
	for _, endpoint := range []string{"http://id.example.invalid/", "https://user:pass@id.example.invalid/token/", "https://id.example.invalid/token/#fragment"} {
		if validEndpoint(endpoint, false) {
			t.Fatal("unsafe endpoint", endpoint)
		}
	}
	if validIssuer("https://id.example.invalid/?query=issuer") || validOrigin("https://portal.example.invalid/") {
		t.Fatal("issuer/origin restrictions lost")
	}
	if !validOrigin("https://portal.example.invalid") || !validIssuer("https://id.example.invalid/application/o/portal/") {
		t.Fatal("valid origin/issuer denied")
	}
}

func TestDiscoveredEndpointRestrictions(t *testing.T) {
	for _, field := range []string{"authorization_endpoint", "token_endpoint", "jwks_uri"} {
		t.Run(field, func(t *testing.T) {
			var issuer string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				d := map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "authorize/", "token_endpoint": issuer + "token/", "jwks_uri": issuer + "keys/"}
				d[field] = "http://unsafe.example.invalid/"
				json.NewEncoder(w).Encode(d)
			}))
			defer server.Close()
			issuer = server.URL + "/"
			if _, err := discoverProvider(context.Background(), server.Client(), issuer); err == nil {
				t.Fatal("insecure discovery endpoint accepted")
			}
		})
	}
}
