// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

var errConflict = errors.New("concurrent update; retry")

type vaultClient struct {
	URL, JWTFile                        string
	Token, Mount, Path, AuthMount, Role string
	HTTP                                *http.Client
	mu                                  sync.Mutex
	token                               string
	expires                             time.Time
}

func (v *vaultClient) request(ctx context.Context, method, path, token string, body any, out any) (int, error) {
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			return 0, err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, v.URL+"/v1/"+path, &b)
	if err != nil {
		return 0, err
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("X-Vault-Token", token)
	}
	resp, err := v.HTTP.Do(r)
	if err != nil {
		return 0, errors.New("Vault unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil {
			err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
		}
		return resp.StatusCode, err
	}
	return resp.StatusCode, nil
}

func (v *vaultClient) login(ctx context.Context) (string, error) {
	if v.Token != "" {
		return v.Token, nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().Before(v.expires) {
		return v.token, nil
	}
	jwt, err := os.ReadFile(v.JWTFile)
	if err != nil {
		return "", errors.New("workload identity unavailable")
	}
	var out struct {
		Auth struct {
			Token string `json:"client_token"`
			TTL   int    `json:"lease_duration"`
		} `json:"auth"`
	}
	status, err := v.request(ctx, "POST", "auth/"+v.AuthMount+"/login", "", map[string]string{"role": v.Role, "jwt": string(jwt)}, &out)
	if err != nil || status != 200 || out.Auth.Token == "" || out.Auth.TTL < 60 {
		return "", errors.New("workload login failed")
	}
	v.token = out.Auth.Token
	v.expires = time.Now().Add(time.Duration(out.Auth.TTL/2) * time.Second)
	return v.token, nil
}

func (v *vaultClient) read(ctx context.Context, path string, dst any) (int, error) {
	token, err := v.login(ctx)
	if err != nil {
		return 0, err
	}
	var out struct {
		Data struct {
			Data     json.RawMessage `json:"data"`
			Metadata struct {
				Version int `json:"version"`
			} `json:"metadata"`
		} `json:"data"`
	}
	status, err := v.request(ctx, "GET", v.Mount+"/data/"+path, token, nil, &out)
	if err != nil {
		return 0, err
	}
	if status == 404 {
		return 0, nil
	}
	if status != 200 {
		return 0, errors.New("Vault read denied or failed")
	}
	if err := json.Unmarshal(out.Data.Data, dst); err != nil {
		return 0, errors.New("invalid Vault data")
	}
	return out.Data.Metadata.Version, nil
}

func (v *vaultClient) write(ctx context.Context, path string, version int, data any) error {
	token, err := v.login(ctx)
	if err != nil {
		return err
	}
	status, err := v.request(ctx, "POST", v.Mount+"/data/"+path, token, map[string]any{"options": map[string]int{"cas": version}, "data": data}, nil)
	if err != nil {
		return err
	}
	if status == 400 {
		return errConflict
	}
	if status != 200 && status != 204 {
		return errors.New("Vault write failed")
	}
	return nil
}

type repository interface {
	Load(context.Context, string) (catalog, int, error)
	Save(context.Context, string, int, catalog) error
	Credentials(context.Context, string) (credentials, error)
}

type vaultRepository struct {
	*vaultClient
	People map[string]person
}

func (v vaultRepository) Load(ctx context.Context, person string) (catalog, int, error) {
	if _, ok := v.People[person]; !ok || !safePerson.MatchString(person) {
		return catalog{}, 0, errors.New("unknown user")
	}
	var c catalog
	n, err := v.read(ctx, v.Path+"/"+person, &c)
	return c, n, err
}
func (v vaultRepository) Save(ctx context.Context, person string, n int, c catalog) error {
	p, ok := v.People[person]
	if !ok || !safePerson.MatchString(person) || c.Person != person || c.Issuer != p.Issuer || c.Subject != p.Subject {
		return errors.New("catalog identity mismatch")
	}
	return v.write(ctx, v.Path+"/"+person, n, c)
}
func (v vaultRepository) Credentials(ctx context.Context, person string) (credentials, error) {
	p, ok := v.People[person]
	if !ok {
		return credentials{}, errors.New("unknown user")
	}
	return p.Credentials, nil
}
