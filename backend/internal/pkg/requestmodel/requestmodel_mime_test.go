package requestmodel

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromBodyCandidatesJSONWithMultipartContentType(t *testing.T) {
	tests := []struct {
		name       string
		route      string
		body       string
		candidates []string
		conflict   bool
	}{
		{name: "single model", body: `{"model":"allowed"}`, candidates: []string{"allowed"}},
		{name: "different duplicates", body: `{"model":"allowed","model":"blocked"}`, candidates: []string{"allowed", "blocked"}, conflict: true},
		{name: "identical duplicates", body: `{"model":"allowed","model":"allowed"}`, candidates: []string{"allowed", "allowed"}},
		{name: "case variant", body: `{"model":"allowed","Model":"blocked"}`, candidates: []string{"allowed", "blocked"}, conflict: true},
		{name: "escaped key", body: `{"model":"allowed","mo\u0064el":"blocked"}`, candidates: []string{"allowed", "blocked"}, conflict: true},
		{name: "nested input is ignored", body: `{"model":"allowed","input":{"model":"blocked"}}`, candidates: []string{"allowed"}},
		{name: "top level wins", body: `{"model":"allowed","session":{"model":"blocked"}}`, candidates: []string{"allowed"}},
		{name: "live prefers session", route: "/v1/live", body: `{"model":"blocked","session":{"Model":"allowed"}}`, candidates: []string{"allowed"}},
		{name: "session fallback", body: `{"session":{"model":"allowed","model":"blocked"}}`, candidates: []string{"allowed", "blocked"}, conflict: true},
		{name: "empty model falls back", body: `{"model":" ","session":{"model":"allowed"}}`, candidates: []string{"allowed"}},
		{name: "whitespace", body: " \r\n\t{\"model\":\"allowed\"}\r\n ", candidates: []string{"allowed"}},
		{name: "empty", body: ``},
		{name: "empty object", body: `{}`},
		{name: "blank model", body: `{"model":" "}`},
		{name: "array", body: `[{"model":"blocked"}]`},
		{name: "string", body: `"model"`},
		{name: "truncated object", body: `{"model":"blocked"`},
		{name: "trailing bytes", body: `{"model":"blocked"}not-json`},
		{name: "concatenated objects", body: `{"model":"allowed"}{"model":"blocked"}`},
	}
	for _, contentType := range []string{"multipart/form-data; boundary=x", "Multipart/Form-Data; boundary=\"x\"", "multipart/form-data"} {
		t.Run(contentType, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got := FromBodyCandidates(tt.route, contentType, []byte(tt.body))
					require.Equal(t, tt.candidates, got)
					require.Equal(t, tt.conflict, ConflictingModelCandidates(got))
				})
			}
		})
	}
}

func TestFromBodyCandidatesRealMultipartKeepsFieldSelection(t *testing.T) {
	for _, duplicateModel := range []string{"allowed", "blocked"} {
		t.Run(duplicateModel, func(t *testing.T) {
			var body bytes.Buffer
			// A JSON-looking preamble must never be scanned for model candidates.
			_, err := body.WriteString("{\"model\":\"preamble-model\"}\r\n")
			require.NoError(t, err)
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("model", "allowed"))
			require.NoError(t, writer.WriteField("model", duplicateModel))
			require.NoError(t, writer.WriteField("session", `{"model":"session-a","Model":"session-b"}`))
			require.NoError(t, writer.WriteField("session", `{"model":"session-c"}`))
			require.NoError(t, writer.Close())

			got := FromBodyCandidates("/v1/images/edits", writer.FormDataContentType(), body.Bytes())
			require.Equal(t, []string{"allowed", duplicateModel}, got)
			require.Equal(t, duplicateModel != "allowed", ConflictingModelCandidates(got))
			require.Equal(t, []string{"session-a", "session-b", "session-c"},
				FromBodyCandidates("/v1/live", writer.FormDataContentType(), body.Bytes()))
		})
	}
}
