// SPDX-License-Identifier: AGPL-3.0-only
// Persistent CouchDB setup protocol adapted from livesync-commonlib 0.2.0
// (Copyright (c) 2022 vorotamoroz, MIT), octagonal-wheels 0.1.57
// (Copyright (c) 2024 vorotamoroz, MIT), and the upstream Setup URI generator
// (Copyright (c) 2021 vorotamoroz, MIT). Their MIT notices are retained in
// licenses/dependencies/npm-_vrtmrz_livesync-commonlib_0.2.0-LICENSE,
// licenses/dependencies/npm-octagonal-wheels_0.1.57-LICENSE, and
// setup/upstream/LICENSE. No upstream runtime is required.
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const setupURIBase = "obsidian://setuplivesync?settings="
const setupMaxPayload = 64 << 10
const setupMaxPassword = 4096

// Base64's 4/3 expansion followed by worst-case percent encoding (3x).
const setupMaxURI = 4 * (setupMaxPayload + 128)

var errSetup = errors.New("invalid or unsupported persistent CouchDB setup")

// GenerateNativeSetup is the in-process replacement for the external helper.
// PBKDF2 is bounded but not interruptible; cancellation is checked around it.
func GenerateNativeSetup(ctx context.Context, c credentials) (setupResult, error) {
	if err := ctx.Err(); err != nil {
		return setupResult{}, err
	}
	password := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, password); err != nil {
		return setupResult{}, errSetup
	}
	r, err := GeneratePersistentSetup(map[string]string{
		"LIVESYNC_HOST": c.Host, "LIVESYNC_USER": c.User, "LIVESYNC_PASS": c.Password,
		"LIVESYNC_DB": c.Database, "LIVESYNC_PASSPHRASE": c.Passphrase,
		"id_recovery_code": c.Recovery, "uri_passphrase": hex.EncodeToString(password),
		"uri_mode": "persistent", "remote_type": "couchdb", "id_mode": "random",
	})
	if err != nil {
		return setupResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return setupResult{}, err
	}
	return setupResult{URI: r.SetupURI, Password: r.SetupPassphrase, Recovery: r.IDRecoveryCode}, nil
}

// PersistentSetupResult mirrors the upstream generator's JSON response.
type PersistentSetupResult struct {
	RemoteType      string `json:"remoteType"`
	SetupURI        string `json:"setupURI"`
	SetupPassphrase string `json:"setupPassphrase"`
	Mode            string `json:"mode"`
	UsableUntil     *int64 `json:"usableUntil"`
	IDRecoveryCode  string `json:"idRecoveryCode"`
}

// GeneratePersistentSetup deliberately supports neither ephemeral/legacy modes
// nor S3/P2P settings. Omitted mode means persistent, unlike upstream's default.
// Values follow ECMAScript trim, without Unicode normalization.
func GeneratePersistentSetup(env map[string]string) (PersistentSetupResult, error) {
	for k, v := range env {
		if !utf8.ValidString(k) || !utf8.ValidString(v) || len(v) > setupMaxPassword {
			return PersistentSetupResult{}, errSetup
		}
	}
	for k, want := range map[string]string{"uri_mode": "persistent", "remote_type": "couchdb", "id_mode": "random"} {
		if v := strings.ToLower(setupTrim(env[k])); v != "" && v != want {
			return PersistentSetupResult{}, errSetup
		}
	}
	get := func(primary, fallback string) string {
		if s := setupTrim(env[primary]); s != "" {
			return s
		}
		return setupTrim(env[fallback])
	}
	host, user, pass := get("LIVESYNC_HOST", "hostname"), get("LIVESYNC_USER", "username"), get("LIVESYNC_PASS", "password")
	db, e2ee := get("LIVESYNC_DB", "database"), get("LIVESYNC_PASSPHRASE", "passphrase")
	if host == "" || user == "" || pass == "" || db == "" || e2ee == "" {
		return PersistentSetupResult{}, errSetup
	}
	endpoint, nameHost, savedHost, path, err := setupEndpoint(host)
	if err != nil {
		return PersistentSetupResult{}, err
	}
	recovery := setupTrim(env["id_recovery_code"])
	if recovery == "" {
		b := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, b); err != nil {
			return PersistentSetupResult{}, errSetup
		}
		recovery = "sls-id-v1:" + hex.EncodeToString(b)
	}
	if !setupRecoveryValid(recovery) {
		return PersistentSetupResult{}, errSetup
	}
	password := setupTrim(env["uri_passphrase"])
	if password == "" {
		b := make([]byte, 24)
		if _, err := io.ReadFull(rand.Reader, b); err != nil {
			return PersistentSetupResult{}, errSetup
		}
		password = hex.EncodeToString(b)
	}
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return PersistentSetupResult{}, errSetup
	}
	id := "remote-" + hex.EncodeToString(b)
	scheme := strings.SplitN(endpoint, ":", 2)[0]
	connection := "sls+" + scheme + "://" + setupURIComponent(user) + ":" + setupURIComponent(pass) + "@" + savedHost + path + "?db=" + setupFormEncode(db)
	settings := map[string]any{
		"couchDB_URI": endpoint, "couchDB_USER": user, "couchDB_PASSWORD": pass, "couchDB_DBNAME": db,
		"syncOnStart": true, "periodicReplication": true, "syncOnFileOpen": true,
		"encrypt": true, "passphrase": e2ee, "idDerivationVersion": 1, "idDerivationKey": recovery[len("sls-id-v1:"):],
		"usePathObfuscation": true, "encryptInternalMetadata": true, "batchSave": true, "syncAfterMerge": true,
		"remoteConfigurations":  map[string]any{id: map[string]any{"id": id, "name": "CouchDB " + nameHost, "uri": connection, "isEncrypted": false}},
		"activeConfigurationId": id, "concurrencyOfReadChunksOnline": 30, "minimumIntervalOfReadChunksOnline": 25,
		"isConfigured": true, "usePluginSyncV2": true, "handleFilenameCaseSensitive": false,
		"configPassphraseStore": "", "encryptedCouchDBConnection": "", "encryptedPassphrase": "", "encryptedIdDerivationKey": "",
	}
	// Activation parses the saved connection through a https proxy URL, so even
	// an http endpoint's explicit :443 disappears in the upstream implementation.
	settings["couchDB_URI"] = scheme + "://" + savedHost
	if path != "/" {
		settings["couchDB_URI"] = scheme + "://" + savedHost + path
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return PersistentSetupResult{}, errSetup
	}
	uri, err := encryptSetupPayload(raw, password, rand.Reader)
	if err != nil {
		return PersistentSetupResult{}, err
	}
	return PersistentSetupResult{"couchdb", uri, password, "persistent", nil, recovery}, nil
}

