//go:build ignore

// gen.go is a standalone generator for SharePoint hero demo sample documents.
// Run with: go run gen.go
//
// Produces minimal-but-valid OOXML (.docx/.pptx/.xlsx) files whose text parts
// are extractable by the sharepoint-adapter's extractText function:
//
//	.docx → word/document.xml  (<w:p><w:r><w:t>)
//	.pptx → ppt/slides/slideN.xml (<a:t>)
//	.xlsx → xl/sharedStrings.xml (<sst><si><t>)
package main

import (
	"archive/zip"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	outDir := "out"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}

	// 1. Incident failover runbook — ALLOWED site
	if err := writeDocx(filepath.Join(outDir, "incident-failover-runbook.docx"), []string{
		"Incident Failover Runbook — Sev1 Database Outage",
		"",
		"Escalation Path",
		"1. On-call SRE is paged (PagerDuty). Acknowledge within 5 minutes.",
		"2. If not resolved in 15 minutes, escalate to Incident Commander.",
		"3. Incident Commander loops in DB Lead and notifies Engineering Manager.",
		"4. Stakeholder update every 30 minutes until resolution.",
		"",
		"Failover Steps",
		"Step 1: Confirm primary is unreachable — ping replica endpoint and verify read traffic is live.",
		"Step 2: Promote replica to primary — run: pg_ctl promote -D /var/lib/postgresql/data",
		"Step 3: Update DNS — change orders-db.internal CNAME to replica hostname; TTL flush via: rndc flush",
		"Step 4: Update connection strings — rotate SECRET/orders-db-url in AWS Secrets Manager to point at new primary.",
		"Step 5: Restart affected services — rolling restart orders-api and payment-service pods.",
		"Step 6: Verify — check orders-api health endpoint returns 200 and monitor error rate < 0.1% for 5 minutes.",
		"Step 7: Open postmortem ticket within 24 hours.",
		"",
		"Contacts",
		"On-call SRE rotation: #oncall-sre Slack channel",
		"Incident Commander: @incident-commander",
		"DB Lead: @db-lead",
	}); err != nil {
		log.Fatalf("incident-failover-runbook.docx: %v", err)
	}
	fmt.Println("wrote out/incident-failover-runbook.docx")

	// 2. Data handling policy — ALLOWED site
	if err := writeDocx(filepath.Join(outDir, "data-handling-policy.docx"), []string{
		"Customer Data Handling Policy",
		"",
		"Purpose",
		"This policy defines the rules for handling customer personally identifiable information (PII) " +
			"across all engineering and operations teams.",
		"",
		"Third-Party Sharing",
		"Customer PII must not be shared with third-party vendors without a signed Data Processing Agreement (DPA).",
		"Before exporting any dataset containing customer records, all PII fields must be redacted or pseudonymised.",
		"Approved vendors are listed in the Vendor Register (internal wiki). Using an unlisted vendor requires " +
			"written approval from the Data Protection Officer.",
		"",
		"Storage and Retention",
		"Customer records must be stored in approved, encrypted storage (AES-256 at rest).",
		"Records must be deleted within 30 days of a verified deletion request.",
		"Logs containing customer identifiers must be purged after 90 days.",
		"",
		"Incident Reporting",
		"Any suspected breach involving customer data must be reported to security@company.example within 24 hours.",
		"",
		"Compliance",
		"Violations of this policy may result in disciplinary action and regulatory penalties under GDPR and CCPA.",
	}); err != nil {
		log.Fatalf("data-handling-policy.docx: %v", err)
	}
	fmt.Println("wrote out/data-handling-policy.docx")

	// 3. On-call ops metrics — ALLOWED site
	if err := writeXlsx(filepath.Join(outDir, "oncall-metrics.xlsx"), []string{
		"Service", "P50 latency (ms)", "P99 latency (ms)", "Error rate (%)", "On-call owner",
		"orders-db", "12", "88", "0.04", "SRE-team",
		"payment-service", "23", "145", "0.12", "payments-oncall",
		"inventory-api", "8", "52", "0.01", "platform-oncall",
		"notification-svc", "5", "31", "0.00", "platform-oncall",
	}); err != nil {
		log.Fatalf("oncall-metrics.xlsx: %v", err)
	}
	fmt.Println("wrote out/oncall-metrics.xlsx")

	// 4. Incident post-mortem deck — ALLOWED site
	if err := writePptx(filepath.Join(outDir, "incident-postmortem.pptx"), [][]string{
		{
			"Incident Post-Mortem",
			"orders-db Sev1 outage — 2026-05-28",
			"Duration: 47 minutes",
		},
		{
			"Timeline",
			"02:14 UTC — PagerDuty alert fires (orders-api error rate > 5%)",
			"02:19 UTC — On-call SRE acknowledges, begins investigation",
			"02:31 UTC — DB Lead confirms primary disk failure; replica promotion starts",
			"02:41 UTC — DNS and connection strings updated; traffic flowing to new primary",
			"02:51 UTC — Error rate back below 0.1%; incident declared resolved",
			"03:01 UTC — Stakeholder communication sent",
		},
		{
			"Root Cause",
			"Primary database host suffered a disk I/O failure due to a degraded RAID volume.",
			"Automated failover was not triggered because the health-check threshold was set too conservatively.",
			"",
			"Action Items",
			"1. Lower health-check failure threshold from 10 to 3 consecutive failures — owner: DB Lead, due: 2026-06-05",
			"2. Enable automated replica promotion in RDS Multi-AZ — owner: SRE-team, due: 2026-06-12",
			"3. Add disk I/O saturation alert to PagerDuty — owner: platform-oncall, due: 2026-06-10",
		},
	}); err != nil {
		log.Fatalf("incident-postmortem.pptx: %v", err)
	}
	fmt.Println("wrote out/incident-postmortem.pptx")

	// 5. Restricted acquisition memo — RESTRICTED site
	// This document goes on the SharePoint site whose URL contains "restricted".
	// The demo's ToolPolicy deny-filter blocks retrieval from that site,
	// demonstrating the governance beat.
	if err := writeDocx(filepath.Join(outDir, "restricted-acquisition-memo.docx"), []string{
		"CONFIDENTIAL — INTERNAL USE ONLY",
		"",
		"Strategic Acquisition Memo",
		"",
		"This document contains non-public information regarding a potential acquisition target.",
		"Distribution is restricted to members of the M&A steering committee.",
		"",
		"Do not forward, print, or discuss the contents of this memo outside the steering committee " +
			"without written approval from General Counsel.",
		"",
		"All copies must be destroyed or returned to Legal upon request.",
		"",
		"Classification: Restricted — Confidential",
	}); err != nil {
		log.Fatalf("restricted-acquisition-memo.docx: %v", err)
	}
	fmt.Println("wrote out/restricted-acquisition-memo.docx")

	// 6. Customer escalation note — ALLOWED site
	// NOTE: All customer details below are entirely synthetic and obviously fictitious.
	// They are placeholder values invented for the demo's PII-redaction beat.
	// Do NOT use real customer names, emails, or phone numbers in demo assets.
	if err := writeDocx(filepath.Join(outDir, "customer-escalation-note.docx"), []string{
		"Customer Escalation Note",
		"",
		"Customer: Jordan Rivers",
		"Email: jordan.rivers@example.com",
		"Phone: +1-555-0142",
		"Account ID: ACCT-00042",
		"",
		"Issue Summary",
		"Jordan Rivers contacted support on 2026-05-29 reporting that their account dashboard " +
			"showed incorrect billing totals for the prior month.",
		"",
		"Actions Taken",
		"1. Support agent verified account ownership via security questions.",
		"2. Billing team corrected the invoice and issued a credit of $24.00.",
		"3. Escalated to engineering for root-cause investigation of the billing calculation bug.",
		"",
		"Status: Resolved — awaiting confirmation from customer.",
	}); err != nil {
		log.Fatalf("customer-escalation-note.docx: %v", err)
	}
	fmt.Println("wrote out/customer-escalation-note.docx")

	fmt.Println("done — 6 files written to out/")
}

