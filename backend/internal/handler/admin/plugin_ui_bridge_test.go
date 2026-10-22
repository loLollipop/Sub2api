package admin

import (
	"bytes"
	"encoding/xml"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginUIBridgePreservesDocumentAndPrecedesPackageScripts(t *testing.T) {
	for _, prefix := range []string{"", "<!doctype html>", "\xef\xbb\xbf<!-- intro -->\n<!DOCTYPE HTML>"} {
		t.Run(prefix, func(t *testing.T) {
			body := "<html><head><meta charset=\"utf-8\"><script>window.packageStarted = true</script></head><body>配置</body></html>"
			original := []byte(prefix + body)
			before := bytes.Clone(original)
			output := injectPluginUIBridge(original)
			require.Equal(t, before, original, "stored package contents must remain intact")
			require.True(t, bytes.HasPrefix(output, []byte(prefix+"<script>\n")))
			require.True(t, bytes.HasSuffix(output, []byte(body)))
			require.Equal(t, before, bytes.Replace(output, append(append([]byte("<script>\n"), pluginUIBridgeScript...), []byte("\n</script>")...), nil, 1))
		})
	}
}

func TestPluginUIBridgePreservesXHTML(t *testing.T) {
	for _, head := range []string{`<head><title>配置</title></head>`, `<head/>`, `<x:head data-test="value" />`} {
		t.Run(head, func(t *testing.T) {
			prefix := `<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:x="http://www.w3.org/1999/xhtml">`
			data := []byte(prefix + head + `<body><script><![CDATA[window.started=true;]]></script></body></html>`)
			output, err := injectPluginXHTMLBridge(data)
			require.NoError(t, err)
			require.True(t, bytes.HasPrefix(output, []byte(prefix)))
			decoder := xml.NewDecoder(bytes.NewReader(output))
			var scripts []string
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				require.NoError(t, err, "injected document must remain valid XML")
				if start, ok := token.(xml.StartElement); ok && start.Name.Local == "script" {
					require.Equal(t, "http://www.w3.org/1999/xhtml", start.Name.Space)
					var script string
					require.NoError(t, decoder.DecodeElement(&script, &start))
					scripts = append(scripts, script)
				}
			}
			require.Len(t, scripts, 2)
			require.Equal(t, string(pluginUIBridgeScript), scripts[0])
			require.Equal(t, "window.started=true;", scripts[1])
		})
	}
}
