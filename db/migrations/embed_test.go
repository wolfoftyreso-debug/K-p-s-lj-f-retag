package migrations

import (
	"testing"
	"testing/fstest"
)

func TestCatalogueRejectsAmbiguousHistory(t *testing.T) {
	for name, files := range map[string]fstest.MapFS{
		"empty":             {},
		"invalid name":      {"1_test.sql": {Data: []byte("SELECT 1")}},
		"gap":               {"000002_test.sql": {Data: []byte("SELECT 1")}},
		"duplicate version": {"000001_a.sql": {Data: []byte("SELECT 1")}, "000001_b.sql": {Data: []byte("SELECT 2")}},
		"empty body":        {"000001_a.sql": {}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := catalogue(files); err == nil {
				t.Fatal("expected invalid catalogue rejection")
			}
		})
	}
}

func TestCatalogueHashesExactReviewedBytes(t *testing.T) {
	first, err := catalogue(fstest.MapFS{"000001_a.sql": {Data: []byte("SELECT 1;\n")}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := catalogue(fstest.MapFS{"000001_a.sql": {Data: []byte("SELECT 2;\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].checksum == second[0].checksum {
		t.Fatal("content change did not change checksum")
	}
}
