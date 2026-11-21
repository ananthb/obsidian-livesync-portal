// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type config struct {
	SourceURL            string            `json:"source_url,omitempty"`
	URL                  string            `json:"url"`
	Issuer               string            `json:"issuer"`
	ClientID             string            `json:"client_id"`
	ClientSecret         string            `json:"client_secret"`
	Scopes               []string          `json:"scopes"`
	GroupsClaim          string            `json:"groups_claim"`
	Groups               []string          `json:"groups"`
	GroupMatch           string            `json:"group_match"`
	RequireVerifiedEmail bool              `json:"require_verified_email"`
	Claims               map[string]string `json:"required_claims"`
	People               map[string]person `json:"people"`
	Listen               string            `json:"listen"`
	SQLite               string            `json:"sqlite"`
	Vault                *vaultConfig      `json:"vault,omitempty"`
}
type vaultConfig struct {
	URL       string `json:"url"`
	Mount     string `json:"mount"`
	Path      string `json:"path"`
	TokenFile string `json:"token_file"`
	JWTFile   string `json:"jwt_file"`
	AuthMount string `json:"auth_mount"`
	Role      string `json:"role"`
}

var safeDatabase = regexp.MustCompile(`^[a-z][a-z0-9-]{0,180}$`)
var safeVaultPath = regexp.MustCompile(`^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$`)

