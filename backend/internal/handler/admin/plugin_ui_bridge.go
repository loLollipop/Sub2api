package admin

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"fmt"

	"golang.org/x/net/html"
)

//go:embed plugin_ui_bridge.js
var pluginUIBridgeScript []byte

// injectPluginUIBridge adds the host-owned v1 transport adapter before package
// scripts, after any doctype so standards-mode layout is preserved. Stored
// package bytes and their manifest hashes remain unchanged.
func injectPluginUIBridge(data []byte) []byte {
	insertion, consumed := 0, 0
	tokens := html.NewTokenizer(bytes.NewReader(data))
scan:
	for {
		kind := tokens.Next()
		consumed += len(tokens.Raw())
		switch kind {
		case html.DoctypeToken:
			insertion = consumed
			break scan
		case html.StartTagToken, html.SelfClosingTagToken, html.ErrorToken:
			break scan
		}
	}
	var output bytes.Buffer
	output.Grow(len(data) + len(pluginUIBridgeScript) + 32)
	// bytes.Buffer writes only to memory and always return a nil error.
	_, _ = output.Write(data[:insertion])
	_, _ = output.WriteString("<script>\n")
	_, _ = output.Write(pluginUIBridgeScript)
	_, _ = output.WriteString("\n</script>")
	_, _ = output.Write(data[insertion:])
	return output.Bytes()
}

// XHTML must retain its XML declaration, namespaces and CDATA script syntax.
// Inserting HTML before an XML document would make the entire UI unparsable.
func injectPluginXHTMLBridge(data []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		start := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("plugin XHTML has no head: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if depth != 2 || element.Name.Local != "head" || element.Name.Space != "http://www.w3.org/1999/xhtml" {
				continue
			}
			end := int(decoder.InputOffset())
			var output bytes.Buffer
			if bytes.HasSuffix(data[start:end], []byte("/>")) {
				// Expand an empty head while preserving its qualified name/attributes.
				nameEnd := start + 1
				for nameEnd < end && !bytes.ContainsRune([]byte(" \t\r\n/>"), rune(data[nameEnd])) {
					nameEnd++
				}
				_, _ = output.Write(data[:end-2])
				_ = output.WriteByte('>')
				_, _ = output.Write(pluginXHTMLBridgeElement())
				_, _ = output.WriteString("</")
				_, _ = output.Write(data[start+1 : nameEnd])
				_ = output.WriteByte('>')
			} else {
				_, _ = output.Write(data[:end])
				_, _ = output.Write(pluginXHTMLBridgeElement())
			}
			_, _ = output.Write(data[end:])
			return output.Bytes(), nil
		case xml.EndElement:
			depth--
		}
	}
}

func pluginXHTMLBridgeElement() []byte {
	script := bytes.ReplaceAll(pluginUIBridgeScript, []byte("]]>"), []byte("]]]]><![CDATA[>"))
	return bytes.Join([][]byte{
		[]byte(`<script xmlns="http://www.w3.org/1999/xhtml"><![CDATA[`),
		script,
		[]byte("]]></script>"),
	}, nil)
}
