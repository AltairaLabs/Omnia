---
title: "Query the Session Archive"
description: "Run SQL over archived session data in Snowflake, BigQuery, ClickHouse, Athena, or DuckDB"
sidebar:
  order: 20
---

Omnia automatically archives completed sessions from the warm Postgres store to object storage (S3, GCS, or Azure Blob) as Parquet files. This guide shows how to query that archive from any major SQL engine.

There is no dedicated sync controller or ETL pipeline to run. The archive is written in a standard Hive-partitioned Parquet layout that every major warehouse can read as an external table, so you point your engine of choice at the bucket and run SQL directly.

## How the archive is produced

The long-term storage path has three pieces, all shipped in the core Helm chart:

1. **`SessionRetentionPolicy`** — a cluster-scoped CRD that declares how long sessions stay in the warm store before being moved to cold.
2. **Compaction CronJob** — the `omnia-compaction` Kubernetes CronJob runs on a schedule (default daily), reads sessions that have exceeded their warm retention, and writes them to the configured object store as Parquet.
3. **Cold archive provider** — the Go package that performs the actual write. Configured via `cold.backend`, `cold.bucket`, `cold.region`, and `cold.endpoint` on the session-api deployment.

Enable it in your Helm values:

```yaml
sessionRetention:
  compaction:
    enabled: true
    schedule: "0 3 * * *"   # daily at 03:00 UTC
  defaultPolicy:
    coldArchive:
      enabled: true
      backend: s3           # s3 | gcs | azure
      bucket: my-omnia-archive
      region: us-east-1
```

## File layout in the bucket

Files are written under a configurable prefix (default `sessions/`) using Hive-style partitioning so partition pruning works in every query engine:

```
s3://my-omnia-archive/sessions/
├── _manifest.json                                              # internal index, skip this
├── year=2026/month=04/day=10/agent=my-agent/
│   ├── part-0000.parquet
│   └── part-0001.parquet
├── year=2026/month=04/day=10/agent=another-agent/
│   └── part-0000.parquet
└── year=2026/month=04/day=11/agent=my-agent/
    └── part-0000.parquet
```

- **Compression**: Snappy (configurable).
- **Max file size**: 128 MB default, so partitions may contain multiple `part-*.parquet` files.
- **Agent name sanitization**: characters outside `[a-zA-Z0-9_-]` are replaced with `_` in the partition path.
- **`_manifest.json`**: an internal JSON index used by Omnia for O(1) session lookups. It is *not* a Parquet file — exclude it from your warehouse's file pattern (e.g. `*.parquet` rather than `*`).

## Parquet schema

Every file has the same schema:

| Column | Type | Notes |
|---|---|---|
| `id` | `string` | Session ID |
| `agent_name` | `string` | Agent that owned the session |
| `namespace` | `string` | Kubernetes namespace |
| `workspace_name` | `string` (optional) | Workspace |
| `status` | `string` | Terminal session status |
| `created_at` | `int64` | Nanoseconds since Unix epoch |
| `updated_at` | `int64` | Nanoseconds since Unix epoch |
| `expires_at` | `int64` | Nanoseconds since Unix epoch (0 when unset) |
| `ended_at` | `int64` | Nanoseconds since Unix epoch (0 when unset) |
| `message_count` | `int32` | |
| `tool_call_count` | `int32` | |
| `total_input_tokens` | `int64` | |
| `total_output_tokens` | `int64` | |
| `estimated_cost_usd` | `float64` | |
| `tags` | `string` | JSON-encoded `map[string]string` |
| `state` | `string` | JSON-encoded session state |
| `last_message_preview` | `string` (optional) | |
| `messages_json` | `string` | JSON-encoded full message history |

The `messages_json` column is the interesting one for analytics: it holds the complete conversation (role, content, tool calls, tool results, token counts) as a JSON string. Every supported warehouse can parse it in place.

