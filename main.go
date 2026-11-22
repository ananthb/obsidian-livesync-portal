// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"crypto/subtle"
	"embed"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

//go:embed templates/* static/*
var assets embed.FS

var sourceVersion = "development"
var sourceRevision = ""

type session struct {
	CSRF                    string
	Expires                 time.Time
	Person, Subject, Issuer string
}
type login struct {
	Nonce, Verifier string
	Expires         time.Time
}
type app struct {
	Config    config
	Manager   *manager
	OAuth     oauth2.Config
	Verifier  *oidc.IDTokenVerifier
	HTTP      *http.Client
	mu        sync.Mutex
	Sessions  map[string]session
	Logins    map[string]login
	Templates *template.Template
	Generate  func(context.Context, credentials) (setupResult, error)
}
type setupResult struct {
	URI      string `json:"setupURI"`
	Password string `json:"setupPassphrase"`
	Recovery string `json:"idRecoveryCode"`
}

func generate(ctx context.Context, c credentials) (setupResult, error) {
	return GenerateNativeSetup(ctx, c)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) cookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: age, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}
func equal(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (a *app) authenticated(r *http.Request) (session, bool) {
	c, err := r.Cookie("__Host-obsidian")
	if err != nil {
		return session{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.Sessions[c.Value]
	if !ok || time.Now().After(s.Expires) {
		delete(a.Sessions, c.Value)
		return session{}, false
	}
	return s, true
}
func (a *app) cleanup() {
	now := time.Now()
	for k, v := range a.Logins {
		if now.After(v.Expires) {
			delete(a.Logins, k)
		}
	}
	for k, v := range a.Sessions {
		if now.After(v.Expires) {
			delete(a.Sessions, k)
		}
	}
}
func (a *app) startLogin(w http.ResponseWriter, r *http.Request) {
	state := secret(32)
	v := login{Nonce: secret(32), Verifier: oauth2.GenerateVerifier(), Expires: time.Now().Add(5 * time.Minute)}
	a.mu.Lock()
	a.cleanup()
	if len(a.Logins) >= 512 {
		a.mu.Unlock()
		http.Error(w, "login capacity reached", 503)
		return
	}
	a.Logins[state] = v
	a.mu.Unlock()
	a.cookie(w, "__Host-obsidian-login", state, 300)
	http.Redirect(w, r, a.OAuth.AuthCodeURL(state, oidc.Nonce(v.Nonce), oauth2.S256ChallengeOption(v.Verifier)), http.StatusSeeOther)
}
func (a *app) callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	c, err := r.Cookie("__Host-obsidian-login")
	if err != nil || !equal(state, c.Value) {
		http.Error(w, "invalid login state", 400)
		return
	}
	a.mu.Lock()
	pending, ok := a.Logins[state]
	delete(a.Logins, state)
	a.mu.Unlock()
	a.cookie(w, "__Host-obsidian-login", "", -1)
	if !ok || time.Now().After(pending.Expires) || r.URL.Query().Get("code") == "" {
		http.Error(w, "login expired", 400)
		return
	}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(r.Context(), a.HTTP), 20*time.Second)
	defer cancel()
	token, err := a.OAuth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(pending.Verifier))
	if err != nil {
		http.Error(w, "login failed", 403)
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing identity", 403)
		return
	}
	id, err := a.Verifier.Verify(ctx, raw)
	if err != nil {
		http.Error(w, "invalid identity", 403)
		return
	}
	var claims map[string]any
	if id.Claims(&claims) != nil || claims["nonce"] != pending.Nonce || !a.Config.admit(claims) {
		http.Error(w, "identity claim policy denied", 403)
		return
	}
	personName, ok := a.identity(id.Issuer, id.Subject)
	if !ok {
		http.Error(w, "unrecognized directory identity", 403)
		return
	}
	// The verified login round-trip authorizes bootstrap; ordinary GETs never mutate data.
	if _, err := a.Manager.run(ctx, personName, "list", "", ""); err != nil {
		log.Print("default initialization pending; automatic retry on management page")
	}
	a.mu.Lock()
	a.cleanup()
	if len(a.Sessions) >= 512 {
		a.mu.Unlock()
		http.Error(w, "session capacity reached", 503)
		return
	}
	key := secret(32)
	a.Sessions[key] = session{CSRF: secret(32), Expires: time.Now().Add(time.Hour), Person: personName, Subject: id.Subject, Issuer: id.Issuer}
	a.mu.Unlock()
	if old, err := r.Cookie("__Host-obsidian"); err == nil {
		a.mu.Lock()
		delete(a.Sessions, old.Value)
		a.mu.Unlock()
	}
	a.cookie(w, "__Host-obsidian", key, 3600)
	http.Redirect(w, r, "/", 303)
}
func (a *app) identity(iss, sub string) (string, bool) {
	if iss != a.Config.Issuer || sub == "" {
		return "", false
	}
	for name, p := range a.Config.People {
		if sub == p.Subject && iss == p.Issuer {
			return name, true
		}
	}
	return "", false
}
func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Native form POSTs need a same-origin referrer policy to retain their Origin
	// header in Chromium. External destinations still receive no referrer.
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		w.WriteHeader(200)
		return
	}
	if r.URL.Path == "/static/htmx.min.js" && r.Method == "GET" {
		b, _ := assets.ReadFile("static/htmx.min.js")
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write(b)
		return
	}
	if r.URL.Path == "/static/style.css" && r.Method == "GET" {
		b, _ := assets.ReadFile("static/style.css")
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write(b)
		return
	}
	if r.URL.Path == "/static/portal.js" && r.Method == "GET" {
		b, _ := assets.ReadFile("static/portal.js")
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write(b)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/login" {
		a.startLogin(w, r)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/callback" {
		a.callback(w, r)
		return
	}
	s, ok := a.authenticated(r)
	if !ok {
		if r.Method == "GET" {
			http.Redirect(w, r, "/login", 303)
		} else {
			a.requestError(w, r, "Your session expired. Sign in again to continue.", 401)
		}
		return
	}
	identity, known := a.Config.People[s.Person]
	if !known || identity.Subject != s.Subject || identity.Issuer != s.Issuer {
		a.requestError(w, r, "Directory access removed.", 403)
		return
	}
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if r.Header.Get("Origin") != a.Config.URL || r.ParseForm() != nil || !equal(r.PostForm.Get("csrf"), s.CSRF) {
			a.requestError(w, r, "Invalid CSRF token or origin. Reload the page and try again.", 403)
			return
		}
		if r.URL.Path == "/logout" {
			c, _ := r.Cookie("__Host-obsidian")
			a.mu.Lock()
			delete(a.Sessions, c.Value)
			a.mu.Unlock()
			a.cookie(w, "__Host-obsidian", "", -1)
			http.Redirect(w, r, "/", 303)
			return
		}
	} else if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	p := r.URL.Query().Get("person")
	if r.Method == "POST" {
		p = r.PostForm.Get("person")
	}
	if p == "" {
		p = s.Person
	}
	if p != s.Person {
		a.requestError(w, r, "Another person's vaults are not accessible.", 403)
		return
	}
	if _, ok := a.Config.People[p]; !ok {
		http.Error(w, "unknown person", 404)
		return
	}
	timeout := 70 * time.Second
	if r.Method == "POST" && r.PostForm.Get("action") == "sync" {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	action := "list"
	if r.Method == "POST" {
		action = r.PostForm.Get("action")
	}
	data := struct {
		Person, CSRF, Error string
		Vaults              []vault
		Setup               *setupResult
		Base                string
		SourceURL           string
		Retry               bool
		VaultName           string
	}{Person: p, CSRF: s.CSRF, Base: r.URL.Path, SourceURL: a.Config.SourceURL}
	if action == "setup" {
		if vaults, err := a.Manager.view(ctx, p); err == nil {
			for _, v := range vaults {
				if v.ID == r.PostForm.Get("id") {
					data.VaultName = v.Name
				}
			}
		}
		cred, err := a.Manager.setup(ctx, p, r.PostForm.Get("id"))
		if err != nil {
			data.Error = err.Error()
		} else {
			result, err := a.Generate(ctx, cred)
			if err != nil {
				data.Error = "Setup generation failed. Retry later."
			} else {
				data.Setup = &result
			}
		}
	} else {
		if action != "list" && action != "sync" && action != "create" && action != "delete" {
			http.Error(w, "unknown action", 400)
			return
		}
		if action == "delete" && r.PostForm.Get("confirm") != "DELETE" {
			http.Error(w, "type DELETE to confirm", 400)
			return
		}
		if action == "delete" && r.PostForm.Get("id") == "default" {
			http.Error(w, "the default vault cannot be deleted", 400)
			return
		}
		var err error
		if r.Method == "GET" {
			data.Vaults, err = a.Manager.view(ctx, p)
		} else {
			if action == "sync" {
				action = "list"
			}
			data.Vaults, err = a.Manager.run(ctx, p, action, r.PostForm.Get("id"), r.PostForm.Get("name"))
		}
		if err != nil {
			data.Error = err.Error()
			// Return persisted pending state, not an empty list after a partial failure.
			data.Vaults, _ = a.Manager.view(ctx, p)
		}
	}
	data.Retry = action != "setup" && (data.Error != "" || len(data.Vaults) == 0)
	for _, v := range data.Vaults {
		if v.State == "creating" || v.State == "deleting" {
			data.Retry = true
		}
	}
	name := "page"
	if r.Header.Get("HX-Request") == "true" {
		name = "content"
		if action == "setup" {
			name = "setup"
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Templates.ExecuteTemplate(w, name, data); err != nil {
		log.Print("template render failed")
	}
}

// Enhanced setup errors stay inside the credential dialog, including expired sessions
// and rejected CSRF checks. No untrusted request text is reflected into the fragment.
func (a *app) requestError(w http.ResponseWriter, r *http.Request, message string, status int) {
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "div#setup-content" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_ = a.Templates.ExecuteTemplate(w, "setup", struct {
			Error     string
			Setup     *setupResult
			VaultName string
		}{Error: message})
		return
	}
	http.Error(w, message, status)
}
