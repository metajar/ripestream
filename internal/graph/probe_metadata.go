package graph

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"ripestream/internal/atlas"
)

const probeMetadataBatchSize = 500

// RunProbeMetadata continuously backfills and refreshes public RIPE Atlas
// inventory onto :Probe nodes. It is deliberately independent of the firehose
// write path so API latency or outages cannot block measurement ingestion.
func (s *Store) RunProbeMetadata(ctx context.Context, client *atlas.ProbeClient, refresh time.Duration) error {
	if refresh <= 0 {
		refresh = 24 * time.Hour
	}
	if client == nil {
		client = &atlas.ProbeClient{}
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		ids, err := s.probeIDsNeedingMetadata(ctx, time.Now().Add(-refresh).Unix(), probeMetadataBatchSize)
		if err != nil {
			slog.Warn("probe metadata scan failed", "err", err)
			if !waitProbeMetadata(ctx, time.Minute) {
				return nil
			}
			continue
		}
		if len(ids) == 0 {
			if !waitProbeMetadata(ctx, time.Minute) {
				return nil
			}
			continue
		}

		started := time.Now()
		metadata, err := client.Fetch(ctx, ids)
		if err != nil {
			slog.Warn("probe metadata fetch failed", "probes", len(ids), "err", err)
			if !waitProbeMetadata(ctx, time.Minute) {
				return nil
			}
			continue
		}
		if err := s.upsertProbeMetadata(ctx, ids, metadata, time.Now().Unix()); err != nil {
			slog.Warn("probe metadata upsert failed", "probes", len(ids), "err", err)
			if !waitProbeMetadata(ctx, time.Minute) {
				return nil
			}
			continue
		}
		slog.Info("probe metadata refreshed", "requested", len(ids), "received", len(metadata),
			"took", time.Since(started).Round(time.Millisecond))
		// The RIPE endpoint accepts 500 IDs per call. Pace consecutive backfill
		// batches while still filling a large existing graph promptly.
		if len(ids) == probeMetadataBatchSize && !waitProbeMetadata(ctx, 2*time.Second) {
			return nil
		}
	}
}

func waitProbeMetadata(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Store) probeIDsNeedingMetadata(ctx context.Context, cutoff int64, limit int) ([]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, `
MATCH (p:Probe)
WHERE coalesce(p.metadata_checked_at, p.metadata_updated_at, 0) < $cutoff
RETURN p.id AS id
ORDER BY coalesce(p.metadata_checked_at, p.metadata_updated_at, 0) ASC, p.id ASC
LIMIT $limit`, map[string]any{"cutoff": cutoff, "limit": limit})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := asInt(row["id"]); id > 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *Store) upsertProbeMetadata(ctx context.Context, checked []int64, metadata []atlas.ProbeMetadata, fetchedAt int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(checked) > 0 {
		checkedValues := make([]any, 0, len(checked))
		for _, id := range checked {
			checkedValues = append(checkedValues, id)
		}
		if _, err := s.graph.Query(`
UNWIND $ids AS probe_id
MERGE (p:Probe {id: probe_id})
SET p.metadata_checked_at = $fetched_at`, map[string]any{
			"ids": checkedValues, "fetched_at": fetchedAt,
		}, nil); err != nil {
			return err
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	ops := make([]any, 0, len(metadata))
	for _, item := range metadata {
		ops = append(ops, map[string]any{
			"id": item.ID, "display_name": item.DisplayName, "description": item.Description,
			"probe_type": item.ProbeType, "country_code": item.CountryCode,
			"latitude": item.Latitude, "longitude": item.Longitude,
			"is_anchor": item.IsAnchor, "is_public": item.IsPublic,
			"firmware_version": item.FirmwareVersion,
			"status_id":        item.StatusID, "status_name": item.StatusName, "status_since": item.StatusSince,
			"first_connected": item.FirstConnected, "last_connected": item.LastConnected,
			"prefix_v4": item.PrefixV4, "prefix_v6": item.PrefixV6,
			"asn_v4": item.ASNv4, "asn_v6": item.ASNv6,
			"tag_slugs": strings.Join(item.TagSlugs, ","), "metadata_updated_at": fetchedAt,
			"metadata_checked_at": fetchedAt,
		})
	}
	_, err := s.graph.Query(`
UNWIND $ops AS op
MERGE (p:Probe {id: op.id})
SET p.display_name = op.display_name,
    p.description = op.description,
    p.probe_type = op.probe_type,
    p.country_code = op.country_code,
    p.latitude = op.latitude,
    p.longitude = op.longitude,
    p.is_anchor = op.is_anchor,
    p.is_public = op.is_public,
    p.firmware_version = op.firmware_version,
    p.status_id = op.status_id,
    p.status_name = op.status_name,
    p.status_since = op.status_since,
    p.first_connected = op.first_connected,
    p.last_connected = op.last_connected,
    p.prefix_v4 = op.prefix_v4,
    p.prefix_v6 = op.prefix_v6,
    p.asn_v4 = op.asn_v4,
    p.asn_v6 = op.asn_v6,
    p.tag_slugs = op.tag_slugs,
    p.metadata_checked_at = op.metadata_checked_at,
    p.metadata_updated_at = op.metadata_updated_at`, map[string]any{"ops": ops}, nil)
	return err
}