// ---------------------------------------------------------------------------
// OOXML helpers
// ---------------------------------------------------------------------------

// writeZip writes a zip archive at path with the given parts (name → content).
func writeZip(path string, parts map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	// Write in a deterministic order for reproducibility.
	order := []string{
		"[Content_Types].xml",
		"_rels/.rels",
	}
	// Append remaining keys not already in order.
	inOrder := map[string]bool{}
	for _, k := range order {
		inOrder[k] = true
	}
	for k := range parts {
		if !inOrder[k] {
			order = append(order, k)
		}
	}
	for _, name := range order {
		content, ok := parts[name]
		if !ok {
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("create zip entry %s: %w", name, err)
		}
		if _, err := fmt.Fprint(w, content); err != nil {
			return fmt.Errorf("write zip entry %s: %w", name, err)
		}
	}
	return zw.Close()
}

// writeDocx writes a minimal valid .docx file whose paragraphs contain paras.
// The extractor reads word/document.xml, matching <w:p><w:r><w:t> elements.
func writeDocx(path string, paras []string) error {
	var body string
	for _, p := range paras {
		if p == "" {
			// Empty paragraph — no <w:t> so extractor skips it, but valid OOXML.
			body += `<w:p/>`
			continue
		}
		body += `<w:p><w:r><w:t xml:space="preserve">` + xmlEscape(p) + `</w:t></w:r></w:p>`
	}
	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"` +
		` xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<w:body>` + body + `</w:body></w:document>`

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/word/document.xml"` +
		` ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`</Types>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"` +
		` Target="word/document.xml"/>` +
		`</Relationships>`

	return writeZip(path, map[string]string{
		"[Content_Types].xml": contentTypes,
		"_rels/.rels":         rels,
		"word/document.xml":   documentXML,
	})
}