### Converting nanosecond timestamps

Timestamp columns are stored as `int64` nanoseconds so they round-trip through Parquet losslessly. Convert them to native timestamps in your query:

- **Snowflake**: `TO_TIMESTAMP_NTZ(created_at / 1000000000)`
- **BigQuery**: `TIMESTAMP_MICROS(DIV(created_at, 1000))`
- **ClickHouse**: `fromUnixTimestamp64Nano(created_at)`
- **Athena / Trino**: `from_unixtime(created_at / 1000000000)`
- **DuckDB**: `make_timestamp(created_at // 1000)`

## Querying from Snowflake

Create an external stage pointing at your bucket, then an external table over the Parquet files:

```sql
CREATE OR REPLACE STAGE omnia_archive_stage
  URL = 's3://my-omnia-archive/sessions/'
  STORAGE_INTEGRATION = my_s3_integration
  FILE_FORMAT = (TYPE = PARQUET);

CREATE OR REPLACE EXTERNAL TABLE omnia_sessions
  WITH LOCATION = @omnia_archive_stage
  PATTERN = '.*[.]parquet'
  FILE_FORMAT = (TYPE = PARQUET)
  AUTO_REFRESH = TRUE;
```

Query the conversation content using `VARIANT`:

```sql
SELECT
  VALUE:id::STRING                                              AS session_id,
  VALUE:agent_name::STRING                                      AS agent,
  TO_TIMESTAMP_NTZ(VALUE:created_at::NUMBER / 1000000000)       AS created_at,
  VALUE:total_input_tokens::NUMBER                              AS input_tokens,
  VALUE:estimated_cost_usd::FLOAT                               AS cost_usd,
  PARSE_JSON(VALUE:messages_json::STRING)                       AS messages
FROM omnia_sessions
WHERE VALUE:agent_name::STRING = 'my-agent'
  AND VALUE:created_at::NUMBER >= DATE_PART('epoch_nanosecond', '2026-04-01'::TIMESTAMP);
```

