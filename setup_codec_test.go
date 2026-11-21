// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func setupFixture() map[string]string {
	return map[string]string{"LIVESYNC_HOST": "https://example.test", "LIVESYNC_USER": "synthetic", "LIVESYNC_PASS": "not-a-real-password", "LIVESYNC_DB": "vault", "LIVESYNC_PASSPHRASE": "synthetic-e2ee", "id_recovery_code": "sls-id-v1:" + strings.Repeat("a", 64), "uri_passphrase": "synthetic-transport", "uri_mode": "persistent"}
}

func TestSetupKnownAnswer(t *testing.T) {
	// Independently calculated using WebCrypto in setup/known_answer.ts.
	const known = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0+P0BBQkNERUZHSElKSy37Cm4k78JPQmCM6xx9EWAdeCtiLNPdLaB6FamNcoOwxrzxpGE="
	header := make([]byte, 76)
	for i := range header {
		header[i] = byte(i)
	}
	raw := []byte(`{"fixture":"雪! +%"}`)
	uri, err := encryptSetupPayload(raw, "pāss🔐!+%", bytes.NewReader(header))
	if err != nil {
		t.Fatal(err)
	}
	if uri != setupURIBase+setupURIComponent("%$"+known) {
		t.Fatal("known-answer frame mismatch")
	}
	got, err := decryptSetupPayload(uri, "pāss🔐!+%")
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("known-answer decrypt failed")
	}
}

