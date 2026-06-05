package main

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeZip builds a minimal in-memory zip containing the given name→content parts.
func makeZip(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractText_Docx(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:p><w:r><w:t>Hello</w:t></w:r><w:r><w:t> World</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Second para</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	raw := makeZip(t, map[string]string{"word/document.xml": docXML})

	result, err := extractText("runbook.docx", raw)

	require.NoError(t, err)
	assert.Contains(t, result, "Hello World")
	assert.Contains(t, result, "Second para")
}

func TestExtractText_Pptx(t *testing.T) {
	const slideNS = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	const slideBody = `<p:cSld><p:spTree><p:sp><p:txBody>` +
		`<a:p><a:r>`
	const slideBodyEnd = `</a:r></a:p>` +
		`</p:txBody></p:sp></p:spTree></p:cSld>`
	slide1XML := `<p:sld ` + slideNS + `>` + slideBody +
		`<a:t>Slide one text</a:t>` + slideBodyEnd + `</p:sld>`
	slide2XML := `<p:sld ` + slideNS + `>` + slideBody +
		`<a:t>Slide two text</a:t>` + slideBodyEnd + `</p:sld>`
	raw := makeZip(t, map[string]string{
		"ppt/slides/slide1.xml": slide1XML,
		"ppt/slides/slide2.xml": slide2XML,
	})

	result, err := extractText("presentation.pptx", raw)

	require.NoError(t, err)
	assert.Contains(t, result, "Slide one text")
	assert.Contains(t, result, "Slide two text")
}

func TestExtractText_Xlsx(t *testing.T) {
	ssXML := `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<si><t>Cell A</t></si><si><t>Cell B</t></si>` +
		`</sst>`
	raw := makeZip(t, map[string]string{"xl/sharedStrings.xml": ssXML})

	result, err := extractText("data.xlsx", raw)

	require.NoError(t, err)
	assert.Contains(t, result, "Cell A")
	assert.Contains(t, result, "Cell B")
}

func TestExtractText_TxtPassthrough(t *testing.T) {
	result, err := extractText("notes.txt", []byte("plain text"))

	require.NoError(t, err)
	assert.Equal(t, "plain text", result)
}

func TestExtractText_MdPassthrough(t *testing.T) {
	result, err := extractText("readme.md", []byte("# Title"))

	require.NoError(t, err)
	assert.Equal(t, "# Title", result)
}

func TestExtractText_UnknownExtensionPassthrough(t *testing.T) {
	result, err := extractText("file.bin", []byte("raw"))

	require.NoError(t, err)
	assert.Equal(t, "raw", result)
}

func TestExtractText_CorruptDocx(t *testing.T) {
	_, err := extractText("bad.docx", []byte("not a zip"))

	require.Error(t, err)
}

func TestExtractText_CaseInsensitiveExtension(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>Upper case ext</w:t></w:r></w:p></w:body></w:document>`
	raw := makeZip(t, map[string]string{"word/document.xml": docXML})

	result, err := extractText("RUNBOOK.DOCX", raw)

	require.NoError(t, err)
	assert.Contains(t, result, "Upper case ext")
}
