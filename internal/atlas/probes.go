package atlas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultProbeAPIURL = "https://atlas.ripe.net/api/v2/probes/"

// ProbeMetadata is the public RIPE Atlas inventory record used to enrich live
// measurement probe IDs. Coordinates are RIPE's privacy-obfuscated values.
type ProbeMetadata struct {
	ID              int64
	DisplayName     string
	Description     string
	ProbeType       string
	CountryCode     string
	Latitude        float64
	Longitude       float64
	IsAnchor        bool
	IsPublic        bool
	FirmwareVersion int64
	StatusID        int64
	StatusName      string
	StatusSince     int64
	FirstConnected  int64
	LastConnected   int64
	PrefixV4        string
	PrefixV6        string
	ASNv4           int64
	ASNv6           int64
	TagSlugs        []string
}

type probeAPIRecord struct {
	ID              int64  `json:"id"`
	Description     string `json:"description"`
	CountryCode     string `json:"country_code"`
	IsAnchor        bool   `json:"is_anchor"`
	IsPublic        bool   `json:"is_public"`
	FirmwareVersion int64  `json:"firmware_version"`
	StatusSince     int64  `json:"status_since"`
	FirstConnected  int64  `json:"first_connected"`
	LastConnected   int64  `json:"last_connected"`
	PrefixV4        string `json:"prefix_v4"`
	PrefixV6        string `json:"prefix_v6"`
	ASNv4           int64  `json:"asn_v4"`
	ASNv6           int64  `json:"asn_v6"`
	Geometry        struct {
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
	Status struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"status"`
	Tags []struct {
		Slug string `json:"slug"`
	} `json:"tags"`
}

type probeAPIPage struct {
	Results []probeAPIRecord `json:"results"`
}

// ProbeClient retrieves public probe inventory in batches. A list endpoint is
// used instead of one request per probe so firehose enrichment stays bounded.
type ProbeClient struct {
	BaseURL   string
	Client    *http.Client
	UserAgent string
}

func (c *ProbeClient) Fetch(ctx context.Context, ids []int64) ([]ProbeMetadata, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultProbeAPIURL
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	userAgent := c.UserAgent
	if userAgent == "" {
		userAgent = "ripestream/0.1 (+https://atlas.ripe.net/)"
	}

	unique := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			unique[id] = struct{}{}
		}
	}
	ordered := make([]int64, 0, len(unique))
	for id := range unique {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	out := make([]ProbeMetadata, 0, len(ordered))
	for start := 0; start < len(ordered); start += 500 {
		end := min(start+500, len(ordered))
		batch, err := fetchProbeBatch(ctx, client, base, userAgent, ordered[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

func fetchProbeBatch(ctx context.Context, client *http.Client, base, userAgent string, ids []int64) ([]ProbeMetadata, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("probe api url: %w", err)
	}
	idParts := make([]string, len(ids))
	for i, id := range ids {
		idParts[i] = strconv.FormatInt(id, 10)
	}
	q := u.Query()
	q.Set("id__in", strings.Join(idParts, ","))
	q.Set("page_size", "500")
	q.Set("format[datetime]", "unix")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("probe api request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("probe api http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var page probeAPIPage
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&page); err != nil {
		return nil, fmt.Errorf("probe api decode: %w", err)
	}
	out := make([]ProbeMetadata, 0, len(page.Results))
	for _, record := range page.Results {
		out = append(out, normalizeProbeMetadata(record))
	}
	return out, nil
}

func normalizeProbeMetadata(record probeAPIRecord) ProbeMetadata {
	tags := make([]string, 0, len(record.Tags))
	for _, tag := range record.Tags {
		if tag.Slug != "" {
			tags = append(tags, tag.Slug)
		}
	}
	sort.Strings(tags)
	metadata := ProbeMetadata{
		ID: record.ID, Description: strings.TrimSpace(record.Description),
		CountryCode: strings.ToUpper(record.CountryCode), IsAnchor: record.IsAnchor,
		IsPublic: record.IsPublic, FirmwareVersion: record.FirmwareVersion,
		StatusID: record.Status.ID, StatusName: record.Status.Name, StatusSince: record.StatusSince,
		FirstConnected: record.FirstConnected, LastConnected: record.LastConnected,
		PrefixV4: record.PrefixV4, PrefixV6: record.PrefixV6,
		ASNv4: record.ASNv4, ASNv6: record.ASNv6, TagSlugs: tags,
	}
	if len(record.Geometry.Coordinates) >= 2 {
		metadata.Longitude = record.Geometry.Coordinates[0]
		metadata.Latitude = record.Geometry.Coordinates[1]
	}
	metadata.ProbeType = probeType(record.IsAnchor, tags)
	metadata.DisplayName = metadata.Description
	if metadata.DisplayName == "" {
		location := metadata.CountryCode
		if location == "" {
			location = "unknown location"
		}
		metadata.DisplayName = metadata.ProbeType + " in " + location
	}
	return metadata
}

func probeType(anchor bool, tags []string) string {
	set := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		set[tag] = struct{}{}
	}
	if anchor {
		if _, ok := set["system-virtual"]; ok {
			return "Virtual anchor"
		}
		return "Anchor"
	}
	if _, ok := set["system-software"]; ok {
		return "Software probe"
	}
	for version := 5; version >= 1; version-- {
		if _, ok := set["system-v"+strconv.Itoa(version)]; ok {
			return "Hardware v" + strconv.Itoa(version)
		}
	}
	return "RIPE Atlas probe"
}
