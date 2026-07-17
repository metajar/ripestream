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
TTL toDateTime(received_at) + INTERVAL 24 HOUR DELETE
SETTINGS index_granularity = 8192;

-- CREATE TABLE IF NOT EXISTS does not update existing installations. Keep the
-- retention migration explicit so applying the embedded schema also schedules
-- deletion of rows that are already older than 24 hours.
ALTER TABLE ripestream.atlas_results
MODIFY TTL toDateTime(received_at) + INTERVAL 24 HOUR DELETE;

-- Idempotent migration for databases created before typed PING metrics were
-- introduced. Existing rows receive zero defaults; new ingestion fills them.
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS sent UInt32 AFTER fw;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS rcvd UInt32 AFTER sent;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS avg_rtt_ms Float64 AFTER rcvd;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS min_rtt_ms Float64 AFTER avg_rtt_ms;
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS max_rtt_ms Float64 AFTER min_rtt_ms;

-- Route correlation is latency-sensitive and runs repeatedly while its page is
-- open. Flatten traceroute replies once on ingestion instead of expanding two
-- levels of raw JSON across a 24-hour window on every request.
CREATE TABLE IF NOT EXISTS ripestream.atlas_results_traceroute_hops
(
    received_at DateTime64(3, 'UTC') CODEC(Delta, ZSTD),
    timestamp   DateTime('UTC'),
    msm_id      UInt32,
    prb_id      UInt32,
    dst_addr    String,
    hop_index   UInt16,
    reply_index UInt8,
    addr        String,
    rtt         Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (timestamp, addr, prb_id, msm_id, hop_index, reply_index)
TTL toDateTime(received_at) + INTERVAL 24 HOUR DELETE
SETTINGS index_granularity = 8192;

ALTER TABLE ripestream.atlas_results_traceroute_hops
MODIFY TTL toDateTime(received_at) + INTERVAL 24 HOUR DELETE;

CREATE MATERIALIZED VIEW IF NOT EXISTS ripestream.atlas_results_traceroute_hops_mv
TO ripestream.atlas_results_traceroute_hops
AS
SELECT received_at, timestamp, msm_id, prb_id, dst_addr,
       toUInt16(hop_index) AS hop_index,
       toUInt8(reply_index) AS reply_index,
       JSONExtractString(reply_json, 'from') AS addr,
       JSONExtractFloat(reply_json, 'rtt') AS rtt
FROM
(
    SELECT received_at, timestamp, msm_id, prb_id, dst_addr, hop_index,
           arrayJoin(arrayEnumerate(JSONExtractArrayRaw(hop_json, 'result'))) AS reply_index,
           arrayElement(JSONExtractArrayRaw(hop_json, 'result'), reply_index) AS reply_json
    FROM
    (
        SELECT received_at, timestamp, msm_id, prb_id, dst_addr,
               arrayJoin(arrayEnumerate(JSONExtractArrayRaw(result_json, 'result'))) AS hop_index,
               arrayElement(JSONExtractArrayRaw(result_json, 'result'), hop_index) AS hop_json
        FROM ripestream.atlas_results
        WHERE type = 'traceroute'
    )
)
WHERE addr != '' AND addr != dst_addr AND rtt > 0 AND rtt < 10000;

-- Existing installations receive as much typed history as remains inside the
-- 24-hour retention window. ApplySchema runs before ingestion starts. The
-- scalar guard makes this a one-time backfill and avoids duplicating rows on
-- subsequent restarts.
INSERT INTO ripestream.atlas_results_traceroute_hops
SELECT received_at, timestamp, msm_id, prb_id, dst_addr,
       toUInt16(hop_index) AS hop_index,
       toUInt8(reply_index) AS reply_index,
       JSONExtractString(reply_json, 'from') AS addr,
       JSONExtractFloat(reply_json, 'rtt') AS rtt
FROM
(
    SELECT received_at, timestamp, msm_id, prb_id, dst_addr, hop_index,
           arrayJoin(arrayEnumerate(JSONExtractArrayRaw(hop_json, 'result'))) AS reply_index,
           arrayElement(JSONExtractArrayRaw(hop_json, 'result'), reply_index) AS reply_json
    FROM
    (
        SELECT received_at, timestamp, msm_id, prb_id, dst_addr,
               arrayJoin(arrayEnumerate(JSONExtractArrayRaw(result_json, 'result'))) AS hop_index,
               arrayElement(JSONExtractArrayRaw(result_json, 'result'), hop_index) AS hop_json
        FROM ripestream.atlas_results
        PREWHERE type = 'traceroute'
          AND timestamp >= now() - INTERVAL 31 HOUR
    )
)
WHERE addr != '' AND addr != dst_addr AND rtt > 0 AND rtt < 10000
  AND (SELECT count() FROM ripestream.atlas_results_traceroute_hops) = 0;