func setupRecoveryValid(s string) bool {
	if len(s) != 74 || !strings.HasPrefix(s, "sls-id-v1:") {
		return false
	}
	for _, c := range s[10:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func setupTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r' || r == ' ' || r == 0xa0 || r == 0x1680 || r >= 0x2000 && r <= 0x200a || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000 || r == 0xfeff
	})
}

// Only unambiguous HTTP(S) endpoints are accepted: ASCII DNS/IPv4 or IPv6,
// optional decimal port and a path. No auth, query, fragment, zones, IDNA,
// alternate IPv4 notation, dot segments, controls or backslashes.
func setupEndpoint(s string) (endpoint, nameHost, savedHost, path string, err error) {
	u, e := url.Parse(s)
	if e != nil || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(s, "\\\r\n\t#?") {
		err = errSetup
		return
	}
	h := strings.ToLower(u.Hostname())
	if strings.Contains(h, ":") {
		ip, e := netip.ParseAddr(h)
		if e != nil || !ip.Is6() || ip.Is4In6() || ip.Zone() != "" {
			err = errSetup
			return
		}
		h = "[" + ip.String() + "]"
	} else {
		if h == "" || strings.HasSuffix(h, ".") {
			err = errSetup
			return
		}
		for _, r := range h {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
				err = errSetup
				return
			}
		}
		for _, label := range strings.Split(h, ".") {
			if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				err = errSetup
				return
			}
		}
		last := h[strings.LastIndex(h, ".")+1:]
		if (last[0] >= '0' && last[0] <= '9') || strings.HasPrefix(last, "0x") {
			ip, e := netip.ParseAddr(h)
			if e != nil || !ip.Is4() || ip.String() != h {
				err = errSetup
				return
			}
		}
	}
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		err = errSetup
		return
	}
	if port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			err = errSetup
			return
		}
		port = strconv.Itoa(n)
	}
	if u.Scheme == "http" && port == "80" || u.Scheme == "https" && port == "443" {
		port = ""
	}
	nameHost = h
	if port != "" {
		nameHost = net.JoinHostPort(strings.Trim(h, "[]"), port)
	}
	savedHost = nameHost
	if port == "443" {
		savedHost = h
	}
	if !utf8.ValidString(u.Path) {
		err = errSetup
		return
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "." || seg == ".." {
			err = errSetup
			return
		}
	}
	// Use original spelling: re-encoding the decoded Go Path would lose valid
	// percent escapes; EscapedPath also escapes more punctuation than WHATWG.
	authorityAndPath := s[strings.Index(s, "://")+3:]
	path = "/"
	if start := strings.IndexByte(authorityAndPath, '/'); start >= 0 {
		path = setupPathEncode(authorityAndPath[start:])
	}
	for _, r := range path {
		if r < 0x21 || r >= 0x7f || strings.ContainsRune("\"<>`{}", r) {
			err = errSetup
			return
		}
	}
	endpoint = u.Scheme + "://" + nameHost
	if path != "/" {
		endpoint += path
	}
	return
}

