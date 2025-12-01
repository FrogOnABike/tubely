package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

// generatePresignedURL generates a presigned URL for accessing an S3 object.
func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	if s3Client == nil {
		return "", fmt.Errorf("s3 client is not configured")
	}
	psCient := s3.NewPresignClient(s3Client)
	req, err := psCient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	// If there's no VideoURL set, return unchanged
	if video.VideoURL == nil || *video.VideoURL == "" {
		return video, nil
	}

	raw := *video.VideoURL

	// Support two historical formats stored in DB:
	//  - "bucket,key"
	//  - full s3 URL (e.g. https://<bucket>.s3.<region>.amazonaws.com/<key> OR https://s3.amazonaws.com/<bucket>/<key>)
	var bucket, key string

	// Try to extract bucket/key using helper
	if b, k, ok := extractBucketKey(raw); ok {
		bucket = b
		key = k
	} else {
		// Try to parse as a URL and extract bucket/key
		u, err := url.Parse(raw)
		if err == nil {
			host := u.Host
			if strings.Contains(host, ".s3.") && strings.Contains(host, "amazonaws.com") {
				// host looks like <bucket>.s3.<region>.amazonaws.com
				hostParts := strings.Split(host, ".")
				if len(hostParts) > 0 {
					bucket = hostParts[0]
					key = strings.TrimPrefix(u.Path, "/")
				}
			} else if strings.HasPrefix(host, "s3.amazonaws.com") || strings.HasPrefix(host, "s3.") {
				// path is /bucket/key
				p := strings.TrimPrefix(u.Path, "/")
				segs := strings.Split(p, "/")
				if len(segs) >= 2 {
					bucket = segs[0]
					key = strings.Join(segs[1:], "/")
				}
			}
		}
	}

	if bucket == "" || key == "" {
		// Can't extract bucket/key; return unchanged instead of panicking
		return video, nil
	}

	psURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 5*time.Minute)
	if err != nil {
		return database.Video{}, err
	}
	video.VideoURL = &psURL
	return video, nil

}

// extractBucketKey tries to parse the stored video URL value into bucket and key.
// Supported formats:
//   - "bucket,key" (legacy)
//   - https://<bucket>.s3.<region>.amazonaws.com/<key>
//   - https://s3.amazonaws.com/<bucket>/<key> (path-style)
func extractBucketKey(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}

	// Quick bucket,key format
	parts := strings.Split(raw, ",")
	if len(parts) == 2 {
		b := strings.TrimSpace(parts[0])
		k := strings.TrimSpace(parts[1])
		if b != "" && k != "" {
			return b, k, true
		}
	}

	// Try parsing as URL
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}

	host := u.Host
	if strings.Contains(host, ".s3.") && strings.Contains(host, "amazonaws.com") {
		// host looks like <bucket>.s3.<region>.amazonaws.com
		hostParts := strings.Split(host, ".")
		if len(hostParts) > 0 {
			b := hostParts[0]
			k := strings.TrimPrefix(u.Path, "/")
			if b != "" && k != "" {
				return b, k, true
			}
		}
	}

	// path-style s3.amazonaws.com/<bucket>/<key>
	if strings.HasPrefix(host, "s3.amazonaws.com") || strings.HasPrefix(host, "s3.") {
		p := strings.TrimPrefix(u.Path, "/")
		segs := strings.Split(p, "/")
		if len(segs) >= 2 {
			b := segs[0]
			k := strings.Join(segs[1:], "/")
			if b != "" && k != "" {
				return b, k, true
			}
		}
	}

	return "", "", false
}
