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
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] },
          "unit": "reqps"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 0 },
      "id": 1,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "sum(rate(omnia_agent_requests_total{agent=\"$agent\", namespace=\"$namespace\"}[5m]))",
          "refId": "A"
        }
      ],
      "title": "Requests/sec",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }, { "color": "yellow", "value": 1 }, { "color": "red", "value": 5 }] },
          "unit": "s"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 0 },
      "id": 2,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(omnia_llm_request_duration_seconds_bucket{agent=\"$agent\", namespace=\"$namespace\"}[5m])) by (le))",
          "refId": "A"
        }
      ],
      "title": "P95 Latency",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }, { "color": "yellow", "value": 0.01 }, { "color": "red", "value": 0.05 }] },
          "unit": "percentunit"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 12, "y": 0 },
      "id": 3,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "sum(rate(omnia_agent_requests_total{agent=\"$agent\", namespace=\"$namespace\", status=\"error\"}[5m])) / sum(rate(omnia_agent_requests_total{agent=\"$agent\", namespace=\"$namespace\"}[5m]))",
          "refId": "A"
        }
      ],
      "title": "Error Rate",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 4, "w": 6, "x": 18, "y": 0 },
      "id": 4,
      "options": {
        "colorMode": "value",
        "graphMode": "area",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "sum(omnia_agent_connections_active{agent=\"$agent\", namespace=\"$namespace\"})",
          "refId": "A"
        }
      ],
      "title": "Active Connections",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "legend": false, "tooltip": false, "viz": false },
            "lineInterpolation": "linear",
            "lineWidth": 1,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "auto",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] },
          "unit": "reqps"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 4 },
      "id": 5,
      "options": {
        "legend": { "calcs": ["mean", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "single", "sort": "none" }
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "sum(rate(omnia_agent_requests_total{agent=\"$agent\", namespace=\"$namespace\"}[5m]))",
          "legendFormat": "Requests",
          "refId": "A"
        },
        {
          "expr": "sum(rate(omnia_agent_requests_total{agent=\"$agent\", namespace=\"$namespace\", status=\"error\"}[5m]))",
          "legendFormat": "Errors",
          "refId": "B"
        }
      ],
      "title": "Request Rate",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "legend": false, "tooltip": false, "viz": false },
            "lineInterpolation": "linear",
            "lineWidth": 1,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "auto",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] },
          "unit": "s"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 4 },
      "id": 6,
      "options": {
        "legend": { "calcs": ["mean", "max"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "single", "sort": "none" }
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "histogram_quantile(0.50, sum(rate(omnia_llm_request_duration_seconds_bucket{agent=\"$agent\", namespace=\"$namespace\"}[5m])) by (le))",
          "legendFormat": "p50",
          "refId": "A"
        },
        {
          "expr": "histogram_quantile(0.95, sum(rate(omnia_llm_request_duration_seconds_bucket{agent=\"$agent\", namespace=\"$namespace\"}[5m])) by (le))",
          "legendFormat": "p95",
          "refId": "B"
        },
        {
          "expr": "histogram_quantile(0.99, sum(rate(omnia_llm_request_duration_seconds_bucket{agent=\"$agent\", namespace=\"$namespace\"}[5m])) by (le))",
          "legendFormat": "p99",
          "refId": "C"
        }
      ],
      "title": "Latency Distribution",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "legend": false, "tooltip": false, "viz": false },
            "lineInterpolation": "linear",
            "lineWidth": 1,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "auto",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "normal" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 12 },
      "id": 7,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "single", "sort": "none" }
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "sum(rate(omnia_llm_input_tokens_total{agent=\"$agent\", namespace=\"$namespace\"}[5m])) * 60",
          "legendFormat": "input",
          "refId": "A"
        },
        {
          "expr": "sum(rate(omnia_llm_output_tokens_total{agent=\"$agent\", namespace=\"$namespace\"}[5m])) * 60",
          "legendFormat": "output",
          "refId": "B"
        }
      ],
      "title": "Token Usage",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisCenteredZero": false,
            "axisColorMode": "text",
            "axisLabel": "",
            "axisPlacement": "auto",
            "barAlignment": 0,
            "drawStyle": "line",
            "fillOpacity": 10,
            "gradientMode": "none",
            "hideFrom": { "legend": false, "tooltip": false, "viz": false },
            "lineInterpolation": "linear",
            "lineWidth": 1,
            "pointSize": 5,
            "scaleDistribution": { "type": "linear" },
            "showPoints": "auto",
            "spanNulls": false,
            "stacking": { "group": "A", "mode": "none" },
            "thresholdsStyle": { "mode": "off" }
          },
          "mappings": [],
          "thresholds": { "mode": "absolute", "steps": [{ "color": "green", "value": null }] },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 12 },
      "id": 8,
      "options": {
        "legend": { "calcs": ["sum"], "displayMode": "table", "placement": "bottom", "showLegend": true },
        "tooltip": { "mode": "single", "sort": "none" }
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "sum by (tool, status) (rate(omnia_runtime_tool_calls_total{agent=\"$agent\", namespace=\"$namespace\"}[5m])) * 60 or vector(0)",
          "legendFormat": "{{`{{tool}}`}} ({{`{{status}}`}})",
          "refId": "A"
        }
      ],
      "title": "Tool Calls",
      "type": "timeseries"
    }
    {{- if .Values.loki.enabled }},
    {
      "datasource": { "type": "loki", "uid": "loki" },
      "gridPos": { "h": 10, "w": {{ if .Values.tempo.enabled }}12{{ else }}24{{ end }}, "x": 0, "y": 20 },
      "id": 9,
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
          "expr": "{namespace=\"$namespace\", pod=~\"$agent.*\"} | json | line_format `{{`{{if .level}}`}}[{{`{{.level}}`}}]{{`{{end}}`}} {{`{{if .caller}}`}}({{`{{.caller}}`}}) {{`{{end}}`}}{{`{{.msg | default .message | default __line__}}`}}`",
          "refId": "A"
        }
      ],
      "title": "Recent Logs",
      "type": "logs"
    }
    {{- end }}
    {{- if .Values.tempo.enabled }},
    {
      "datasource": { "type": "tempo", "uid": "tempo" },
      "gridPos": { "h": 10, "w": {{ if .Values.loki.enabled }}12{{ else }}24{{ end }}, "x": {{ if .Values.loki.enabled }}12{{ else }}0{{ end }}, "y": 20 },
      "id": 10,
      "options": {},
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "limit": 20,
          "query": "{resource.service.name=\"$agent.$namespace\"}",
          "queryType": "traceqlSearch",
          "refId": "A",
          "tableType": "traces"
        }
      ],
      "title": "Recent Traces",
      "type": "traces"
    }
    {{- end }}
  ],
  "refresh": "10s",
  "schemaVersion": 38,
  "style": "dark",
  "tags": ["omnia", "agents", "llm"],
  "templating": {
    "list": [
      {
        "current": {},
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "definition": "label_values(omnia_agent_requests_total, namespace)",
        "hide": 0,
        "includeAll": false,
        "label": "Namespace",
        "multi": false,
        "name": "namespace",
        "options": [],
        "query": { "query": "label_values(omnia_agent_requests_total, namespace)", "refId": "StandardVariableQuery" },
        "refresh": 2,
        "regex": "",
        "skipUrlSync": false,
        "sort": 1,
        "type": "query"
      },
      {
        "current": {},
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "definition": "label_values(omnia_agent_requests_total{namespace=\"$namespace\"}, agent)",
        "hide": 0,
        "includeAll": false,
        "label": "Agent",
        "multi": false,
        "name": "agent",
        "options": [],
        "query": { "query": "label_values(omnia_agent_requests_total{namespace=\"$namespace\"}, agent)", "refId": "StandardVariableQuery" },
        "refresh": 2,
        "regex": "",
        "skipUrlSync": false,
        "sort": 1,
        "type": "query"
      }
    ]
  },
  "time": { "from": "now-1h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Omnia Agent Detail",
  "uid": "omnia-agent-detail",
  "version": 1
}
