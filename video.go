package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

type video struct {
	Streams []struct {
		Index              int    `json:"index"`
		CodecName          string `json:"codec_name,omitempty"`
		CodecLongName      string `json:"codec_long_name,omitempty"`
		Profile            string `json:"profile,omitempty"`
		CodecType          string `json:"codec_type"`
		CodecTagString     string `json:"codec_tag_string"`
		CodecTag           string `json:"codec_tag"`
		Width              int    `json:"width,omitempty"`
		Height             int    `json:"height,omitempty"`
		CodedWidth         int    `json:"coded_width,omitempty"`
		CodedHeight        int    `json:"coded_height,omitempty"`
		HasBFrames         int    `json:"has_b_frames,omitempty"`
		SampleAspectRatio  string `json:"sample_aspect_ratio,omitempty"`
		DisplayAspectRatio string `json:"display_aspect_ratio,omitempty"`
		PixFmt             string `json:"pix_fmt,omitempty"`
		Level              int    `json:"level,omitempty"`
		ColorRange         string `json:"color_range,omitempty"`
		ColorSpace         string `json:"color_space,omitempty"`
		ColorTransfer      string `json:"color_transfer,omitempty"`
		ColorPrimaries     string `json:"color_primaries,omitempty"`
		ChromaLocation     string `json:"chroma_location,omitempty"`
		FieldOrder         string `json:"field_order,omitempty"`
		Refs               int    `json:"refs,omitempty"`
		IsAvc              string `json:"is_avc,omitempty"`
		NalLengthSize      string `json:"nal_length_size,omitempty"`
		ID                 string `json:"id"`
		RFrameRate         string `json:"r_frame_rate"`
		AvgFrameRate       string `json:"avg_frame_rate"`
		TimeBase           string `json:"time_base"`
		StartPts           int    `json:"start_pts"`
		StartTime          string `json:"start_time"`
		DurationTs         int    `json:"duration_ts"`
		Duration           string `json:"duration"`
		BitRate            string `json:"bit_rate,omitempty"`
		BitsPerRawSample   string `json:"bits_per_raw_sample,omitempty"`
		NbFrames           string `json:"nb_frames"`
		ExtradataSize      int    `json:"extradata_size"`
		Disposition        struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
			NonDiegetic     int `json:"non_diegetic"`
			Captions        int `json:"captions"`
			Descriptions    int `json:"descriptions"`
			Metadata        int `json:"metadata"`
			Dependent       int `json:"dependent"`
			StillImage      int `json:"still_image"`
			Multilayer      int `json:"multilayer"`
		} `json:"disposition"`
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
			Encoder     string `json:"encoder"`
			Timecode    string `json:"timecode"`
		} `json:"tags,omitempty"`
		SampleFmt      string `json:"sample_fmt,omitempty"`
		SampleRate     string `json:"sample_rate,omitempty"`
		Channels       int    `json:"channels,omitempty"`
		ChannelLayout  string `json:"channel_layout,omitempty"`
		BitsPerSample  int    `json:"bits_per_sample,omitempty"`
		InitialPadding int    `json:"initial_padding,omitempty"`
	} `json:"streams"`
}

// processVideoForFastStart uses ffmpeg to process the video file at filePath
// so that it is optimized for fast start (i.e., the moov atom is at the beginning).
// It returns the path to the processed file.
func processVideoForFastStart(filePath string) (string, error) {
	// Use ffmpeg to process the video for fast start
	outputPath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-movflags", "faststart", "-f", "mp4", "-c", "copy", outputPath)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		// Include stderr content to make debugging easier (missing ffprobe, bad file, etc.)
		stderr := strings.TrimSpace(errOut.String())
		if stderr == "" {
			return "", fmt.Errorf("ffprobe failed: %w", err)
		}
		return "", fmt.Errorf("ffprobe failed: %w: %s", err, stderr)
	}
	return outputPath, nil

}
func getVideoAspectRatio(filePath string) (string, error) {
	// Use ffprobe to get video metadata
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		// Include stderr content to make debugging easier (missing ffprobe, bad file, etc.)
		stderr := strings.TrimSpace(errOut.String())
		if stderr == "" {
			return "", fmt.Errorf("ffprobe failed: %w", err)
		}
		return "", fmt.Errorf("ffprobe failed: %w: %s", err, stderr)
	}

	// If ffprobe produced no output, return an explicit error
	if strings.TrimSpace(out.String()) == "" {
		return "", fmt.Errorf("ffprobe returned no output for %s", filePath)
	}

	// Parse the ffprobe output and determine the aspect ratio
	return parseVideoAspectFromJSON(out.Bytes())
}

// parseVideoAspectFromJSON extracts the primary video stream from ffprobe JSON
// and decides whether the video is "landscape", "portrait" or "other".
//
// This function is separated so it can be unit-tested without calling ffprobe.
func parseVideoAspectFromJSON(data []byte) (string, error) {
	var vid video
	if err := json.Unmarshal(data, &vid); err != nil {
		return "", err
	}

	for _, stream := range vid.Streams {
		if stream.CodecType != "video" {
			continue
		}

		// Try display_aspect_ratio first (often present and authoritative)
		if stream.DisplayAspectRatio != "" {
			if r, err := parseRatioString(stream.DisplayAspectRatio); err == nil {
				return aspectLabelFromRatio(r), nil
			}
		}

		// Fall back to sample_aspect_ratio combined with width/height
		if stream.SampleAspectRatio != "" && stream.Width > 0 && stream.Height > 0 {
			if sar, err := parseRatioString(stream.SampleAspectRatio); err == nil {
				r := (float64(stream.Width) / float64(stream.Height)) * sar
				return aspectLabelFromRatio(r), nil
			}
		}

		// Final fallback: use coded width/height or width/height if available
		w := stream.CodedWidth
		h := stream.CodedHeight
		if w == 0 || h == 0 {
			w = stream.Width
			h = stream.Height
		}

		if w > 0 && h > 0 {
			r := float64(w) / float64(h)
			return aspectLabelFromRatio(r), nil
		}

		// If we can't determine anything, return other
		return "other", nil
	}

	// No video stream found
	return "other", nil
}

func parseRatioString(s string) (float64, error) {
	// Common ffprobe formats are like "16:9" or "1:1". Accept both ints and floats.
	parts := []rune(s)
	// Quick check — if it doesn't contain ':' try parsing as float
	if !containsColon(s) {
		return strconv.ParseFloat(s, 64)
	}
	// split on ':'
	idx := stringsIndexRune(parts, ':')
	if idx < 0 {
		return 0, strconv.ErrSyntax
	}
	left := string(parts[:idx])
	right := string(parts[idx+1:])
	l, err := strconv.ParseFloat(left, 64)
	if err != nil {
		return 0, err
	}
	r, err := strconv.ParseFloat(right, 64)
	if err != nil {
		return 0, err
	}
	if r == 0 {
		return 0, strconv.ErrRange
	}
	return l / r, nil
}

// aspectLabelFromRatio returns a high-level label for a numeric aspect ratio.
func aspectLabelFromRatio(r float64) string {
	// Use a tolerance to avoid flakiness for near-square inputs
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return "other"
	}
	if r > 1.1 {
		return "landscape"
	}
	if r < 0.9 {
		return "portrait"
	}
	return "other"
}

// --- lightweight rune helpers (to avoid adding extra stdlib imports) ---
func containsColon(s string) bool {
	for _, r := range s {
		if r == ':' {
			return true
		}
	}
	return false
}

func stringsIndexRune(runes []rune, target rune) int {
	for i, r := range runes {
		if r == target {
			return i
		}
	}
	return -1
}
