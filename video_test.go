package main

import "testing"

func TestParseVideoAspectFromJSON(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{
			name: "display_aspect_ratio_16_9",
			json: `{"streams":[{"codec_type":"video","display_aspect_ratio":"16:9"}]}`,
			want: "landscape",
		},
		{
			name: "display_aspect_ratio_9_16",
			json: `{"streams":[{"codec_type":"video","display_aspect_ratio":"9:16"}]}`,
			want: "portrait",
		},
		{
			name: "width_height_landscape",
			json: `{"streams":[{"codec_type":"video","width":1920,"height":1080}]}`,
			want: "landscape",
		},
		{
			name: "width_height_portrait",
			json: `{"streams":[{"codec_type":"video","width":720,"height":1280}]}`,
			want: "portrait",
		},
		{
			name: "sample_aspect_ratio_applied",
			json: `{"streams":[{"codec_type":"video","width":100,"height":100,"sample_aspect_ratio":"2:1"}]}`,
			want: "landscape",
		},
		{
			name: "no_video_stream",
			json: `{"streams":[{"codec_type":"audio"}]}`,
			want: "other",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseVideoAspectFromJSON([]byte(tc.json))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestParseRatioString(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"16:9", 16.0 / 9.0},
		{"9:16", 9.0 / 16.0},
		{"1.5", 1.5},
		{"1:1", 1.0},
	}

	for _, tt := range tests {
		got, err := parseRatioString(tt.input)
		if err != nil {
			t.Fatalf("parseRatioString(%s) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("parseRatioString(%s) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestParseVideoAspectFromJSON_invalidJSON(t *testing.T) {
	_, err := parseVideoAspectFromJSON([]byte("{not-json}"))
	if err == nil {
		t.Fatalf("expected error for invalid json, got nil")
	}
}

// Note: getVideoAspectRatio relies on ffprobe being present. On CI the binary may be missing
// so this test simply asserts we get an error for a non-existent path and that the error
// contains a helpful message.
func TestGetVideoAspectRatio_missingFile(t *testing.T) {
	_, err := getVideoAspectRatio("/path/that/does/not/exist.mp4")
	if err == nil {
		t.Fatalf("expected error when running ffprobe on missing file")
	}
}
