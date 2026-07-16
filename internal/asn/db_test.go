package asn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLookupGoogleDNS(t *testing.T) {
	path := findDB(t)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	num, org, ok := db.Lookup("8.8.8.8")
	if !ok {
		t.Fatal("expected ASN for 8.8.8.8")
	}
	if num != 15169 {
		t.Fatalf("asn=%d want 15169", num)
	}
	if org == "" {
		t.Fatal("expected organization")
	}
	t.Logf("8.8.8.8 -> AS%d (%s)", num, org)
}

func TestLookupInvalid(t *testing.T) {
	path := findDB(t)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, _, ok := db.Lookup("not-an-ip"); ok {
		t.Fatal("expected miss")
	}
	if _, _, ok := db.Lookup("10.0.0.1"); ok {
		t.Fatal("private IP should miss")
	}
}

func findDB(t *testing.T) string {
	t.Helper()
	candidates := []string{
		DefaultDBPath,
		filepath.Join("..", "..", DefaultDBPath),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	t.Skip("GeoLite2-ASN.mmdb not found")
	return ""
}