For high-frequency BI dashboards where external table read cost becomes noticeable, use [Snowpipe](https://docs.snowflake.com/en/user-guide/data-load-snowpipe-intro) to load the files into a native table on an S3 `ObjectCreated` event — Omnia does not need to be involved.

## Querying from BigQuery

Create a Hive-partitioned external table:

```sql
CREATE OR REPLACE EXTERNAL TABLE `my_project.omnia.sessions`
  WITH PARTITION COLUMNS (
    year INT64,
    month INT64,
    day INT64,
    agent STRING
  )
  OPTIONS (
    format = 'PARQUET',
    hive_partition_uri_prefix = 'gs://my-omnia-archive/sessions/',
    uris = ['gs://my-omnia-archive/sessions/year=*/month=*/day=*/agent=*/*.parquet']
  );
```

Partition pruning works automatically because BigQuery recognizes the `year=/month=/day=/agent=` segments:

```sql
SELECT
  id,
  agent_name,
  TIMESTAMP_MICROS(DIV(created_at, 1000)) AS created_at,
  total_input_tokens,
  estimated_cost_usd,
  JSON_EXTRACT_ARRAY(messages_json, '$') AS messages
FROM `my_project.omnia.sessions`
WHERE year = 2026 AND month = 4
  AND agent_name = 'my-agent';
```

For native-table performance, use [BigQuery Data Transfer Service](https://cloud.google.com/bigquery/docs/dts-introduction) to ingest the same files into a partitioned managed table — again, no Omnia-side changes needed.

## Querying from ClickHouse

ClickHouse reads S3 Parquet natively via the `s3` table function:

```sql
SELECT
  id,
  agent_name,
  fromUnixTimestamp64Nano(created_at) AS created_at,
  total_input_tokens,
  estimated_cost_usd,
  JSONExtractArrayRaw(messages_json)  AS messages
FROM s3(
  'https://my-omnia-archive.s3.us-east-1.amazonaws.com/sessions/year=2026/month=04/**/*.parquet',
  'AKIA...', 'SECRET...',
  'Parquet'
)
WHERE agent_name = 'my-agent';
```

Or create a persistent table using the `S3` engine:

```sql
CREATE TABLE omnia_sessions
ENGINE = S3(
  'https://my-omnia-archive.s3.us-east-1.amazonaws.com/sessions/**/*.parquet',
  'AKIA...', 'SECRET...',
  'Parquet'
);
```

## Querying from Athena

Athena reads Parquet on S3 via a Glue-registered external table:

```sql
CREATE EXTERNAL TABLE omnia_sessions (
  id                   STRING,
  agent_name           STRING,
  namespace            STRING,
  workspace_name       STRING,
  status               STRING,
  created_at           BIGINT,
  updated_at           BIGINT,
  expires_at           BIGINT,
  ended_at             BIGINT,
  message_count        INT,
  tool_call_count      INT,
  total_input_tokens   BIGINT,
  total_output_tokens  BIGINT,
  estimated_cost_usd   DOUBLE,
  tags                 STRING,
  state                STRING,
  last_message_preview STRING,
  messages_json        STRING
)
PARTITIONED BY (year INT, month INT, day INT, agent STRING)
STORED AS PARQUET
LOCATION 's3://my-omnia-archive/sessions/'
TBLPROPERTIES ('parquet.compression' = 'SNAPPY');

MSCK REPAIR TABLE omnia_sessions;
```

Then query with partition pruning:

```sql
SELECT
  id,
  agent_name,
  from_unixtime(created_at / 1000000000) AS created_at,
  CAST(json_extract(messages_json, '$') AS ARRAY<JSON>) AS messages
FROM omnia_sessions
WHERE year = 2026 AND month = 4
  AND agent_name = 'my-agent';
```

## Querying from DuckDB

For local / ad-hoc exploration, DuckDB reads Parquet on object storage directly:

```sql
INSTALL httpfs;
LOAD httpfs;

SET s3_region = 'us-east-1';
SET s3_access_key_id = 'AKIA...';
SET s3_secret_access_key = 'SECRET...';

SELECT
  id,
  agent_name,
  make_timestamp(created_at // 1000)                 AS created_at,
  total_input_tokens,
  estimated_cost_usd,
  json_extract(messages_json, '$')::JSON             AS messages
FROM read_parquet('s3://my-omnia-archive/sessions/**/*.parquet', hive_partitioning = true)
WHERE year = 2026 AND month = 4
  AND agent_name = 'my-agent';
```

DuckDB infers the `year`, `month`, `day`, and `agent` partition columns automatically when `hive_partitioning = true` is set.

## Retention and cleanup

Retention is declarative via `SessionRetentionPolicy`. The compaction CronJob enforces the warm→cold move, and optionally the cold purge after a configurable age. Cleanup of files in the bucket past the cold retention period happens in the same job — no separate lifecycle policy needed, though you can still configure an S3 Lifecycle rule as a backstop if you prefer belt-and-braces.

See the `SessionRetentionPolicy` CRD reference for full field documentation.

## Why not a dedicated sync controller?

Earlier versions of Omnia shipped a `SessionAnalyticsSync` CRD that imported the Parquet archive into Snowflake-native tables via a Kubernetes controller. It was removed before general availability because every major warehouse can query object-store Parquet natively as external tables, and the managed ingestion paths (Snowpipe, BigQuery Data Transfer Service, ClickHouse `INSERT FROM s3()`, Glue crawlers) are strictly better tools for high-throughput native-table loads than anything we could ship inside the operator.

If you need native-table performance for high-volume BI dashboards, use your warehouse's own ingestion pipeline pointed at the Omnia archive bucket. The Parquet schema above is stable and Hive-partitioned, so any standard ingestion tool will work without custom adapters.
