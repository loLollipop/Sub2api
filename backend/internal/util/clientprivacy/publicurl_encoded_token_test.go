package clientprivacy

import (
	"strings"
	"testing"
)

func TestTextRedactsEncodedCredentialsInPublicURLPaths(t *testing.T) {
	for _, prefix := range []string{"%73%6b-", "%2573%256b-"} {
		link := "https://platform.openai.com/account/api-keys/" + prefix + strings.Repeat("synthetic", 4)
		if got := Text("see " + link); got != "see [redacted]" {
			t.Fatalf("encoded credential survived public URL exemption: %q", got)
		}
	}
}
