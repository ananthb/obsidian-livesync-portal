// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

type setupOracleResult struct {
	Result     PersistentSetupResult `json:"result"`
	Raw        map[string]any        `json:"raw"`
	Decoded    map[string]any        `json:"decoded"`
	Connection map[string]any        `json:"connection"`
}

func setupOracle(t *testing.T, input any) setupOracleResult {
	t.Helper()
	deno := os.Getenv("SETUP_INTEROP_DENO")
	if deno == "" {
		deno = "deno"
	}
	if _, err := exec.LookPath(deno); err != nil {
		if os.Getenv("SETUP_INTEROP_REQUIRED") == "1" {
			t.Fatal("Deno required for upstream interop")
		}
		t.Skip("test-only Deno unavailable; set SETUP_INTEROP_REQUIRED=1 in interop CI")
	}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(deno, "run", "--frozen", "--cached-only", "--allow-env=__MINIMATCH_TESTING_PLATFORM__", "setup/interop.ts")
	cmd.Stdin = bytes.NewReader(b)
	// Upstream failures may contain credentials; do not forward diagnostics.
	out, err := cmd.Output()
	if err != nil {
		t.Fatal("pinned upstream interop process failed")
	}
	var r setupOracleResult
	if json.Unmarshal(out, &r) != nil {
		t.Fatal("invalid upstream oracle output")
	}
	return r
}

func TestSetupUpstreamInteroperability(t *testing.T) {
	// Complete raw payload comparison, normalizing only the random remote ID.
	// Decoder-added defaults are compared independently, never merged into raw.
	hosts := []string{"https://example.test", "http://EXAMPLE.test:80/", "https://EXAMPLE.test:443/proxy/", "http://example.test:443/base", "http://example.test:5984/prefix%20one/%2Fkeep/", "https://[2001:0db8::1]:8443/path", "http://[::1]:80/", "https://example.test/雪/!$&'()*+,;=:@[]|^", "https://127.0.0.1:443/a%25b"}
	passwords := []string{"plain", "雪🔐e\u0301", "!#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", "\ufeff transport \u00a0", "\u0085not-trimmed\u0085"}
	rng := rand.New(rand.NewPCG(5, 17))
	for i := 0; i < 4; i++ {
		runes := []rune("abcXYZ09雪🔐é!#$%&'()*+,-./:;<=>?@[\\]^_`{|}~")
		s := make([]rune, 32)
		for j := range s {
			s[j] = runes[rng.IntN(len(runes))]
		}
		passwords = append(passwords, string(s))
	}
	for i, host := range hosts {
		t.Run(host, func(t *testing.T) {
			f := setupFixture()
			f["LIVESYNC_HOST"] = host
			f["uri_passphrase"] = passwords[i%len(passwords)]
			f["LIVESYNC_USER"] = "\ufeff 雪!$&'()*+,;=:@[]|^%/?# user \u00a0"
			f["LIVESYNC_PASS"] = " p🔐 !+%/'() "
			f["LIVESYNC_DB"] = "vault !~*'()+/%雪"
			f["LIVESYNC_PASSPHRASE"] = "\u0085e\u0301🔐\u0085"
			goResult, err := GeneratePersistentSetup(f)
			if err != nil {
				t.Fatal(err)
			}
			goRaw, err := DecryptPersistentSetup(goResult.SetupURI, goResult.SetupPassphrase)
			if err != nil {
				t.Fatal(err)
			}
			// Go -> actual decodeSettingsFromSetupURI plus separate raw decrypt.
			goOracle := setupOracle(t, map[string]any{"result": goResult})
			if !reflect.DeepEqual(goRaw, goOracle.Raw) {
				t.Fatal("Go -> upstream raw mismatch")
			}
			expectedDecoded := make(map[string]any, len(goRaw)+1)
			for k, v := range goRaw {
				expectedDecoded[k] = v
			}
			expectedDecoded["pathNormalizationVersion"] = float64(0)
			if !reflect.DeepEqual(expectedDecoded, goOracle.Decoded) {
				t.Fatal("upstream decoder defaults differ")
			}
			// Upstream -> Go, comparing every sparse setting, including saved URI.
			upstream := setupOracle(t, map[string]any{"environment": f})
			upRaw, err := DecryptPersistentSetup(upstream.Result.SetupURI, upstream.Result.SetupPassphrase)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(upRaw, upstream.Raw) {
				t.Fatal("upstream -> Go raw mismatch")
			}
			if err := normalizeSetupRemote(upRaw); err != nil {
				t.Fatal(err)
			}
			if err := normalizeSetupRemote(goRaw); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(goRaw, upRaw) {
				for k, v := range goRaw {
					if !reflect.DeepEqual(v, upRaw[k]) {
						t.Errorf("payload field %s differs", k)
					}
				}
				t.Fatal("complete raw payload mismatch")
			}
			if !reflect.DeepEqual(goOracle.Connection, upstream.Connection) {
				t.Fatal("saved connection parse differs")
			}
			conn := goOracle.Connection["settings"].(map[string]any)
			for _, k := range []string{"couchDB_URI", "couchDB_USER", "couchDB_PASSWORD", "couchDB_DBNAME"} {
				if !reflect.DeepEqual(conn[k], goRaw[k]) {
					t.Fatalf("saved connection and top-level %s differ", k)
				}
			}
			if goResult.SetupPassphrase != upstream.Result.SetupPassphrase || goResult.IDRecoveryCode != upstream.Result.IDRecoveryCode {
				t.Fatal("result fields differ")
			}
		})
	}
}
