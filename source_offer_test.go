// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"strings"
	"testing"
)

func TestVersionedSourceOffer(t *testing.T) {
	for _, version := range []string{"1.0.0", "1.2.3-rc.1", "1.2.3+build.1"} {
		url := releaseSourceURL(version, strings.Repeat("a", 40))
		if !strings.Contains(url, "/releases/download/source-"+strings.Repeat("a", 40)+"/") || !strings.HasSuffix(url, "_"+version+"_source.tar.gz") || !validEndpoint(url, false) {
			t.Fatal("incorrect exact source offer", url)
		}
	}
	for _, version := range []string{"development", "0.0.0-SNAPSHOT-none", "../main", "1.0.0/other"} {
		if releaseSourceURL(version, strings.Repeat("a", 40)) != "" {
			t.Fatal("development source offer must be explicit")
		}
	}
}

func TestSourceOfferVisibleInNetworkUI(t *testing.T) {
	a := testApp(t)
	a.Config.SourceURL = releaseSourceURL("1.0.0", strings.Repeat("a", 40))
	w := call(a, "/", "GET", nil, "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<footer><a href=\""+a.Config.SourceURL+"\">Source code</a>") {
		t.Fatal("network UI source offer missing")
	}
}
