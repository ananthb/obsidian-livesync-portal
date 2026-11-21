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

func TestReleaseOrderingAndFailClosedWithMockedCLIs(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq required for release script simulations")
	}
	for _, scenario := range []string{"success", "retry-draft", "retry-source", "smoke-arm64", "upload", "promotion", "finalized", "moved-tag", "metadata", "signature", "legal", "source-legal", "image-legal", "source-public", "source-public-bytes", "source-public-signature", "source-signature", "source-late", "source-mismatch"} {
		t.Run(scenario, func(t *testing.T) {
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
			for _, name := range []string{"release-common.sh", "release-images.sh", "release-finalize.sh", "publish-source.sh", "image-smoke.sh"} {
				b, err := os.ReadFile("scripts/" + name)
				if err != nil {
					t.Fatal(err)
				}
				write("scripts/"+name, b, 0600)
			}
			write("Dockerfile.release", []byte("FROM scratch\n"), 0600)
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
			for _, name := range []string{"gh", "docker", "cosign", "go", "python3", "curl"} {
				write("bin/"+name, []byte(releaseMockCLI), 0700)
			}
			if scenario == "retry-draft" {
				write("state", []byte("draft"), 0600)
			}
			if scenario == "finalized" {
				write("state", []byte("final"), 0600)
			}
			if scenario == "retry-source" || scenario == "source-mismatch" {
				write("state-source", []byte("final"), 0600)
				old := source
				if scenario == "source-mismatch" {
					old = []byte("different source; must never overwrite")
				}
				write("source-published/obsidian-livesync-portal_1.2.3_source.tar.gz", old, 0600)
				write("source-published/obsidian-livesync-portal_1.2.3_source.tar.gz.sigstore.json", []byte("dummy\n"), 0600)
			}
			cmd := exec.Command("bash", "-c", "bash scripts/release-images.sh && bash scripts/release-finalize.sh")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+":"+os.Getenv("PATH"), "GITHUB_REPOSITORY=test/portal", "GITHUB_SHA="+strings.Repeat("a", 40), "GITHUB_REF_NAME=v1.2.3", "GITHUB_RUN_ID=1", "GITHUB_RUN_ATTEMPT=1", "TEST_ROOT="+root, "TEST_CASE="+scenario)
			out, err := cmd.CombinedOutput()
			traceBytes, _ := os.ReadFile(filepath.Join(root, "trace"))
			trace := string(traceBytes)
			wantSuccess := scenario == "success" || scenario == "retry-draft" || scenario == "retry-source"
			if (err == nil) != wantSuccess {
				t.Fatalf("unexpected result %v\n%s\n%s", err, out, trace)
			}
			final := strings.Contains(trace, "repos/test/portal/releases/1 -F draft=false")
			if final != wantSuccess {
				t.Fatalf("incorrect finalization\n%s", trace)
			}
			firstPush := -1
			offset := 0
			for _, line := range strings.Split(trace, "\n") {
				if firstPush < 0 && strings.HasPrefix(line, "docker buildx build ") && strings.Contains(line, " --push ") {
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
				draft := strings.Index(trace, "gh release create v1.2.3")
				if armSmoke < 0 || sign < armSmoke || draft >= 0 && draft < sign {
					t.Fatalf("unsafe release order\n%s", trace)
				}
				if strings.Count(trace, "docker buildx build ") != 2 {
					t.Fatalf("images rebuilt after testing\n%s", trace)
				}
				if strings.Contains(trace, "imagetools inspect") {
					t.Fatal("mutable tag was re-resolved")
				}
				publicSource := strings.Index(trace, "curl --disable")
				if publicSource < 0 || draft >= 0 && publicSource > draft {
					t.Fatal("binary draft created before public source verification")
				}
				if scenario == "retry-draft" && (draft >= 0 || !strings.Contains(trace, "--method DELETE")) {
					t.Fatal("draft retry did not replace old assets")
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
  if [[ "$1" == create ]]; then printf '%064d\n' 0 | tr 0 c; exit 0; fi
  if [[ "$1" == rm ]]; then exit 0; fi
  if [[ "$1" == export ]]; then : > "$3"; exit 0; fi
  if [[ "$1" == run && "$*" == *linux/arm64* && "$TEST_CASE" == smoke-arm64 ]]; then exit 1; fi
  if [[ "$1 $2" == 'buildx build' || "$1 $2 $3" == 'buildx imagetools create' ]]; then
    meta=''
    for ((i=1;i<=$#;i++)); do if [[ "${!i}" == --metadata-file ]]; then j=$((i+1)); meta=${!j}; fi; done
    char=c
    [[ "$meta" != *image-amd64* ]] || char=a
    [[ "$meta" != *image-arm64* ]] || char=b
    if [[ "$meta" == *promoted* && "$TEST_CASE" == promotion ]]; then char=d; fi
    hash=$(printf '%064d' 0 | tr 0 "$char")
    if [[ "$meta" == *image-* ]]; then
      if [[ "$TEST_CASE" == metadata ]]; then hash=invalid; fi
      printf '{"containerimage.digest":"sha256:%s"}\n' "$hash" > "$meta"
    else printf '{"containerimage.descriptor":{"digest":"sha256:%s"}}\n' "$hash" > "$meta"; fi
  fi
  exit 0;;
gh)
  if [[ "$1" == api ]]; then
    if [[ "$*" == *git/ref/tags* ]]; then
      sha=$GITHUB_SHA; [[ "$TEST_CASE" != moved-tag ]] || sha=$(printf '%040d' 0 | tr 0 b)
      printf '{"object":{"type":"commit","sha":"%s"}}\n' "$sha"
    elif [[ "$*" == *'releases?per_page'* ]]; then
      if [[ "$*" == *env.SOURCE_RELEASE_TAG* ]]; then
        if [[ -f "$TEST_ROOT/state-source" ]]; then state=$(cat "$TEST_ROOT/state-source"); if [[ "$state" == final ]]; then printf '{"id":2,"draft":false}\n'; else printf '{"id":2,"draft":true}\n'; fi; fi
      elif [[ -f "$TEST_ROOT/state" ]]; then state=$(cat "$TEST_ROOT/state"); if [[ "$state" == final ]]; then printf '{"id":1,"draft":false}\n'; else printf '{"id":1,"draft":true}\n'; fi; fi
    elif [[ "$*" == *'/releases/1/assets'* ]]; then printf '17\n'
    elif [[ "$*" == *'/releases/2/assets'* ]]; then
      if [[ -n "${SOURCE_ASSET_NAME:-}" && -f "$TEST_ROOT/source-published/$SOURCE_ASSET_NAME" ]]; then printf '27\n'; fi
    elif [[ "$*" == *'-F draft=false'* ]]; then
      if [[ "$*" == *'/releases/2 '* ]]; then printf final > "$TEST_ROOT/state-source"; else printf final > "$TEST_ROOT/state"; fi
    fi
  elif [[ "$1 $2" == 'release create' ]]; then
    if [[ "$3" == source-* ]]; then printf draft > "$TEST_ROOT/state-source"; else printf draft > "$TEST_ROOT/state"; fi
  elif [[ "$1 $2" == 'release upload' ]]; then
    [[ "$TEST_CASE" != upload ]] || exit 1
    dir="$TEST_ROOT/published"; if [[ "$3" == source-* ]]; then dir="$TEST_ROOT/source-published"; fi
    mkdir -p "$dir"
    for item in "$@"; do [[ "$item" != dist/* ]] || cp "$item" "$dir/"; done
  elif [[ "$1 $2" == 'release download' ]]; then
    dir="$TEST_ROOT/published"; if [[ "$3" == source-* ]]; then dir="$TEST_ROOT/source-published"; fi
    for ((i=1;i<=$#;i++)); do if [[ "${!i}" == --dir ]]; then j=$((i+1)); cp "$dir/"* "${!j}/"; fi; done
  fi
  exit 0;;
esac
exit 1
`