func TestSetupStableRemint(t *testing.T) {
	f := setupFixture()
	a, err := GeneratePersistentSetup(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratePersistentSetup(f)
	if err != nil {
		t.Fatal(err)
	}
	if a.SetupURI == b.SetupURI || a.IDRecoveryCode != b.IDRecoveryCode {
		t.Fatal("randomness or recovery changed")
	}
	x, err := DecryptPersistentSetup(a.SetupURI, a.SetupPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	y, err := DecryptPersistentSetup(b.SetupURI, b.SetupPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(x) != 26 {
		t.Fatalf("payload property count: %d", len(x))
	}
	if x["activeConfigurationId"] == y["activeConfigurationId"] {
		t.Fatal("remote ID was reused")
	}
	if err := normalizeSetupRemote(x); err != nil {
		t.Fatal(err)
	}
	if err := normalizeSetupRemote(y); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(x, y) {
		t.Fatal("stable payload changed")
	}
	delete(f, "id_recovery_code")
	delete(f, "uri_passphrase")
	r, err := GeneratePersistentSetup(f)
	if err != nil {
		t.Fatal(err)
	}
	if !setupRecoveryValid(r.IDRecoveryCode) || len(r.SetupPassphrase) != 48 {
		t.Fatal("unexpected secret format")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GenerateNativeSetup(ctx, credentials{}); err == nil {
		t.Fatal("cancelled context accepted")
	}
	native, err := GenerateNativeSetup(context.Background(), credentials{Host: f["LIVESYNC_HOST"], User: f["LIVESYNC_USER"], Password: f["LIVESYNC_PASS"], Database: f["LIVESYNC_DB"], Passphrase: f["LIVESYNC_PASSPHRASE"], Recovery: a.IDRecoveryCode})
	if err != nil {
		t.Fatal(err)
	}
	if native.Recovery != a.IDRecoveryCode || len(native.Password) != 48 {
		t.Fatal("native response mismatch")
	}
	if _, err := DecryptPersistentSetup(native.URI, native.Password); err != nil {
		t.Fatal(err)
	}
}

func normalizeSetupRemote(m map[string]any) error {
	remotes, ok := m["remoteConfigurations"].(map[string]any)
	if !ok || len(remotes) != 1 {
		return errors.New("expected exactly one saved remote")
	}
	for key, v := range remotes {
		remote, ok := v.(map[string]any)
		if !ok || key == "" || remote["id"] != key || m["activeConfigurationId"] != key {
			return errors.New("saved remote key, id and active reference must match")
		}
		// Validate linkage before erasing nondeterminism, never repairing it.
		remote["id"] = "remote-normalized"
		m["remoteConfigurations"] = map[string]any{"remote-normalized": remote}
	}
	m["activeConfigurationId"] = "remote-normalized"
	return nil
}

func TestSetupRemoteNormalizationRejectsBrokenReferences(t *testing.T) {
	fixture := func() map[string]any {
		return map[string]any{"activeConfigurationId": "remote-original", "remoteConfigurations": map[string]any{"remote-original": map[string]any{"id": "remote-original"}}}
	}
	for _, mode := range []string{"active-reference", "remote-id", "map-key", "no-remotes", "multiple-remotes"} {
		t.Run(mode, func(t *testing.T) {
			broken := fixture()
			remotes := broken["remoteConfigurations"].(map[string]any)
			switch mode {
			case "active-reference":
				broken["activeConfigurationId"] = "remote-original-broken-reference"
			case "remote-id":
				remotes["remote-original"].(map[string]any)["id"] = "remote-other"
			case "map-key":
				remotes["remote-other"] = remotes["remote-original"]
				delete(remotes, "remote-original")
			case "no-remotes":
				delete(remotes, "remote-original")
			case "multiple-remotes":
				remotes["remote-other"] = map[string]any{"id": "remote-other"}
			}
			before, _ := json.Marshal(broken)
			if err := normalizeSetupRemote(broken); err == nil {
				t.Fatal("normalization accepted broken remote linkage")
			}
			after, _ := json.Marshal(broken)
			if !bytes.Equal(before, after) {
				t.Fatal("normalization repaired or mutated rejected input")
			}
		})
	}
	valid := fixture()
	if err := normalizeSetupRemote(valid); err != nil {
		t.Fatal(err)
	}
	remote := valid["remoteConfigurations"].(map[string]any)["remote-normalized"].(map[string]any)
	if valid["activeConfigurationId"] != "remote-normalized" || remote["id"] != "remote-normalized" {
		t.Fatal("valid remote was not normalized")
	}
}

func TestSetupMalformedAndTampered(t *testing.T) {
	r, err := GeneratePersistentSetup(setupFixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptPersistentSetup(r.SetupURI, "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	encoded, _ := url.PathUnescape(strings.TrimPrefix(r.SetupURI, setupURIBase))
	frame, _ := base64.StdEncoding.DecodeString(encoded[2:])
	uriFor := func(b []byte) string {
		return setupURIBase + setupURIComponent("%$"+base64.StdEncoding.EncodeToString(b))
	}
	for _, n := range []int{0, 1, 31, 32, 33, 43, 44, 45, 75, 76, 77, 91, 92, len(frame) - 17, len(frame) - 16, len(frame) - 1} {
		if _, err := DecryptPersistentSetup(uriFor(frame[:n]), r.SetupPassphrase); err == nil {
			t.Fatalf("accepted truncation %d", n)
		}
	}
	for _, i := range []int{0, 31, 32, 43, 44, 75, 76, len(frame) - 17, len(frame) - 16, len(frame) - 1} {
		copyFrame := bytes.Clone(frame)
		copyFrame[i] ^= 1
		if _, err := DecryptPersistentSetup(uriFor(copyFrame), r.SetupPassphrase); err == nil {
			t.Fatalf("accepted corruption %d", i)
		}
	}
	for _, bad := range []string{"", setupURIBase + "%", setupURIBase + "%GG", setupURIBase + "%25%3Daaa", setupURIBase + "%25%24!!!!", r.SetupURI + "&x=y", r.SetupURI + "#x", r.SetupURI + "%0A", strings.Repeat("x", setupMaxURI+1)} {
		if _, err := DecryptPersistentSetup(bad, r.SetupPassphrase); err == nil {
			t.Fatal("accepted malformed URI")
		}
	}
	for _, password := range []string{"", strings.Repeat("p", setupMaxPassword+1), string([]byte{0xff})} {
		if _, err := DecryptPersistentSetup(r.SetupURI, password); err == nil {
			t.Fatal("accepted invalid password")
		}
	}
	// Normalization and whitespace are not applied by the cryptographic codec.
	unicodeURI, err := encryptSetupPayload([]byte(`{"test":true}`), " é ", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"é", " e\u0301 "} {
		if _, err := decryptSetupPayload(unicodeURI, password); err == nil {
			t.Fatal("password was normalized or trimmed")
		}
	}
	if _, err := encryptSetupPayload(make([]byte, setupMaxPayload+1), "p", rand.Reader); err == nil {
		t.Fatal("oversize payload accepted")
	}
}

func TestSetupVersionAndUnicodeRejection(t *testing.T) {
	for _, raw := range []string{
		`{"idDerivationVersion":2,"idDerivationKey":"` + strings.Repeat("a", 64) + `"}`,
		`{"idDerivationVersion":1,"idDerivationKey":"bad"}`,
		`{"idDerivationVersion":1,"idDerivationKey":"` + strings.Repeat("a", 64) + `","pathNormalizationVersion":1}`,
		`{"idDerivationVersion":1,"idDerivationKey":"` + strings.Repeat("a", 64) + `","bad":"\ud800"}`,
		`{"idDerivationVersion":1,"idDerivationKey":"` + strings.Repeat("a", 64) + `","bad":"\udc00"}`,
	} {
		uri, err := encryptSetupPayload([]byte(raw), "p", rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecryptPersistentSetup(uri, "p"); err == nil {
			t.Fatal("unsupported or lossy payload accepted")
		}
	}
	for _, raw := range []string{`"\ud800"`, `"\udc00"`, `"\ud800\u0041"`, string([]byte{0xff})} {
		if setupJSONUnicodeValid([]byte(raw)) {
			t.Fatal("invalid Unicode accepted")
		}
	}
	for _, raw := range []string{`"\ud83d\udd10"`, `"\\ud800"`, `"雪"`} {
		if !setupJSONUnicodeValid([]byte(raw)) {
			t.Fatal("valid Unicode rejected")
		}
	}
	for k, v := range map[string]string{"uri_mode": "ephemeral", "remote_type": "s3", "id_mode": "legacy", "id_recovery_code": "sls-id-v2:" + strings.Repeat("a", 64), "LIVESYNC_USER": string([]byte{0xff})} {
		f := setupFixture()
		f[k] = v
		if _, err := GeneratePersistentSetup(f); err == nil {
			t.Fatalf("accepted unsupported %s", k)
		}
	}
	for _, host := range []string{"https://u:p@example.test", "https://example.test?q=x", "https://example.test#fragment", "https://example.test/a/../b", "https://example.test/%2e%2e/b", "https://example.test/a\\b", "https://127.1", "https://0x7f000001", "https://example.test:", "https://example.test:0", "https://☃.test", "https://example.test/%ff"} {
		f := setupFixture()
		f["LIVESYNC_HOST"] = host
		if _, err := GeneratePersistentSetup(f); err == nil {
			t.Fatalf("accepted ambiguous endpoint %q", host)
		}
	}
	if setupTrim("\u0085x\u0085") != "\u0085x\u0085" || setupTrim("\ufeff x \u00a0") != "x" {
		t.Fatal("ECMAScript trim mismatch")
	}
	var result PersistentSetupResult
	if err := json.Unmarshal([]byte(`{"mode":"persistent","usableUntil":null}`), &result); err != nil || result.UsableUntil != nil {
		t.Fatal("result JSON mismatch")
	}
}
