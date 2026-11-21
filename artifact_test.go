// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestInstalledArtifact(t *testing.T) {
	binary := os.Getenv("PORTAL_INSTALLED_TEST_BINARY")
	if binary == "" {
		t.Skip("set PORTAL_INSTALLED_TEST_BINARY to an extracted release executable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := sampleConfig().People["alice"].Credentials
	input, _ := json.Marshal(c)
	cmd := exec.CommandContext(ctx, binary, "setup-uri")
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal("installed single-binary setup generation", err)
	}
	var result setupResult
	if json.Unmarshal(out, &result) != nil || result.Recovery != c.Recovery || result.Password == "" {
		t.Fatal("installed artifact setup contract")
	}
	raw, err := DecryptPersistentSetup(result.URI, result.Password)
	if err != nil || raw["couchDB_PASSWORD"] != c.Password || raw["passphrase"] != c.Passphrase {
		t.Fatal("installed artifact URI decryption", err)
	}
	if os.Getenv("SETUP_INTEROP_REQUIRED") == "1" {
		setupOracle(t, map[string]any{"result": PersistentSetupResult{SetupURI: result.URI, SetupPassphrase: result.Password}})
	}
	if expected := os.Getenv("PORTAL_EXPECTED_SOURCE_URL"); expected != "" {
		dir := t.TempDir()
		file := filepath.Join(dir, "config.json")
		body, _ := json.Marshal(sampleConfig())
		if err := os.WriteFile(file, body, 0600); err != nil {
			t.Fatal(err)
		}
		db := filepath.Join(dir, "config.db")
		cmd = exec.CommandContext(ctx, binary, "configure", file)
		cmd.Env = append(os.Environ(), "PORTAL_DB="+db)
		if err := cmd.Run(); err != nil {
			t.Fatal("versioned binary configuration", err)
		}
		configured, err := loadConfig("sqlite:" + db)
		if err != nil || configured.SourceURL != expected {
			t.Fatal("binary source offer does not match deployed revision", err)
		}
	}
	testInstalledStartup(t, binary)
}
