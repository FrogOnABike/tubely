package main

import "testing"

func TestExtractBucketKey(t *testing.T) {
	cases := []struct{ raw, wantB, wantK string }{
		{"mybucket,mykey/path.mp4", "mybucket", "mykey/path.mp4"},
		{"https://mybucket.s3.us-west-2.amazonaws.com/landscape/abc.mp4", "mybucket", "landscape/abc.mp4"},
		{"https://s3.amazonaws.com/mybucket/landscape/abc.mp4", "mybucket", "landscape/abc.mp4"},
		{"  mybucket , key.mp4  ", "mybucket", "key.mp4"},
		{"not-a-url-or-pair", "", ""},
		{"", "", ""},
	}

	for _, c := range cases {
		b, k, ok := extractBucketKey(c.raw)
		if c.wantB == "" && ok {
			t.Fatalf("expected failure parsing %q, got %q,%q", c.raw, b, k)
		}
		if c.wantB != "" {
			if !ok {
				t.Fatalf("expected success parsing %q", c.raw)
			}
			if b != c.wantB || k != c.wantK {
				t.Fatalf("for %q got (%q,%q) want (%q,%q)", c.raw, b, k, c.wantB, c.wantK)
			}
		}
	}
}
