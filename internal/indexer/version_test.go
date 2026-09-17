package indexer_test

import (
	"testing"

	"github.com/follenfang/wowdoc/internal/indexer"
)

func TestBuildInterfaceFromVersion(t *testing.T) {
	cases := []struct{ payload, want string }{
		{"1.60.1.69893", "16001"},
		{"1.60.1.69893\n", "16001"},
		{"12.1.0.69814", "120100"},
		{"12.0.0.60000", "120000"},
		{"5.5.4.69585", "50504"},
		{"3.80.2.69874", "38002"},
		{"1.15.9.69722", "11509"},
		{"2.5.6.69795", "20506"},
		{"1.60.1", "16001"},
		{"\xef\xbb\xbf1.60.1.69893", "16001"},
		{"", ""},
		{"empty", ""},
		{"latest", ""},
		{"1.60", ""},
		{"a.b.c", ""},
		{"1.60.x", ""},
	}
	for _, tc := range cases {
		if got := indexer.BuildInterfaceFromVersion([]byte(tc.payload)); got != tc.want {
			t.Fatalf("payload %q: got %q, want %q", tc.payload, got, tc.want)
		}
	}
}
