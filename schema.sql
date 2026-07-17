-- RIPE Atlas result stream schema.
--
-- The live firehose delivers a mix of measurement types (ping, traceroute,
-- dns, http, sslcert, ntp, ...). Their payloads share a common envelope of
-- fields, but the type-specific detail (per-packet RTTs, hop lists, DNS
-- answers, HTTP headers, ...) varies wildly.
--
-- Strategy: store the universal envelope as typed, queryable columns and keep
-- the full original payload verbatim in `result_json` so nothing is lost and
-- type-specific analysis can be done later via JSONExtract*/materialized views.

CREATE DATABASE IF NOT EXISTS ripestream;

CREATE TABLE IF NOT EXISTS ripestream.atlas_results
(
    received_at DateTime64(3, 'UTC') DEFAULT now64(3) CODEC(Delta, ZSTD),
    timestamp   DateTime('UTC'),
    msm_id      UInt32,
    prb_id      UInt32,
    type        LowCardinality(String),
    msm_name    LowCardinality(String),
    from_ip     String,
    af          UInt8,
    proto       LowCardinality(String),
    dst_name    String,
    dst_addr    String,
    src_addr    String,
    fw          UInt32,
    sent        UInt32,
    rcvd        UInt32,
    avg_rtt_ms  Float64,
    min_rtt_ms  Float64,
    max_rtt_ms  Float64,
    result_json String CODEC(ZSTD(3)),

    -- Lets queries like `result_json LIKE '%rtt%'` skip irrelevant granules.
    INDEX idx_result_json result_json TYPE tokenbf_v1(30720, 3, 0) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (type, msm_id, prb_id, timestamp)
SETTINGS index_granularity = 8192;

-- Idempotent migration for databases created before typed PING metrics were
-- introduced. Existing rows receive zero defaults; new ingestion fills them.
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS sent UInt32 AFTER fw;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS rcvd UInt32 AFTER sent;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS avg_rtt_ms Float64 AFTER rcvd;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS min_rtt_ms Float64 AFTER avg_rtt_ms;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS max_rtt_ms Float64 AFTER min_rtt_ms;
