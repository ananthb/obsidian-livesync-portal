// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseOrderingAndRESTContract(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("jq required for release REST contract tests")
		}
		t.Skip("jq required for release script simulations")
	}
	realGH, err := exec.LookPath("gh")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("real gh required for release REST contract tests")
		}
		t.Skip("real gh required for release REST contract tests")
	}
	for _, scenario := range []string{"success", "annotated-tag", "retry-draft", "retry-source", "retry-source-bundle", "retry-partial", "new-draft-invisible", "smoke-arm64", "upload", "partial-upload", "promotion", "finalized", "retagged-draft", "moved-tag", "metadata", "signature", "legal", "source-legal", "image-legal", "source-public", "source-public-bytes", "source-public-signature", "source-signature", "source-late", "source-mismatch", "update-missing-tag", "update-mutated-tag", "update-wrong-id", "update-wrong-target", "update-wrong-body", "update-wrong-draft", "persisted-wrong-tag", "upload-wrong-name", "upload-wrong-size", "upload-corrupt-bytes", "final-patch-failure", "asset-mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			write := func(name string, b []byte, mode os.FileMode) {
				t.Helper()
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, b, mode); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"release-api.sh", "release-common.sh", "release-images.sh", "release-finalize.sh", "publish-source.sh", "image-smoke.sh"} {
				b, err := os.ReadFile("scripts/" + name)
				if err != nil {
					t.Fatal(err)
				}
				write("scripts/"+name, b, 0600)
			}
			write("licenses/distroless/inventory.json", []byte(`{"review_status":"approved","image":"base","index":"sha256:base"}`), 0600)
			write("manifest-fixture", []byte(`{"manifests":[{"digest":"sha256:`+strings.Repeat("a", 64)+`","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:`+strings.Repeat("b", 64)+`","platform":{"os":"linux","architecture":"arm64"}}]}`), 0600)
			var checksums strings.Builder
			for _, arch := range []string{"amd64", "arm64"} {
				name := "obsidian-livesync-portal_1.2.3_linux_" + arch + ".tar.gz"
				path := filepath.Join(root, "dist", name)
				os.MkdirAll(filepath.Dir(path), 0700)
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				gz := gzip.NewWriter(f)
				tw := tar.NewWriter(gz)
				for _, file := range []string{"livesync-portal"} {
					body := []byte("dummy")
					if err = tw.WriteHeader(&tar.Header{Name: file, Mode: 0755, Size: int64(len(body))}); err != nil {
						t.Fatal(err)
					}
					tw.Write(body)
				}
				tw.Close()
				gz.Close()
				f.Close()
				body, _ := os.ReadFile(path)
				fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(body), name)
			}
			write("dist/checksums.txt", []byte(checksums.String()), 0600)
			source := []byte("mock exact build source")
			write("dist/obsidian-livesync-portal_1.2.3_source.tar.gz", source, 0600)
			fmt.Fprintf(&checksums, "%x  obsidian-livesync-portal_1.2.3_source.tar.gz\n", sha256.Sum256(source))
			write("dist/checksums.txt", []byte(checksums.String()), 0600)
			for _, name := range []string{"docker", "cosign", "go", "python3", "curl", "goreleaser", "crane"} {
				write("bin/"+name, []byte(releaseMockCLI), 0700)
			}
			server := releaseAPIFixture(t, root, scenario)
			write("bin/gh", []byte(`#!/usr/bin/env bash
set -euo pipefail
printf 'gh %s\n' "$*" >> "$TEST_ROOT/trace"
[[ "$1" == api ]] || { echo 'Tag-based release CLI forbidden' >&2; exit 1; }
shift
args=()
for arg in "$@"; do
  case "$arg" in
    repos/*) arg="$API_FIXTURE/$arg";;
    https://uploads.github.com/*) arg="$API_FIXTURE/${arg#https://uploads.github.com/}";;
  esac
  args+=("$arg")
done
exec "$REAL_GH" api "${args[@]}"
`), 0700)
			if scenario == "retry-draft" {
				write("state", []byte("draft"), 0600)
			}
			if scenario == "finalized" {
				write("state", []byte("final"), 0600)
			}
			if scenario == "retry-source" || scenario == "retry-source-bundle" || scenario == "source-mismatch" {
				write("state-source", []byte("final"), 0600)
				old := source
				if scenario == "source-mismatch" {
					old = []byte("different source; must never overwrite")
				}
				write("source-published/obsidian-livesync-portal_1.2.3_source.tar.gz", old, 0600)
				write("source-published/obsidian-livesync-portal_1.2.3_source.tar.gz.sigstore.json", []byte("dummy\n"), 0600)
				if scenario == "retry-source-bundle" {
					write("source-published/obsidian-livesync-portal_1.2.3_source.tar.gz.sigstore.json", []byte("previous valid signature\n"), 0600)
				}
			}
			cmd := exec.Command("bash", "-c", "bash scripts/release-images.sh && bash scripts/release-finalize.sh")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+":"+os.Getenv("PATH"), "GITHUB_REPOSITORY=test/portal", "GITHUB_SHA="+strings.Repeat("a", 40), "GITHUB_REF_NAME=v1.2.3", "GITHUB_RUN_ID=1", "GITHUB_RUN_ATTEMPT=1", "TEST_ROOT="+root, "TEST_CASE="+scenario, "API_FIXTURE="+server.URL, "REAL_GH="+realGH, "GH_TOKEN=fixture-token", "GH_DEBUG=")
			out, err := cmd.CombinedOutput()
			if scenario == "retry-partial" {
				state, _ := os.ReadFile(filepath.Join(root, "state"))
				if err == nil || string(state) != "draft" {
					t.Fatalf("partial upload must fail and remain draft: %v\n%s", err, out)
				}
				retry := exec.Command("bash", "scripts/release-finalize.sh")
				retry.Dir, retry.Env = root, cmd.Env
				out, err = retry.CombinedOutput()
			}
			traceBytes, _ := os.ReadFile(filepath.Join(root, "trace"))
			trace := string(traceBytes)
			wantSuccess := scenario == "success" || scenario == "annotated-tag" || scenario == "retry-draft" || scenario == "retry-source" || scenario == "retry-source-bundle" || scenario == "retry-partial" || scenario == "new-draft-invisible"
			if (err == nil) != wantSuccess {
				t.Fatalf("unexpected result %v\n%s\n%s", err, out, trace)
			}
			if !wantSuccess && scenario != "finalized" {
				state, _ := os.ReadFile(filepath.Join(root, "state"))
				if string(state) == "final" {
					t.Fatal("failure published an application release")
				}
			}
			final := false
			for _, line := range strings.Split(trace, "\n") {
				if strings.Contains(line, "PATCH repos/test/portal/releases/1 ") && strings.Contains(line, "-F draft=false") {
					final = true
				}
			}
			if final != (wantSuccess || scenario == "final-patch-failure") {
				t.Fatalf("incorrect finalization\n%s", trace)
			}
			if strings.HasPrefix(scenario, "update-") || scenario == "persisted-wrong-tag" {
				if strings.Contains(trace, "POST https://uploads.github.com/repos/test/portal/releases/1/") {
					t.Fatal("invalid release update allowed asset upload")
				}
			}
			firstPush := -1
			offset := 0
			for _, line := range strings.Split(trace, "\n") {
				if firstPush < 0 && strings.HasPrefix(line, "goreleaser release ") {
					firstPush = offset
				}
				offset += len(line) + 1
			}
			publicVerified := strings.Index(trace, "/public-source.sigstore.json --certificate-identity")
			if firstPush >= 0 && (publicVerified < 0 || publicVerified > firstPush) {
				t.Fatalf("image push preceded anonymous source bytes/signature verification\n%s", trace)
			}
			switch scenario {
			case "source-public", "source-public-bytes", "source-public-signature", "source-signature", "source-mismatch", "signature", "upload", "legal", "source-legal":
				if firstPush >= 0 {
					t.Fatalf("failed source preparation allowed an image push\n%s", trace)
				}
			}
			if wantSuccess {
				sourceGate := strings.Index(trace, "python3 scripts/license-provenance.py --release --archive")
				if sourceGate < 0 || sourceGate > publicVerified {
					t.Fatal("source publication skipped the strict archive/source gate")
				}
				armSmoke := strings.Index(trace, "docker run --platform linux/arm64")
				sign := strings.Index(trace, "cosign sign --yes")
				draft := strings.Index(trace, "gh api --method POST repos/test/portal/releases -f tag_name=v1.2.3")
				if armSmoke < 0 || sign < armSmoke || draft >= 0 && draft < sign {
					t.Fatalf("unsafe release order\n%s", trace)
				}
				if strings.Count(trace, "goreleaser release ") != 1 {
					t.Fatalf("images rebuilt after testing\n%s", trace)
				}
				if strings.Contains(trace, "imagetools inspect") {
					t.Fatal("mutable tag was re-resolved")
				}
				publicSource := strings.Index(trace, "curl --disable")
				if publicSource < 0 || draft >= 0 && publicSource > draft {
					t.Fatal("binary draft created before public source verification")
				}
				if scenario == "retry-draft" && (draft >= 0 || strings.Contains(trace, "--method DELETE")) {
					t.Fatal("draft retry created a release or deleted immutable assets")
				}
				if scenario == "retry-source" && strings.Contains(trace, "gh release upload source-") {
					t.Fatal("immutable source assets were overwritten")
				}
			} else if scenario == "smoke-arm64" {
				if strings.Contains(trace, "gh release create v1.2.3") {
					t.Fatal("failed smoke created an application release")
				}
				state, _ := os.ReadFile(filepath.Join(root, "state-source"))
				if string(state) != "final" {
					t.Fatal("source must remain published after image failure")
				}
			}
			if scenario == "legal" && (strings.Contains(trace, "docker ") || strings.Contains(trace, "gh ")) {
				t.Fatal("legal gate did not precede external CLI operations")
			}
		})
	}
}