// writePptx writes a minimal valid .pptx file.
// slides is a slice of slide-content slices; each inner slice is one slide's lines.
// The extractor reads ppt/slides/slideN.xml matching <a:t> elements.
func writePptx(path string, slides [][]string) error {
	parts := map[string]string{}

	var overrides string
	for i, lines := range slides {
		n := i + 1
		var body string
		for _, line := range lines {
			if line == "" {
				continue
			}
			body += `<a:p><a:r><a:t>` + xmlEscape(line) + `</a:t></a:r></a:p>`
		}
		slideXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
			` xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
			`<p:cSld><p:spTree>` +
			`<p:sp><p:txBody>` + body + `</p:txBody></p:sp>` +
			`</p:spTree></p:cSld></p:sld>`
		partName := fmt.Sprintf("ppt/slides/slide%d.xml", n)
		parts[partName] = slideXML
		overrides += fmt.Sprintf(
			`<Override PartName="/%s" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`,
			partName,
		)
	}

	parts["[Content_Types].xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		overrides +
		`</Types>`

	parts["_rels/.rels"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"` +
		` Target="ppt/presentation.xml"/>` +
		`</Relationships>`

	return writeZip(path, parts)
}

// writeXlsx writes a minimal valid .xlsx file using a shared-string table.
// cells is a flat list of string values; the extractor reads xl/sharedStrings.xml <t> elements.
func writeXlsx(path string, cells []string) error {
	var sis string
	for _, c := range cells {
		sis += `<si><t>` + xmlEscape(c) + `</t></si>`
	}
	sharedStrings := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		fmt.Sprintf(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`,
			len(cells), len(cells)) +
		sis + `</sst>`

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/sharedStrings.xml"` +
		` ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>` +
		`</Types>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"` +
		` Target="xl/workbook.xml"/>` +
		`</Relationships>`

	return writeZip(path, map[string]string{
		"[Content_Types].xml":  contentTypes,
		"_rels/.rels":          rels,
		"xl/sharedStrings.xml": sharedStrings,
	})
}

// xmlEscape escapes the five XML special characters in s.
func xmlEscape(s string) string {
	out := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			out = append(out, '&', 'a', 'm', 'p', ';')
		case '<':
			out = append(out, '&', 'l', 't', ';')
		case '>':
			out = append(out, '&', 'g', 't', ';')
		case '"':
			out = append(out, '&', 'q', 'u', 'o', 't', ';')
		case '\'':
			out = append(out, '&', 'a', 'p', 'o', 's', ';')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
