// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestFailedReleasePatchWirePayload(t *testing.T) {
	gh, err := exec.LookPath("gh")
	if err != nil {
		t.Skip("gh required")
	}
	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if r.Method != "PATCH" {
			t.Errorf("method = %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		received <- payload
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	// The exact flags from failed attempt 3: -F emits a boolean, -f emits strings.
	cmd := exec.Command(gh, "api", "--method", "PATCH", server.URL,
		"-F", "draft=true", "-f", "target_commitish="+strings.Repeat("a", 40),
		"-f", "name=v1.2.3", "-f", "body=Built from commit; Corresponding Source")
	cmd.Env = append(os.Environ(), "GH_TOKEN=fixture-token", "GH_DEBUG=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	payload := <-received
	if len(payload) != 4 || payload["draft"] != true || payload["tag_name"] != nil || payload["target_commitish"] != strings.Repeat("a", 40) || payload["name"] != "v1.2.3" {
		t.Fatalf("unexpected original wire payload: %#v", payload)
	}
	t.Logf("original PATCH wire payload (tag_name absent, draft boolean): %#v", payload)
}

func TestReleaseAssetBounds(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"empty.txt", "payload.txt", "large.txt"} {
		if err := os.WriteFile(filepath.Join(root, "dist", name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Truncate(filepath.Join(root, "dist/large.txt"), 2147483648); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("payload.txt", filepath.Join(root, "dist/link.txt")); err != nil {
		t.Fatal(err)
	}
	helper, err := filepath.Abs("scripts/release-api.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"dist/empty.txt", "dist/large.txt", "dist/link.txt", "dist/-option", "dist/trailing.", "dist/name with spaces", "dist/name?query", "dist/" + strings.Repeat("a", 256), "dist/../payload.txt", "elsewhere/payload.txt"} {
		t.Run(file, func(t *testing.T) {
			cmd := exec.Command("bash", "-c", `set -euo pipefail; source "$1"; gh() { echo 'unexpected API call' >&2; exit 99; }; release_asset '{}' "$2" .`, "bash", helper, file)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err == nil || strings.Contains(string(out), "unexpected API call") {
				t.Fatalf("unsafe file reached API: %v: %s", err, out)
			}
		})
	}
}

// Exercise the real gh api serializer, pagination, raw --input upload and binary
// download against HTTP, not a release CLI stub. No GitHub resources are mutated.
func releaseAPIFixture(t *testing.T, root, scenario string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	sha := strings.Repeat("a", 40)
	releases := map[int]map[string]any{}
	assets := map[int]map[string]any{}
	contents := map[int][]byte{}
	owners := map[int]int{}
	nextAsset := 100
	appUploads := 0
	partialFailed := false
	newRelease := func(id int, draft bool) map[string]any {
		tag, title := "v1.2.3", "v1.2.3"
		body := fmt.Sprintf("Built from commit %s. Corresponding Source: https://github.com/test/portal/releases/download/source-%s/obsidian-livesync-portal_1.2.3_source.tar.gz . Verify the signed checksums and immutable image digest.", sha, sha)
		if id == 2 {
			tag, title = "source-"+sha, "Corresponding Source "+sha
			body = "Exact build source for " + sha + ". No application binaries are published in this source-only release."
		}
		return map[string]any{"id": id, "tag_name": tag, "target_commitish": sha, "name": title, "body": body, "draft": draft, "upload_url": fmt.Sprintf("https://uploads.github.com/repos/test/portal/releases/%d/assets{?name,label}", id), "author": map[string]any{"login": "github-actions[bot]"}}
	}
	addAsset := func(releaseID int, name string, data []byte) map[string]any {
		nextAsset++
		a := map[string]any{"id": nextAsset, "name": name, "size": len(data), "state": "uploaded", "content_type": "application/octet-stream"}
		assets[nextAsset], contents[nextAsset], owners[nextAsset] = a, data, releaseID
		return a
	}
	if scenario == "retry-draft" || scenario == "asset-mismatch" || scenario == "finalized" || scenario == "retagged-draft" {
		releases[1] = newRelease(1, scenario != "finalized")
		if scenario == "retagged-draft" {
			releases[1]["tag_name"] = "untagged-f9b838d15f641da3fc16"
		}
		if scenario == "asset-mismatch" {
			data, err := os.ReadFile(filepath.Join(root, "dist", "obsidian-livesync-portal_1.2.3_linux_amd64.tar.gz"))
			if err != nil || len(data) == 0 {
				t.Fatalf("archive fixture: %v", err)
			}
			data[0] ^= 0xff
			addAsset(1, "obsidian-livesync-portal_1.2.3_linux_amd64.tar.gz", data)
		}
		if scenario == "retry-draft" {
			data, err := os.ReadFile(filepath.Join(root, "dist", "obsidian-livesync-portal_1.2.3_linux_amd64.tar.gz"))
			if err != nil {
				t.Fatal(err)
			}
			addAsset(1, "obsidian-livesync-portal_1.2.3_linux_amd64.tar.gz", data)
		}
	}
	if scenario == "retry-source" || scenario == "retry-source-bundle" || scenario == "source-mismatch" {
		releases[2] = newRelease(2, false)
		data := []byte("mock exact build source")
		if scenario == "source-mismatch" {
			data = []byte("different source; must never overwrite")
		}
		addAsset(2, "obsidian-livesync-portal_1.2.3_source.tar.gz", data)
		bundle := []byte("dummy\n")
		if scenario == "retry-source-bundle" {
			bundle = []byte("previous valid signature\n")
		}
		addAsset(2, "obsidian-livesync-portal_1.2.3_source.tar.gz.sigstore.json", bundle)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fail := func(status int, message string) {
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"message": message})
		}
		respond := func(value any) { json.NewEncoder(w).Encode(value) }
		if strings.HasPrefix(r.URL.Path, "/download/") {
			id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/download/"))
			w.Header().Set("Content-Type", "application/octet-stream")
			data := contents[id]
			if scenario == "upload-corrupt-bytes" && owners[id] == 1 {
				data = append([]byte("bad"), data...)
			}
			w.Write(data)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/repos/test/portal/")
		if strings.HasPrefix(path, "git/ref/tags/") {
			value := sha
			if scenario == "moved-tag" {
				value = strings.Repeat("b", 40)
			}
			kind := "commit"
			if scenario == "annotated-tag" {
				kind = "tag"
			}
			respond(map[string]any{"object": map[string]string{"type": kind, "sha": value}})
			return
		}
		if strings.HasPrefix(path, "git/tags/") && scenario == "annotated-tag" {
			respond(map[string]any{"object": map[string]string{"type": "commit", "sha": sha}})
			return
		}
		if path == "releases" && r.Method == "GET" {
			result := []any{}
			for _, release := range releases {
				if scenario != "new-draft-invisible" || release["draft"] == false {
					result = append(result, release)
				}
			}
			respond(result)
			return
		}
		if strings.HasPrefix(path, "releases/assets/") && r.Method == "GET" {
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "releases/assets/"))
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Error("download did not request binary data")
			}
			if id%2 == 0 {
				http.Redirect(w, r, fmt.Sprintf("/download/%d", id), http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			data := contents[id]
			if scenario == "upload-corrupt-bytes" && owners[id] == 1 {
				data = append([]byte("bad"), data...)
			}
			w.Write(data)
			return
		}
		parts := strings.Split(path, "/")
		id := 0
		if len(parts) >= 2 {
			id, _ = strconv.Atoi(parts[1])
		}
		if len(parts) == 3 && parts[2] == "assets" {
			if releases[id] == nil {
				fail(404, "Release not found")
				return
			}
			if r.Method == "GET" {
				result := []any{}
				ids := []int{}
				for assetID := range assets {
					ids = append(ids, assetID)
				}
				sort.Ints(ids)
				for _, assetID := range ids {
					if owners[assetID] == id {
						result = append(result, assets[assetID])
					}
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if page == 0 {
					page = 1
				}
				start, end := (page-1)*2, page*2
				if end < len(result) {
					w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=%d>; rel="next"`, r.Host, r.URL.Path, page+1))
				} else {
					end = len(result)
				}
				result = result[start:end]
				respond(result)
				return
			}
			if r.Method == "POST" {
				if scenario == "upload" || ((scenario == "partial-upload" || scenario == "retry-partial") && !partialFailed && id == 1 && appUploads == 2) {
					partialFailed = true
					fail(502, "simulated partial upload")
					return
				}
				name := r.URL.Query().Get("name")
				if len(r.URL.Query()) != 1 || name == "" {
					t.Error("invalid upload URL query")
				}
				mime := "application/octet-stream"
				switch {
				case strings.HasSuffix(name, ".tar.gz"):
					mime = "application/gzip"
				case strings.HasSuffix(name, ".json"):
					mime = "application/json"
				case strings.HasSuffix(name, ".txt"):
					mime = "text/plain"
				}
				if r.Header.Get("Content-Type") != mime {
					t.Errorf("upload %s MIME %q, want %q", name, r.Header.Get("Content-Type"), mime)
				}
				data, _ := io.ReadAll(r.Body)
				expected, err := os.ReadFile(filepath.Join(root, "dist", name))
				if err != nil || string(data) != string(expected) || r.ContentLength != int64(len(data)) {
					t.Errorf("upload was not exact raw file bytes with correct Content-Length: %s", name)
				}
				for assetID, a := range assets {
					if owners[assetID] == id && a["name"] == name {
						fail(422, "already_exists")
						return
					}
				}
				a := addAsset(id, name, data)
				dir := "published"
				if id == 2 {
					dir = "source-published"
				} else {
					appUploads++
					if scenario == "upload-wrong-name" {
						a["name"] = "wrong"
					}
					if scenario == "upload-wrong-size" {
						a["size"] = -1
					}
				}
				os.MkdirAll(filepath.Join(root, dir), 0700)
				os.WriteFile(filepath.Join(root, dir, name), data, 0600)
				w.WriteHeader(201)
				respond(a)
				return
			}
		}
		if r.Method == "GET" && len(parts) == 2 && releases[id] != nil {
			respond(releases[id])
			return
		}
		if (path == "releases" && r.Method == "POST") || (len(parts) == 2 && r.Method == "PATCH") {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
				fail(400, "invalid JSON")
				return
			}
			if path == "releases" {
				id = 1
				if strings.HasPrefix(fmt.Sprint(payload["tag_name"]), "source-") {
					id = 2
				}
				if releases[id] != nil {
					fail(422, "already_exists")
					return
				}
			}
			expected := newRelease(id, true)
			for _, key := range []string{"tag_name", "target_commitish", "name", "body"} {
				if payload[key] != expected[key] {
					t.Errorf("%s %s: incorrect %s: %#v", r.Method, path, key, payload[key])
				}
			}
			draft, ok := payload["draft"].(bool)
			if !ok {
				t.Errorf("draft must be JSON boolean, got %#v", payload["draft"])
			}
			latest, ok := payload["make_latest"].(string)
			if !ok || (latest != "false" && !(id == 1 && !draft && latest == "true")) {
				t.Errorf("make_latest must be contract string, got %#v", payload["make_latest"])
			}
			if r.Method == "PATCH" && id == 1 && !draft && scenario == "final-patch-failure" {
				fail(403, "Resource not accessible by integration")
				return
			}
			// Reproduce the observed server hazard if a future regression omits tag_name.
			if payload["tag_name"] == nil {
				payload["tag_name"] = "untagged-f9b838d15f641da3fc16"
			}
			expected["draft"] = draft
			for key, value := range payload {
				expected[key] = value
			}
			releases[id] = expected
			state := "draft"
			if !draft {
				state = "final"
			}
			stateFile := "state"
			if id == 2 {
				stateFile = "state-source"
			}
			os.WriteFile(filepath.Join(root, stateFile), []byte(state), 0600)
			response := map[string]any{}
			for key, value := range expected {
				response[key] = value
			}
			if r.Method == "PATCH" && id == 1 && draft {
				switch scenario {
				case "update-missing-tag":
					delete(response, "tag_name")
				case "update-mutated-tag":
					response["tag_name"] = "untagged-f9b838d15f641da3fc16"
				case "update-wrong-id":
					response["id"] = 99
				case "update-wrong-target":
					response["target_commitish"] = "main"
				case "update-wrong-body":
					response["body"] = "wrong source URL"
				case "update-wrong-draft":
					response["draft"] = "true"
				case "persisted-wrong-tag":
					releases[id]["tag_name"] = "untagged-f9b838d15f641da3fc16"
				}
			}
			respond(response)
			return
		}
		t.Errorf("unexpected REST request: %s %s", r.Method, r.URL)
		fail(404, "Not Found")
	}))
	t.Cleanup(server.Close)
	return server
}
