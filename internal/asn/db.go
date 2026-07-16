// Package asn looks up Autonomous System numbers from a MaxMind GeoLite2-ASN
// database (.mmdb).
package asn

import (
	"fmt"
	"net/netip"

	"github.com/oschwald/maxminddb-golang/v2"
)

// DefaultDBPath is the repo-local GeoLite2 ASN database path.
const DefaultDBPath = "geolite/GeoLite2-ASN.mmdb"

// Record is the GeoLite2-ASN schema for a single IP.
type Record struct {
	Number       uint   `maxminddb:"autonomous_system_number"`
	Organization string `maxminddb:"autonomous_system_organization"`
}

// DB is a read-only GeoLite2-ASN database.
type DB struct {
	r *maxminddb.Reader
}

// Open opens a GeoLite2-ASN .mmdb file.
func Open(path string) (*DB, error) {
	if path == "" {
		path = DefaultDBPath
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open asn db %q: %w", path, err)
	}
	return &DB{r: r}, nil
}

// Close releases the underlying file mapping.
func (d *DB) Close() error {
	if d == nil || d.r == nil {
		return nil
	}
	return d.r.Close()
}

// Lookup returns the ASN and organization for ip. ok is false when the address
// is invalid, private/unmapped, or has no ASN entry.
func (d *DB) Lookup(ip string) (number int64, org string, ok bool) {
	if d == nil || d.r == nil || ip == "" {
		return 0, "", false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return 0, "", false
	}
	var rec Record
	err = d.r.Lookup(addr).Decode(&rec)
	if err != nil || rec.Number == 0 {
		return 0, "", false
	}
	return int64(rec.Number), rec.Organization, true
}