func setupPercent(s string, safe func(byte) bool, form bool) string {
	const hexChars = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if safe(c) {
			b.WriteByte(c)
		} else if form && c == ' ' {
			b.WriteByte('+')
		} else {
			b.WriteByte('%')
			b.WriteByte(hexChars[c>>4])
			b.WriteByte(hexChars[c&15])
		}
	}
	return b.String()
}
func setupAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
func setupURIComponent(s string) string {
	return setupPercent(s, func(c byte) bool { return setupAlpha(c) || strings.ContainsRune("-_.!~*'()", rune(c)) }, false)
}
func setupFormEncode(s string) string {
	return setupPercent(s, func(c byte) bool { return setupAlpha(c) || strings.ContainsRune("*-._", rune(c)) }, true)
}
func setupPathEncode(s string) string {
	return setupPercent(s, func(c byte) bool { return c >= 0x21 && c < 0x7f && !strings.ContainsRune("\"#<>?`{}", rune(c)) }, false)
}

func setupAEAD(password string, salt, hkdfSalt []byte) (cipher.AEAD, error) {
	master, err := pbkdf2.Key(sha256.New, password, salt, 310000, 32)
	if err != nil {
		return nil, errSetup
	}
	key, err := hkdf.Key(sha256.New, master, hkdfSalt, "", 32)
	if err != nil {
		return nil, errSetup
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errSetup
	}
	return cipher.NewGCM(block)
}

func encryptSetupPayload(raw []byte, password string, random io.Reader) (string, error) {
	if len(raw) == 0 || len(raw) > setupMaxPayload || !utf8.Valid(raw) || len(password) == 0 || len(password) > setupMaxPassword || !utf8.ValidString(password) {
		return "", errSetup
	}
	header := make([]byte, 76)
	if _, err := io.ReadFull(random, header); err != nil {
		return "", errSetup
	}
	aead, err := setupAEAD(password, header[:32], header[44:76])
	if err != nil {
		return "", err
	}
	frame := aead.Seal(header, header[32:44], raw, nil)
	return setupURIBase + setupURIComponent("%$"+base64.StdEncoding.EncodeToString(frame)), nil
}

// DecryptPersistentSetup returns the complete raw sparse payload, not defaults
// added by upstream's decoder. It accepts only the current %$ frame and ID v1.
// Bounds are checked before the expensive KDF; authentication errors are opaque.
func DecryptPersistentSetup(uri, password string) (map[string]any, error) {
	raw, err := decryptSetupPayload(uri, password)
	if err != nil {
		return nil, err
	}
	if !setupJSONUnicodeValid(raw) {
		return nil, errSetup
	}
	var settings map[string]any
	if json.Unmarshal(raw, &settings) != nil || settings == nil {
		return nil, errSetup
	}
	if settings["idDerivationVersion"] != float64(1) {
		return nil, errSetup
	}
	key, ok := settings["idDerivationKey"].(string)
	if !ok || !setupRecoveryValid("sls-id-v1:"+key) {
		return nil, errSetup
	}
	if v, ok := settings["pathNormalizationVersion"]; ok && v != float64(0) {
		return nil, errSetup
	}
	return settings, nil
}

func decryptSetupPayload(uri, password string) ([]byte, error) {
	if len(uri) > setupMaxURI || !strings.HasPrefix(uri, setupURIBase) || len(password) == 0 || len(password) > setupMaxPassword || !utf8.ValidString(password) {
		return nil, errSetup
	}
	encoded, err := url.PathUnescape(strings.TrimPrefix(uri, setupURIBase))
	if err != nil || !strings.HasPrefix(encoded, "%$") {
		return nil, errSetup
	}
	b64 := encoded[2:]
	if len(b64) > base64.StdEncoding.EncodedLen(setupMaxPayload+92) || strings.ContainsAny(b64, "\r\n") {
		return nil, errSetup
	}
	frame, err := base64.StdEncoding.Strict().DecodeString(b64)
	if err != nil || len(frame) < 93 || len(frame) > setupMaxPayload+92 {
		return nil, errSetup
	}
	aead, err := setupAEAD(password, frame[:32], frame[44:76])
	if err != nil {
		return nil, errSetup
	}
	raw, err := aead.Open(nil, frame[32:44], frame[76:], nil)
	if err != nil || !utf8.Valid(raw) {
		return nil, errSetup
	}
	return raw, nil
}

// encoding/json replaces invalid surrogate escapes instead of rejecting them.
// Scan escapes first so no lossy Unicode input can enter the protocol.
func setupJSONUnicodeValid(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		n, e := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if e != nil {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return false
			}
			m, e := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
			if e != nil || m < 0xdc00 || m > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