// These CLIs simulate ordering and failure responses, never Docker execution or cryptography.
const releaseMockCLI = `#!/usr/bin/env bash
set -euo pipefail
name=$(basename "$0")
printf '%s %s\n' "$name" "$*" >> "$TEST_ROOT/trace"
case "$name" in
goreleaser)
  mkdir -p dist
  digest="sha256:$(sha256sum "$TEST_ROOT/manifest-fixture" | awk '{print $1}')"
  [[ "$TEST_CASE" != metadata ]] || digest=invalid
  printf '[{"type":"Docker Manifest","extra":{"ID":"portal","Digest":"%s"}}]\n' "$digest" > dist/artifacts.json
  exit 0;;
crane)
  case "$1" in
    manifest) cat "$TEST_ROOT/manifest-fixture";;
    export) : > "$3";;
    digest)
      if [[ "$TEST_CASE" == promotion ]]; then printf 'sha256:wrong\n'; else printf 'sha256:%s\n' "$(sha256sum "$TEST_ROOT/manifest-fixture" | awk '{print $1}')"; fi;;
    copy) :;;
    *) exit 1;;
  esac
  exit 0;;
python3)
  [[ "$TEST_CASE" != legal ]] || exit 1
  if [[ "$TEST_CASE" == source-legal && "$*" == *--release* && "$*" != *--images* ]]; then exit 1; fi
  if [[ "$TEST_CASE" == image-legal && "$*" == *--images* ]]; then exit 1; fi
  exit 0;;
go) exit 0;;
curl)
  [[ "$TEST_CASE" != source-public ]] || exit 1
  if [[ "$TEST_CASE" == source-late ]]; then
    count=0; if [[ -f "$TEST_ROOT/public-count" ]]; then count=$(cat "$TEST_ROOT/public-count"); fi
    count=$((count+1)); printf '%s' "$count" > "$TEST_ROOT/public-count"; [[ "$count" -lt 3 ]] || exit 1
  fi
  payload="$TEST_ROOT/source-published/obsidian-livesync-portal_1.2.3_source.tar.gz"
  if [[ "${!#}" == *.sigstore.json ]]; then payload="$payload.sigstore.json"; fi
  copied=false
  for ((i=1;i<=$#;i++)); do
    if [[ "${!i}" == --output ]]; then
      j=$((i+1)); if [[ "$TEST_CASE" == source-public-bytes && "${!#}" != *.sigstore.json ]]; then printf 'wrong anonymous source bytes' > "${!j}"; else cp "$payload" "${!j}"; fi
      copied=true
    fi
  done
  if [[ "$copied" == false ]]; then cat "$payload"; fi
  exit 0;;
cosign)
  if [[ "$1" == verify-blob ]]; then
    [[ "$TEST_CASE" != source-signature ]] || exit 1
    if [[ "$TEST_CASE" == source-public-signature && "$*" == *'/public-source.sigstore.json '* ]]; then exit 1; fi
  fi
  if [[ "$1" == sign-blob ]]; then
    [[ "$TEST_CASE" != signature ]] || exit 1
    for ((i=1;i<=$#;i++)); do if [[ "${!i}" == --bundle ]]; then j=$((i+1)); printf 'dummy\n' > "${!j}"; fi; done
  fi
  exit 0;;
docker)
  if [[ "$1 $2" == 'image inspect' ]]; then printf '65532:65532\n'; exit 0; fi
  if [[ "$1 $2" == 'volume create' ]]; then printf 'test-volume\n'; exit 0; fi
  if [[ "$1" == run && "$*" == *linux/arm64* && "$TEST_CASE" == smoke-arm64 ]]; then exit 1; fi
  exit 0;;
esac
exit 1
`
