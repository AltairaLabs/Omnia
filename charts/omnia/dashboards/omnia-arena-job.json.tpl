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
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
      "id": 100,
      "title": "Traces",
      "type": "row"
    }
    {{- if .Values.tempo.enabled }},
    {
      "datasource": { "type": "tempo", "uid": "tempo" },
      "gridPos": { "h": 10, "w": 24, "x": 0, "y": 1 },
      "id": 1,
      "options": {},
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "limit": 100,
          "query": "{span.arena.job=\"$job_name\"}",
          "queryType": "traceql",
          "refId": "A",
          "tableType": "traces"
        }
      ],
      "title": "Job Traces",
      "type": "traces"
    }
    {{- end }},
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 11 },
      "id": 101,
      "title": "Logs",
      "type": "row"
    }
    {{- if .Values.loki.enabled }},
    {
      "datasource": { "type": "loki", "uid": "loki" },
      "gridPos": { "h": 12, "w": 24, "x": 0, "y": 12 },
      "id": 2,
      "options": {
        "dedupStrategy": "none",
        "enableLogDetails": true,
        "prettifyLogMessage": true,
        "showCommonLabels": false,
        "showLabels": false,
        "showTime": true,
        "sortOrder": "Descending",
        "wrapLogMessage": true
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "{pod=~\"$job_name.*\"} | json | line_format `[{{ "{{.pod}}" }}] {{ "{{.msg}}" }}`",
          "refId": "A"
        }
      ],
      "title": "Worker Logs",
      "type": "logs"
    }
    {{- end }},
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 24 },
      "id": 102,
      "title": "Metrics",
      "type": "row"
    }
    {{- if .Values.prometheus.enabled }},
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 80 }
            ]
          },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 8, "x": 0, "y": 25 },
      "id": 3,
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "arena_work_items_total{job_name=\"$job_name\", status=\"completed\"}",
          "legendFormat": "Completed",
          "refId": "A"
        }
      ],
      "title": "Completed Work Items",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 1 }
            ]
          },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 8, "x": 8, "y": 25 },
      "id": 4,
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "arena_work_items_total{job_name=\"$job_name\", status=\"failed\"}",
          "legendFormat": "Failed",
          "refId": "A"
        }
      ],
      "title": "Failed Work Items",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null }
            ]
          },
          "unit": "s"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 8, "x": 16, "y": 25 },
      "id": 5,
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, rate(arena_work_item_duration_seconds_bucket{job_name=\"$job_name\"}[$__rate_interval]))",
          "legendFormat": "p95",
          "refId": "A"
        }
      ],
      "title": "Work Item Duration (p95)",
      "type": "stat"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisBorderShow": false,
            "axisLabel": "",
            "fillOpacity": 80,
            "lineWidth": 1,
            "scaleDistribution": { "type": "linear" },
            "stacking": { "group": "A", "mode": "normal" }
          },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 33 },
      "id": 6,
      "options": {
        "barRadius": 0,
        "barWidth": 0.9,
        "groupWidth": 0.7,
        "legend": { "calcs": [], "displayMode": "list", "placement": "bottom" },
        "orientation": "auto",
        "tooltip": { "mode": "single", "sort": "none" },
        "xTickLabelRotation": 0
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "arena_eval_results_total{job_name=\"$job_name\"}",
          "legendFormat": "{{ "{{eval_id}}" }} ({{ "{{status}}" }})",
          "refId": "A"
        }
      ],
      "title": "Eval Results by Type",
      "type": "barchart"
    },
    {
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "fieldConfig": {
        "defaults": {
          "color": { "mode": "palette-classic" },
          "custom": {
            "axisBorderShow": false,
            "axisLabel": "",
            "drawStyle": "line",
            "fillOpacity": 10,
            "lineInterpolation": "smooth",
            "lineWidth": 2,
            "pointSize": 5,
            "showPoints": "auto",
            "spanNulls": false
          },
          "unit": "short"
        },
        "overrides": []
      },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 33 },
      "id": 7,
      "options": {
        "legend": { "calcs": [], "displayMode": "list", "placement": "bottom" },
        "tooltip": { "mode": "single", "sort": "none" }
      },
      "pluginVersion": "10.0.0",
      "targets": [
        {
          "expr": "rate(arena_work_items_total{job_name=\"$job_name\"}[$__rate_interval])",
          "legendFormat": "{{ "{{status}}" }}",
          "refId": "A"
        }
      ],
      "title": "Work Item Throughput",
      "type": "timeseries"
    }
    {{- end }}
  ],
  "refresh": "10s",
  "schemaVersion": 38,
  "style": "dark",
  "tags": ["omnia", "arena"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 0,
        "label": "Job Name",
        "name": "job_name",
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
  "time": { "from": "now-1h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Omnia Arena Job",
  "uid": "omnia-arena-job",
  "version": 1
}
