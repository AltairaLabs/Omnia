{
  "annotations": {
    "list": []
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 0,
  "id": null,
  "links": [
    {
      "asDropdown": false,
      "icon": "external link",
      "includeVars": true,
      "keepTime": true,
      "tags": ["omnia"],
      "targetBlank": true,
      "title": "Omnia Dashboards",
      "type": "dashboards"
    }
  ],
  "panels": [
    {{- if .Values.tempo.enabled }}
    {
      "datasource": { "type": "tempo", "uid": "tempo" },
      "gridPos": { "h": 12, "w": 24, "x": 0, "y": 0 },
      "id": 1,
      "options": {},
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "limit": 50,
          "query": "$trace_id",
          "queryType": "traceql",
          "refId": "A",
          "tableType": "traces"
        }
      ],
      "title": "Traces",
      "type": "traces"
    }
    {{- end }}
    {{- if and .Values.tempo.enabled .Values.loki.enabled }},{{ end }}
    {{- if .Values.loki.enabled }}
    {
      "datasource": { "type": "loki", "uid": "loki" },
      "gridPos": { "h": 12, "w": 24, "x": 0, "y": 12 },
      "id": 2,
      "options": {
        "dedupStrategy": "none",
        "enableLogDetails": true,
        "prettifyLogMessage": true,
        "showCommonLabels": false,
        "showLabels": true,
        "showTime": true,
        "sortOrder": "Descending",
        "wrapLogMessage": true
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "{agent=~\".+\"} | session_id = `$session_id`",
          "refId": "A"
        }
      ],
      "title": "Logs",
      "type": "logs"
    }
    {{- end }}
  ],
  "refresh": "",
  "schemaVersion": 38,
  "style": "dark",
  "tags": ["omnia", "sessions"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 0,
        "label": "Session ID",
        "name": "session_id",
        "options": [],
        "query": "",
        "skipUrlSync": false,
        "type": "textbox"
      },
      {
        "current": {},
        "hide": 2,
        "label": "Trace ID",
        "name": "trace_id",
        "options": [],
        "query": "",
        "skipUrlSync": false,
        "type": "textbox"
      }
    ]
  },
  "time": { "from": "now-6h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Omnia Session Detail",
  "uid": "omnia-session-detail",
  "version": 1
}