func validEndpoint(s string, internal bool) bool {
	u, err := url.Parse(s)
	return err == nil && u.Hostname() != "" && (u.Scheme == "https" || internal && u.Scheme == "http") && u.User == nil && !strings.Contains(s, "#") && !strings.Contains(u.Path, "..")
}
func validOrigin(s string) bool {
	u, err := url.Parse(s)
	return err == nil && validEndpoint(s, false) && u.Path == "" && u.RawQuery == "" && !u.ForceQuery
}
func validIssuer(s string) bool {
	u, err := url.Parse(s)
	return err == nil && validEndpoint(s, false) && u.RawQuery == "" && !u.ForceQuery
}
func validBaseEndpoint(s string, internal bool) bool {
	u, err := url.Parse(s)
	return err == nil && validEndpoint(s, internal) && u.RawQuery == "" && !u.ForceQuery && !strings.HasSuffix(s, "/")
}
func validCredentials(c credentials) bool {
	return validBaseEndpoint(c.Host, false) && validBaseEndpoint(c.ManagementURL, true) && safeDatabase.MatchString(c.Database) && c.User != "" && c.Password != "" && c.Passphrase != "" && safeRecovery.MatchString(c.Recovery)
}
func (c *config) validate() error {
	bad := errors.New("invalid configuration; see configuration reference")
	if c.SourceURL == "" {
		c.SourceURL = releaseSourceURL(sourceVersion, sourceRevision)
	}
	if c.SourceURL != "" && !validEndpoint(c.SourceURL, false) {
		return bad
	}
	if !validOrigin(c.URL) || !validIssuer(c.Issuer) || c.ClientID == "" || c.ClientSecret == "" {
		return bad
	}
	if len(c.Scopes) == 0 {
		c.Scopes = []string{"openid", "profile"}
	}
	if !slices.Contains(c.Scopes, "openid") {
		return bad
	}
	if c.GroupMatch == "" {
		c.GroupMatch = "any"
	}
	if c.GroupMatch != "any" && c.GroupMatch != "all" {
		return bad
	}
	if c.GroupsClaim == "" {
		c.GroupsClaim = "groups"
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.SQLite == "" {
		c.SQLite = "data/portal.db"
	}
	if len(c.People) == 0 {
		return bad
	}
	seen := map[string]bool{}
	servers := map[string]bool{}
	for id, p := range c.People {
		key := p.Issuer + "\x00" + p.Subject
		if !safePerson.MatchString(id) || p.Issuer != c.Issuer || p.Subject == "" || seen[key] || !validCredentials(p.Credentials) {
			return bad
		}
		// Each person gets a dedicated server, not a namespace on a shared admin server.
		if servers[p.Credentials.ManagementURL] {
			return bad
		}
		seen[key] = true
		servers[p.Credentials.ManagementURL] = true
	}
	if v := c.Vault; v != nil {
		if !validBaseEndpoint(v.URL, true) || !safeVaultPath.MatchString(v.Mount) || !safeVaultPath.MatchString(v.Path) {
			return bad
		}
		if (v.TokenFile == "") == (v.JWTFile == "") {
			return bad
		}
		if v.JWTFile != "" && (!safeVaultPath.MatchString(v.AuthMount) || v.Role == "") {
			return bad
		}
	}
	return nil
}

func releaseSourceURL(version, revision string) string {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(revision) || !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`).MatchString(version) || strings.Contains(version, "SNAPSHOT") {
		return ""
	}
	return "https://github.com/ananthb/obsidian-livesync-portal/releases/download/source-" + revision + "/obsidian-livesync-portal_" + version + "_source.tar.gz"
}
func (c config) admit(claims map[string]any) bool {
	if c.RequireVerifiedEmail && claims["email_verified"] != true {
		return false
	}
	for key, want := range c.Claims {
		if claims[key] != want {
			return false
		}
	}
	if len(c.Groups) == 0 {
		return true
	}
	groups := []string{}
	switch v := claims[c.GroupsClaim].(type) {
	case string:
		groups = []string{v}
	case []any:
		for _, x := range v {
			s, ok := x.(string)
			if !ok {
				return false
			}
			groups = append(groups, s)
		}
	default:
		return false
	}
	matches := 0
	for _, g := range c.Groups {
		if slices.Contains(groups, g) {
			matches++
		}
	}
	return c.GroupMatch == "all" && matches == len(c.Groups) || c.GroupMatch != "all" && matches > 0
}
func loadConfig(path string) (config, error) {
	if strings.HasPrefix(path, "sqlite:") {
		path = strings.TrimPrefix(path, "sqlite:")
		r, err := openSQLite(path, nil)
		if err != nil {
			return config{}, err
		}
		defer r.Close()
		var c config
		var body []byte
		if r.db.QueryRow("SELECT body FROM configuration WHERE id=1").Scan(&body) != nil || json.Unmarshal(body, &c) != nil {
			return c, errors.New("initialize SQLite configuration with configure first")
		}
		c.SQLite = path
		return c, c.validate()
	}
	f, err := os.Open(path)
	if err != nil {
		return config{}, errors.New("cannot open configuration")
	}
	defer f.Close()
	d := json.NewDecoder(http.MaxBytesReader(nil, f, 2<<20))
	d.DisallowUnknownFields()
	var c config
	if d.Decode(&c) != nil {
		return c, errors.New("cannot decode configuration")
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("trailing configuration data")
	}
	return c, c.validate()
}
func run() error {
	if len(os.Args) == 3 && os.Args[1] == "configure" {
		c, err := loadConfig(os.Args[2])
		if err != nil {
			return err
		}
		path := os.Getenv("PORTAL_DB")
		if path == "" {
			path = "data/portal.db"
		}
		c.SQLite = path
		r, err := openSQLite(path, c.People)
		if err != nil {
			return err
		}
		defer r.Close()
		body, err := json.Marshal(c)
		if err != nil {
			return err
		}
		_, err = r.db.Exec("INSERT INTO configuration(id,body) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", body)
		return err
	}
	if len(os.Args) == 2 && os.Args[1] == "setup-uri" {
		var c credentials
		if json.NewDecoder(io.LimitReader(os.Stdin, 16385)).Decode(&c) != nil || !validCredentials(c) {
			return errors.New("invalid setup credentials")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := generate(ctx, c)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://127.0.0.1:8080/healthz")
		if err != nil {
			return errors.New("unhealthy")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return errors.New("unhealthy")
		}
		return nil
	}
	path := os.Getenv("PORTAL_CONFIG")
	if path == "" {
		path = "sqlite:data/portal.db"
	}
	c, err := loadConfig(path)
	if err != nil {
		return err
	}
	// A persistent local lock protects side effects, even when Vault stores catalogs.
	r, err := openSQLite(c.SQLite, c.People)
	if err != nil {
		return err
	}
	defer r.Close()
	var repo repository = r
	h := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if v := c.Vault; v != nil {
		vc := &vaultClient{URL: v.URL, Mount: v.Mount, Path: v.Path, AuthMount: v.AuthMount, Role: v.Role, JWTFile: v.JWTFile, HTTP: h}
		if v.TokenFile != "" {
			b, e := os.ReadFile(v.TokenFile)
			if e != nil {
				return errors.New("cannot read Vault token")
			}
			vc.Token = strings.TrimSpace(string(b))
			if vc.Token == "" {
				return errors.New("empty Vault token")
			}
		}
		repo = vaultRepository{vaultClient: vc, People: c.People}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, err := discoverProvider(ctx, h, c.Issuer)
	if err != nil {
		return err
	}
	a := &app{Config: c, Manager: &manager{Repo: repo, DB: couchDB{h}, People: c.People}, OAuth: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: p.Endpoint(), RedirectURL: c.URL + "/callback", Scopes: c.Scopes}, Verifier: p.Verifier(&oidc.Config{ClientID: c.ClientID}), HTTP: h, Sessions: map[string]session{}, Logins: map[string]login{}, Templates: template.Must(template.ParseFS(assets, "templates/*")), Generate: generate}
	s := http.Server{Addr: c.Listen, Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	return s.ListenAndServe()
}

func discoverProvider(ctx context.Context, h *http.Client, issuer string) (*oidc.Provider, error) {
	if !validIssuer(issuer) {
		return nil, errors.New("invalid OIDC issuer")
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, h), issuer)
	if err != nil {
		return nil, errors.New("OIDC discovery failed")
	}
	// Discovery is trusted configuration, never a URL supplied by the browser.
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if p.Claims(&discovery) != nil || !validEndpoint(discovery.JWKS, false) || !validEndpoint(p.Endpoint().AuthURL, false) || !validEndpoint(p.Endpoint().TokenURL, false) {
		return nil, errors.New("OIDC requires HTTPS endpoints")
	}
	return p, nil
}
