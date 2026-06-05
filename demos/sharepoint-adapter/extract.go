package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// extractText converts raw document bytes into plain text.
// For OOXML formats (.docx, .pptx, .xlsx) it unzips and extracts text runs.
// All other extensions (including .txt, .md, unknown) are returned as-is.
func extractText(filename string, raw []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".docx":
		return extractDocx(raw)
	case ".pptx":
		return extractPptx(raw)
	case ".xlsx":
		return extractXlsx(raw)
	default:
		return string(raw), nil
	}
}

// docxDocumentPart is the OOXML part holding a Word document's body text.
const docxDocumentPart = "word/document.xml"

// extractDocx extracts text from a .docx zip by reading word/document.xml.
// Paragraphs are separated by newlines; text runs within a paragraph are joined.
func extractDocx(raw []byte) (string, error) {
	parts, err := zipTextFromParts(raw, func(name string) bool {
		return name == docxDocumentPart
	}, paragraphExtractor("p", "t"))
	if err != nil {
		return "", fmt.Errorf("docx extraction: %w", err)
	}
	return strings.Join(parts, "\n"), nil
}

// extractPptx extracts text from a .pptx zip by reading each slide XML.
// Slides are separated by newlines.
func extractPptx(raw []byte) (string, error) {
	parts, err := zipTextFromParts(raw, func(name string) bool {
		return strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml")
	}, flatTextExtractor("t"))
	if err != nil {
		return "", fmt.Errorf("pptx extraction: %w", err)
	}
	return strings.Join(parts, "\n"), nil
}

// extractXlsx extracts text from a .xlsx zip by reading xl/sharedStrings.xml.
func extractXlsx(raw []byte) (string, error) {
	parts, err := zipTextFromParts(raw, func(name string) bool {
		return name == "xl/sharedStrings.xml"
	}, flatTextExtractor("t"))
	if err != nil {
		return "", fmt.Errorf("xlsx extraction: %w", err)
	}
	return strings.Join(parts, " "), nil
}

// textExtractFn extracts text tokens from an XML reader.
type textExtractFn func(r io.Reader) ([]string, error)

// zipTextFromParts opens a zip from raw bytes, finds files matching the
// predicate, applies extractFn to each, and returns one string per part.
func zipTextFromParts(raw []byte, match func(string) bool, extractFn textExtractFn) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	var results []string
	for _, f := range zr.File {
		if !match(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		tokens, err := extractFn(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(tokens) > 0 {
			results = append(results, strings.Join(tokens, ""))
		}
	}
	return results, nil
}

// flatTextExtractor returns a textExtractFn that collects all <localName>
// element text content into a flat slice (one entry per element).
func flatTextExtractor(localName string) textExtractFn {
	return func(r io.Reader) ([]string, error) {
		return extractLocalText(r, localName, false)
	}
}

// paragraphExtractor returns a textExtractFn that collects text from
// <textLocal> elements and inserts a newline token at each <paraLocal>
// closing tag, producing paragraph-separated text.
func paragraphExtractor(paraLocal, textLocal string) textExtractFn {
	return func(r io.Reader) ([]string, error) {
		return extractParagraphText(r, paraLocal, textLocal)
	}
}

// extractLocalText streams XML from r, collecting CharData after each
// StartElement whose Local name matches localName.
// When groupByParent is false every matched element's text is a separate token.
func extractLocalText(r io.Reader, localName string, _ bool) ([]string, error) {
	dec := xml.NewDecoder(r)
	var tokens []string
	var capture bool
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			capture = v.Name.Local == localName
		case xml.CharData:
			if capture {
				tokens = append(tokens, string(v))
			}
		case xml.EndElement:
			capture = false
		}
	}
	return tokens, nil
}

// paraState tracks running state while walking paragraph XML.
type paraState struct {
	tokens      []string
	capture     bool
	inPara      bool
	paraHasText bool
}

func (s *paraState) onStart(local, paraLocal, textLocal string) {
	switch local {
	case paraLocal:
		s.inPara = true
		s.paraHasText = false
	case textLocal:
		s.capture = true
	}
}

func (s *paraState) onChar(data string) {
	if s.capture {
		s.tokens = append(s.tokens, data)
		if s.inPara {
			s.paraHasText = true
		}
	}
}

func (s *paraState) onEnd(local, paraLocal, textLocal string) {
	switch local {
	case textLocal:
		s.capture = false
	case paraLocal:
		if s.inPara && s.paraHasText {
			s.tokens = append(s.tokens, "\n")
		}
		s.inPara = false
		s.paraHasText = false
	}
}

// extractParagraphText streams XML from r, collecting text from <textLocal>
// elements and appending a newline sentinel at each </paraLocal> close tag.
// The caller joins the returned slice with "" to get paragraph-separated text.
func extractParagraphText(r io.Reader, paraLocal, textLocal string) ([]string, error) {
	dec := xml.NewDecoder(r)
	var s paraState
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			s.onStart(v.Name.Local, paraLocal, textLocal)
		case xml.CharData:
			s.onChar(string(v))
		case xml.EndElement:
			s.onEnd(v.Name.Local, paraLocal, textLocal)
		}
	}
	// Trim trailing newline token if present
	if len(s.tokens) > 0 && s.tokens[len(s.tokens)-1] == "\n" {
		s.tokens = s.tokens[:len(s.tokens)-1]
	}
	return s.tokens, nil
}
