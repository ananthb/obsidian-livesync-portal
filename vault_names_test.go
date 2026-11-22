// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestQualifiedVaultNames(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{" work ", "alice-work"}, {"alice-work", "alice-work"}, {"ALICE-Work", "alice-Work"}, {"雪 notes", "alice-雪 notes"}, {strings.Repeat("x", 74), "alice-" + strings.Repeat("x", 74)}} {
		got, err := qualifiedVaultName("alice", tc.input)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
	for _, input := range []string{"", " ", "alice-", "ALICE-  ", strings.Repeat("x", 75), "a\nb", "../work", "a\\b", string([]byte{0xff})} {
		if _, err := qualifiedVaultName("alice", input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestNamespacedCreateDuplicateConcurrencyAndKeys(t *testing.T) {
	m, _, d, _ := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for _, label := range []string{"work", " alice-work ", "WORK", "ALICE-Work"} {
		wg.Add(1)
		go func(label string) {
			defer wg.Done()
			if _, err := m.run(ctx, "alice", "create", "", label); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(label)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatal("noncanonical concurrent duplicates", successes)
	}
	vs, err := m.view(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, v := range vs {
		if v.ID != "default" {
			id = v.ID
			if !strings.EqualFold(v.Name, "alice-work") || v.Database != "alice-"+v.ID || !safeDatabase.MatchString(v.Database) {
				t.Fatal(v)
			}
		} else if v.Name != "Default" || v.Database != "notes-alice" {
			t.Fatal("default renamed", v)
		}
	}
	first, err := m.setup(ctx, "alice", id)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = m.run(ctx, "alice", "list", "", "")
	second, err := m.setup(ctx, "alice", id)
	if err != nil || first != second {
		t.Fatal("remint credentials changed")
	}
	before := len(d.calls)
	for _, bad := range []string{"alice-work", "WORK", "alice-", "../bad", strings.Repeat("x", 81)} {
		if _, err = m.run(ctx, "alice", "create", "", bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	if len(d.calls) != before {
		t.Fatal("database side effect before valid reservation")
	}
	if _, err = m.run(ctx, "alice", "delete", "default", ""); err == nil {
		t.Fatal("default delete")
	}
	// Pending deletion still reserves the name; tombstones can be reused with a fresh ID.
	d.fail = 1
	_, _ = m.run(ctx, "alice", "delete", id, "")
	if _, err = m.run(ctx, "alice", "create", "", "work"); err == nil {
		t.Fatal("pending name reused")
	}
	_, _ = m.run(ctx, "alice", "list", "", "")
	if _, err = m.run(ctx, "alice", "create", "", "work"); err != nil {
		t.Fatal("tombstone name reuse", err)
	}
}

func TestExistingVaultIdentityUnchanged(t *testing.T) {
	m, r, d, _ := fixture(t)
	ctx := context.Background()
	_, _ = m.run(ctx, "alice", "list", "", "")
	c, n, err := r.Load(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	legacy := vault{ID: id, Name: "Work", Database: "notes-alice-" + id, State: "active", Passphrase: "legacy-dummy", Recovery: "sls-id-v1:" + strings.Repeat("ab", 32)}
	c.Vaults[id] = legacy
	if err = r.Save(ctx, "alice", n, c); err != nil {
		t.Fatal(err)
	}
	before := len(d.calls)
	if _, err = m.run(ctx, "alice", "list", "", ""); err != nil {
		t.Fatal(err)
	}
	after, _, _ := r.Load(ctx, "alice")
	if after.Vaults[id] != legacy || len(d.calls) != before {
		t.Fatal("legacy renamed/recreated")
	}
	if _, err = m.run(ctx, "alice", "create", "", "alice-work"); err == nil {
		t.Fatal("legacy canonical duplicate")
	}
}

func TestNamespacedCreateHTTPAndMaximumOwner(t *testing.T) {
	a := testApp(t)
	form := url.Values{"csrf": {"csrf"}, "action": {"create"}, "name": {"work"}, "username": {"bob"}}
	w := call(a, "/", "POST", form, a.Config.URL, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "alice-work") {
		t.Fatal(w.Code, w.Body.String())
	}
	form.Set("name", "ALICE-Work")
	if w = call(a, "/", "POST", form, a.Config.URL, true); !strings.Contains(w.Body.String(), "name already used") {
		t.Fatal(w.Body.String())
	}
	form.Set("person", "bob")
	if w = call(a, "/", "POST", form, a.Config.URL, true); w.Code != 403 {
		t.Fatal("cross-person create", w.Code)
	}
	owner := "a" + strings.Repeat("b", 62)
	identity := a.Manager.People["alice"]
	identity.Subject = "max-owner"
	a.Manager.People[owner] = identity
	vs, err := a.Manager.run(context.Background(), owner, "create", "", strings.Repeat("x", 16))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.ID != "default" && (len(v.Database) != 96 || v.Database != owner+"-"+v.ID || !safeDatabase.MatchString(v.Database) || len(v.Name) != 80) {
			t.Fatal(v)
		}
	}
}
