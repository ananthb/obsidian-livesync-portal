// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type credentials struct {
	ManagementURL string `json:"management_url"`
	Host          string `json:"host"`
	User          string `json:"user"`
	Password      string `json:"password"`
	Database      string `json:"database"`
	Passphrase    string `json:"passphrase"`
	Recovery      string `json:"id_recovery_code"`
}
type person struct {
	Issuer      string      `json:"issuer"`
	Subject     string      `json:"subject"`
	Credentials credentials `json:"credentials"`
}
type vault struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Database   string `json:"database"`
	State      string `json:"state"`
	Passphrase string `json:"passphrase"`
	Recovery   string `json:"recovery"`
}
type catalog struct {
	Issuer  string           `json:"issuer"`
	Person  string           `json:"person"`
	Subject string           `json:"subject"`
	Vaults  map[string]vault `json:"vaults"`
}
type database interface {
	Change(context.Context, string, credentials, string) error
}
type couchDB struct{ HTTP *http.Client }

var safePerson = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var safeID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var safeRecovery = regexp.MustCompile(`^sls-id-v1:[0-9a-f]{64}$`)

func secret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("system entropy unavailable")
	}
	return hex.EncodeToString(b)
}

func (c couchDB) Change(ctx context.Context, method string, cred credentials, db string) error {
	// Only the generated database name crosses this URL boundary.
	if !safeDatabase.MatchString(db) {
		return errors.New("unsafe database")
	}
	if !validEndpoint(cred.ManagementURL, true) || (method != "PUT" && method != "DELETE") {
		return errors.New("invalid server host")
	}
	r, err := http.NewRequestWithContext(ctx, method, cred.ManagementURL+"/"+db, nil)
	if err != nil {
		return err
	}
	r.SetBasicAuth(cred.User, cred.Password)
	resp, err := c.HTTP.Do(r)
	if err != nil {
		return errors.New("CouchDB unavailable")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if (method == "PUT" && (resp.StatusCode == 201 || resp.StatusCode == 202 || resp.StatusCode == 412)) || (method == "DELETE" && (resp.StatusCode == 200 || resp.StatusCode == 202 || resp.StatusCode == 404)) {
		return nil
	}
	return fmt.Errorf("CouchDB operation failed (%d)", resp.StatusCode)
}

type manager struct {
	Repo   repository
	DB     database
	People map[string]person
	once   sync.Once
	gate   chan struct{}
}

func (m *manager) lock(ctx context.Context) error {
	m.once.Do(func() { m.gate = make(chan struct{}, 1) })
	select {
	case m.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return errors.New("operation timed out waiting for another request")
	}
}
func (m *manager) unlock() { <-m.gate }

func (m *manager) load(ctx context.Context, p string) (catalog, int, credentials, error) {
	identity, ok := m.People[p]
	if !ok || !safePerson.MatchString(p) {
		return catalog{}, 0, credentials{}, errors.New("unknown person")
	}
	c, n, err := m.Repo.Load(ctx, p)
	if err != nil {
		return c, n, credentials{}, err
	}
	if n == 0 {
		c = catalog{Person: p, Issuer: identity.Issuer, Subject: identity.Subject, Vaults: map[string]vault{}}
	}
	if c.Person != p || c.Subject != identity.Subject || c.Issuer != identity.Issuer || c.Vaults == nil {
		return c, n, credentials{}, errors.New("catalog identity mismatch")
	}
	cred, err := m.Repo.Credentials(ctx, p)
	if err != nil {
		return c, n, cred, err
	}
	if !validCredentials(cred) {
		return c, n, cred, errors.New("invalid default credentials")
	}
	for id, v := range c.Vaults {
		expected := cred.Database + "-" + id
		// Keep existing database identities untouched. New databases use a bounded,
		// validated owner prefix and random immutable ID, never a display label.
		if id != "default" && v.Database == p+"-"+id {
			expected = v.Database
		}
		if id == "default" {
			expected = cred.Database
			if v.State == "deleting" || v.State == "deleted" {
				return c, n, cred, errors.New("invalid protected default state")
			}
		}
		if (id != "default" && !safeID.MatchString(id)) || v.ID != id || v.Database != expected || !validState(v.State) {
			return c, n, cred, errors.New("invalid catalog")
		}
	}
	return c, n, cred, nil
}
func validState(s string) bool {
	return s == "creating" || s == "active" || s == "deleting" || s == "deleted"
}

func (m *manager) view(ctx context.Context, p string) ([]vault, error) {
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.unlock()
	c, _, _, err := m.load(ctx, p)
	if err != nil {
		return nil, err
	}
	return sortedVaults(c), nil
}
func sortedVaults(c catalog) []vault {
	out := make([]vault, 0, len(c.Vaults))
	for _, v := range c.Vaults {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *manager) run(ctx context.Context, p, action, id, name string) ([]vault, error) {
	// One replica serializes CouchDB side effects; CAS also protects Vault against stale writes.
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.unlock()
	for attempt := 0; attempt < 5; attempt++ {
		c, n, cred, err := m.load(ctx, p)
		if err != nil {
			return nil, err
		}
		dirty := false
		if _, ok := c.Vaults["default"]; !ok {
			c.Vaults["default"] = vault{ID: "default", Name: "Default", Database: cred.Database, State: "creating", Passphrase: cred.Passphrase, Recovery: cred.Recovery}
			dirty = true
		}
		if action == "create" {
			name, err = qualifiedVaultName(p, name)
			if err != nil {
				return nil, err
			}
			if len(c.Vaults) >= 200 {
				return nil, errors.New("vault limit reached (including tombstones)")
			}
			for _, v := range c.Vaults {
				existing, _ := qualifiedVaultName(p, v.Name)
				if (strings.EqualFold(existing, name) || strings.EqualFold(v.Name, name)) && v.State != "deleted" {
					return nil, errors.New("name already used")
				}
			}
			id = secret(16)
			c.Vaults[id] = vault{ID: id, Name: name, Database: p + "-" + id, State: "creating", Passphrase: secret(32), Recovery: "sls-id-v1:" + secret(32)}
			dirty = true
		}
		if action == "delete" {
			if id == "default" {
				return nil, errors.New("the default vault cannot be deleted")
			}
			v, ok := c.Vaults[id]
			if !ok {
				return nil, errors.New("unknown vault")
			}
			if v.State != "deleted" {
				v.State = "deleting"
				c.Vaults[id] = v
				dirty = true
			}
		}
		if dirty {
			err = m.Repo.Save(ctx, p, n, c)
			if errors.Is(err, errConflict) {
				continue
			}
			if err != nil {
				return nil, err
			}
			n++
		}
		for key, v := range c.Vaults {
			method := ""
			switch v.State {
			case "creating":
				method = "PUT"
			case "deleting":
				method = "DELETE"
			}
			if method == "" {
				continue
			}
			if err = m.DB.Change(ctx, method, cred, v.Database); err != nil {
				return nil, errors.New("Database operation pending. Automatic retries will resume shortly.")
			}
			if method == "PUT" {
				v.State = "active"
			} else {
				v.State = "deleted"
				v.Passphrase = ""
				v.Recovery = ""
			}
			c.Vaults[key] = v
			if err = m.Repo.Save(ctx, p, n, c); err != nil {
				return nil, errors.New("Metadata update pending. Automatic retries will resume shortly.")
			}
			n++
		}
		return sortedVaults(c), nil
	}
	return nil, errConflict
}

func qualifiedVaultName(owner, label string) (string, error) {
	if !safePerson.MatchString(owner) || !utf8.ValidString(label) {
		return "", errors.New("invalid vault name")
	}
	prefix := owner + "-"
	label = strings.TrimSpace(label)
	if len(label) >= len(prefix) && strings.EqualFold(label[:len(prefix)], prefix) {
		label = strings.TrimSpace(label[len(prefix):])
	}
	name := prefix + label
	if label == "" || len(name) > 80 {
		return "", errors.New("vault name must include a label and be at most 80 bytes including the username prefix")
	}
	for _, r := range label {
		if unicode.IsControl(r) || strings.ContainsRune("/\\", r) {
			return "", errors.New("vault name cannot contain control characters or slashes")
		}
	}
	return name, nil
}

func (m *manager) setup(ctx context.Context, p, id string) (credentials, error) {
	if err := m.lock(ctx); err != nil {
		return credentials{}, err
	}
	defer m.unlock()
	c, _, cred, err := m.load(ctx, p)
	if err != nil {
		return credentials{}, err
	}
	v, ok := c.Vaults[id]
	if !ok || v.State != "active" {
		return credentials{}, errors.New("vault not active")
	}
	cred.Database = v.Database
	cred.Passphrase = v.Passphrase
	cred.Recovery = v.Recovery
	return cred, nil
}
