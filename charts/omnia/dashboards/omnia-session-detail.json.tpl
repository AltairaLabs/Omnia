{
  "annotations": {
    "list": []
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
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
      "description": "Distributed trace waterfall for this session. Session ID (without dashes) = trace ID.",
      "gridPos": { "h": 14, "w": 24, "x": 0, "y": 0 },
      "id": 1,
      "options": {
        "spanBar": {
          "type": "Tag",
          "tag": "service.name"
        }
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "query": "$trace_id",
          "queryType": "traceId",
          "refId": "A"
        }
      ],
      "title": "Session Traces",
      "type": "traces"
    },
    {{- end }}
    {{- if .Values.loki.enabled }}
    {
      "datasource": { "type": "loki", "uid": "loki" },
      "description": "All log lines for this session, correlated via trace ID.",
      "gridPos": { "h": 10, "w": 24, "x": 0, "y": 14 },
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
          "expr": "{agent=\"$agent\"} | trace_id = `$trace_id` | line_format `{{ "{{.msg}}" }}`",
          "refId": "A"
        }
      ],
      "title": "Session Logs",
      "type": "logs"
    },
    {{- end }}
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 24 },
      "id": 10,
      "title": "Agent Metrics (for $agent in $namespace)",
      "type": "row"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "description": "LLM request latency percentiles for this agent during the selected time range.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisBorderShow": false,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "lineWidth": 2,
            "pointSize": 5,
            "showPoints": "auto",
            "spanNulls": true
          },
          "unit": "s"
        },
        "overrides": [
          { "matcher": { "id": "byName", "options": "p50" }, "properties": [{ "id": "color", "value": { "fixedColor": "green", "mode": "fixed" } }] },
          { "matcher": { "id": "byName", "options": "p95" }, "properties": [{ "id": "color", "value": { "fixedColor": "orange", "mode": "fixed" } }] },
          { "matcher": { "id": "byName", "options": "p99" }, "properties": [{ "id": "color", "value": { "fixedColor": "red", "mode": "fixed" } }] }
        ]
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 25 },
      "id": 3,
      "options": {
        "legend": { "calcs": ["lastNotNull"], "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "expr": "histogram_quantile(0.50, sum(rate(omnia_llm_request_duration_seconds_bucket{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval])) by (le))",
          "legendFormat": "p50",
          "refId": "A"
        },
        {
          "expr": "histogram_quantile(0.95, sum(rate(omnia_llm_request_duration_seconds_bucket{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval])) by (le))",
          "legendFormat": "p95",
          "refId": "B"
        },
        {
          "expr": "histogram_quantile(0.99, sum(rate(omnia_llm_request_duration_seconds_bucket{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval])) by (le))",
          "legendFormat": "p99",
          "refId": "C"
        }
      ],
      "title": "LLM Request Latency",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "description": "Input and output token throughput for this agent.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisBorderShow": false,
            "drawStyle": "line",
            "fillOpacity": 20,
            "gradientMode": "scheme",
            "lineWidth": 2,
            "pointSize": 5,
            "showPoints": "auto",
            "spanNulls": true,
            "stacking": { "group": "A", "mode": "normal" }
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 25 },
      "id": 4,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "expr": "sum(increase(omnia_llm_input_tokens_total{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval]))",
          "legendFormat": "Input tokens",
          "refId": "A"
        },
        {
          "expr": "sum(increase(omnia_llm_output_tokens_total{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval]))",
          "legendFormat": "Output tokens",
          "refId": "B"
        }
      ],
      "title": "Token Usage",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "description": "Tool call rate broken down by tool name and status.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisBorderShow": false,
            "drawStyle": "bars",
            "fillOpacity": 80,
            "gradientMode": "none",
            "lineWidth": 1,
            "pointSize": 5,
            "showPoints": "never",
            "spanNulls": true,
            "stacking": { "group": "A", "mode": "normal" }
          },
          "unit": "short"
        }
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 33 },
      "id": 5,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        {
          "expr": "sum by (tool, status) (increase(omnia_runtime_tool_calls_total{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval]))",
          "legendFormat": "{{ "{{tool}}" }} ({{ "{{status}}" }})",
          "refId": "A"
        }
      ],
      "title": "Tool Calls",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "description": "Agent request rate by status (success/error) and estimated LLM cost.",
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisBorderShow": false,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "lineWidth": 2,
            "pointSize": 5,
            "showPoints": "auto",
            "spanNulls": true
          },
          "unit": "reqps"
        },
        "overrides": [
          {
            "matcher": { "id": "byName", "options": "Cost (USD)" },
            "properties": [
              { "id": "custom.axisPlacement", "value": "right" },
              { "id": "unit", "value": "currencyUSD" },
              { "id": "custom.drawStyle", "value": "bars" },
              { "id": "custom.fillOpacity", "value": 30 },
              { "id": "color", "value": { "fixedColor": "yellow", "mode": "fixed" } }
            ]
          },
          {
            "matcher": { "id": "byName", "options": "error" },
            "properties": [{ "id": "color", "value": { "fixedColor": "red", "mode": "fixed" } }]
          }
        ]
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 33 },
      "id": 6,
      "options": {
        "legend": { "calcs": ["sum", "lastNotNull"], "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "multi", "sort": "none" }
      },
      "targets": [
        {
          "expr": "sum by (status) (rate(omnia_agent_requests_total{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval]))",
          "legendFormat": "{{ "{{status}}" }}",
          "refId": "A"
        },
        {
          "expr": "sum(increase(omnia_llm_cost_usd_total{agent=\"$agent\", namespace=\"$namespace\"}[$__rate_interval]))",
          "legendFormat": "Cost (USD)",
          "refId": "B"
        }
      ],
      "title": "Requests & Cost",
      "type": "timeseries"
    }
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
        "hide": 0,
        "label": "Agent",
        "name": "agent",
        "options": [],
        "query": "",
        "skipUrlSync": false,
        "type": "textbox"
      },
      {
        "current": {},
        "hide": 0,
        "label": "Namespace",
        "name": "namespace",
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
  "version": 2
}
