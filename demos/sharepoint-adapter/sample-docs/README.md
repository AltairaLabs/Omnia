# SharePoint Hero Demo — Sample Documents

These are demo Office documents for the [SharePoint RAG hero demo](../../../docs/local-backlog/).
An operator uploads them to SharePoint Online so the demo's RAG agent has real OOXML files to retrieve and extract text from.

## Generating the files

```bash
cd demos/sharepoint-adapter/sample-docs
go run gen.go
```

Output is written to `out/` (gitignored). The `//go:build ignore` tag on `gen.go` means it is excluded from normal `go build`/`go test` runs; `go run gen.go` is the only way to execute it.

## Files and target sites

| File | SharePoint site | Demo beat |
|------|----------------|-----------|
| `incident-failover-runbook.docx` | **Allowed** | Incident-runbook scenario — escalation path + failover steps for an orders-db Sev1 |
| `data-handling-policy.docx` | **Allowed** | Policy-lookup scenario — PII/vendor sharing rules, DPA requirement, retention |
| `oncall-metrics.xlsx` | **Allowed** | Format variety — shared-string table of service latency + error-rate metrics |
| `incident-postmortem.pptx` | **Allowed** | Format variety — 3-slide post-mortem deck (Timeline / Root cause / Action items) |
| `customer-escalation-note.docx` | **Allowed** | PII-redaction beat — contains synthetic customer details (see note below) |
| `restricted-acquisition-memo.docx` | **Restricted** | Governance beat — blocked by ToolPolicy deny-filter on the restricted site |

### Allowed vs Restricted sites

The demo uses two SharePoint site collections:

- **Allowed site** — the standard ops/engineering SharePoint site. The ToolPolicy permits the adapter to fetch documents from it.
- **Restricted site** — a separate site whose URL **must contain the string `restricted`** (e.g. `https://contoso.sharepoint.com/sites/restricted-internal`). The ToolPolicy deny-filter blocks fetches from URLs matching `*restricted*`, so `restricted-acquisition-memo.docx` is retrievable at the listing stage but blocked when the agent tries to fetch its content — demonstrating the governance beat.

## Extractability

`gen.go` produces minimal-but-valid OOXML zips. Text is placed in exactly the parts that `extract.go` reads:

| Format | Part | Elements |
|--------|------|----------|
| `.docx` | `word/document.xml` | `<w:p><w:r><w:t>` |
| `.pptx` | `ppt/slides/slideN.xml` | `<a:t>` |
| `.xlsx` | `xl/sharedStrings.xml` | `<sst><si><t>` |

## Synthetic PII note

`customer-escalation-note.docx` contains the following **entirely synthetic, obviously fictitious** values used only for the PII-redaction demo beat:

- Name: **Jordan Rivers**
- Email: **jordan.rivers@example.com**
- Phone: **+1-555-0142**

`example.com` is an IANA-reserved test domain. The `555` prefix is the conventional US fiction placeholder. These values do not correspond to any real person.
