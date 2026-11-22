// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestVaultUIAndSetupFragments(t *testing.T) {
	a := testApp(t)
	_, _ = a.Manager.run(context.Background(), "alice", "list", "", "")
	page := call(a, "/", "GET", nil, "", true).Body.String()
	for _, absent := range []string{"The default vault cannot be deleted.", "Initialize default", "<nav", `href="/">My vaults`, "dummy-uri-key", "dummy-stable-key", "sls-id-v1:"} {
		if strings.Contains(page, absent) {
			t.Fatalf("initial page contains %q", absent)
		}
	}
	if !strings.Contains(page, "<h1>My vaults</h1>") || !strings.Contains(page, "Signed in as <strong>alice</strong>") || strings.Index(page, `id="vaults-title"`) > strings.Index(page, `id="add-title"`) {
		t.Fatal("page hierarchy")
	}
	for _, tc := range []struct {
		id, csrf, person string
		status           int
	}{{"default", "csrf", "alice", 200}, {"missing", "csrf", "alice", 200}, {"default", "wrong", "alice", 403}, {"default", "csrf", "bob", 403}} {
		f := url.Values{"action": {"setup"}, "csrf": {tc.csrf}, "person": {tc.person}, "id": {tc.id}}
		r := httptest.NewRequest("POST", a.Config.URL+"/", strings.NewReader(f.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", a.Config.URL)
		r.Header.Set("HX-Request", "true")
		r.Header.Set("HX-Target", "div#setup-content")
		r.AddCookie(&http.Cookie{Name: "__Host-obsidian", Value: "valid"})
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		body := w.Body.String()
		if w.Code != tc.status || !strings.Contains(body, `id="setup-title"`) || strings.Contains(body, "<html") || strings.Contains(body, "<footer") {
			t.Fatal(tc, w.Code, body)
		}
		if tc.id == "default" && tc.status == 200 {
			if strings.Count(body, `type="password"`) != 3 || strings.Count(body, "data-copy=") != 3 || strings.Count(body, "data-reveal=") != 3 {
				t.Fatal("secret controls", body)
			}
		} else if strings.Contains(body, "dummy-uri-key") {
			t.Fatal("error leaked credentials")
		}
	}
}

func TestAutomaticReconcilePreservesIntent(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	d := a.Manager.DB.(*fakeDB)
	d.fail = 1
	_, _ = a.Manager.run(ctx, "alice", "list", "", "")
	before := len(d.calls)
	page := call(a, "/", "GET", nil, "", true).Body.String()
	if len(d.calls) != before || !strings.Contains(page, "data-retry") {
		t.Fatal("GET reconciled or missing retry")
	}
	sync := url.Values{"csrf": {"csrf"}, "action": {"sync"}}
	if w := call(a, "/", "POST", sync, a.Config.URL, true); w.Code != 200 || strings.Contains(w.Body.String(), "data-retry") {
		t.Fatal(w.Body.String())
	}
	vs, err := a.Manager.run(ctx, "alice", "create", "", "Work")
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, v := range vs {
		if v.ID != "default" {
			id = v.ID
		}
	}
	d.fail = 1
	_, _ = a.Manager.run(ctx, "alice", "delete", id, "")
	before = len(d.calls)
	_ = call(a, "/", "GET", nil, "", true)
	if len(d.calls) != before {
		t.Fatal("view executed delete")
	}
	_ = call(a, "/", "POST", sync, a.Config.URL, true)
	vs, err = a.Manager.view(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.ID == id && v.State != "deleted" {
			t.Fatal("queued delete not completed")
		}
		if v.ID == "default" && v.State != "active" {
			t.Fatal("default changed")
		}
	}
	before = len(d.calls)
	_ = call(a, "/", "POST", sync, a.Config.URL, true)
	if len(d.calls) != before {
		t.Fatal("reconcile resurrected database or created deletion intent")
	}
}

// Opt-in real-browser coverage. All identities, catalogs and credentials are fixtures.
func TestBrowserUI(t *testing.T) {
	if os.Getenv("PORTAL_BROWSER") != "1" {
		t.Skip("set PORTAL_BROWSER=1 with Node, Playwright and Chromium; see VALIDATION.md")
	}
	a := testApp(t)
	_, _ = a.Manager.run(context.Background(), "alice", "list", "", "")
	server := httptest.NewTLSServer(a)
	defer server.Close()
	a.Config.URL = server.URL
	a.Config.SourceURL = releaseSourceURL("1.0.0", strings.Repeat("a", 40))
	cmd := exec.Command("node", "scripts/test-browser.mjs", server.URL)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, out)
	}
	t.Log(string(out))
}

func TestBrowserAutomaticRetries(t *testing.T) {
	if os.Getenv("PORTAL_BROWSER") != "1" {
		t.Skip("opt-in browser test")
	}
	for _, mode := range []string{"bootstrap", "delete", "bounded"} {
		t.Run(mode, func(t *testing.T) {
			a := testApp(t)
			d := a.Manager.DB.(*fakeDB)
			ctx := context.Background()
			if mode == "delete" {
				vs, err := a.Manager.run(ctx, "alice", "create", "", "Work")
				if err != nil {
					t.Fatal(err)
				}
				var id string
				for _, v := range vs {
					if v.ID != "default" {
						id = v.ID
					}
				}
				d.fail = 1
				_, _ = a.Manager.run(ctx, "alice", "delete", id, "")
			} else {
				d.fail = 1
				if mode == "bounded" {
					d.fail = 100
				}
			}
			server := httptest.NewTLSServer(a)
			defer server.Close()
			a.Config.URL = server.URL
			cmd := exec.Command("node", "scripts/test-retry-browser.mjs", server.URL, mode)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			t.Log(string(out))
		})
	}
}
