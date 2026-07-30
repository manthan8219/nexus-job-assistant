package greenhouse

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// uploadAttachment replicates the browser's resume/cover-letter upload:
//
//  1. GET {boardsBaseURL}/uncacheable_attributes/presigned_fields?fields[]=<field>
//     → S3 presigned POST policy fields + an object key template.
//  2. POST the file to the S3 bucket URL (multipart form, exactly as the
//     Greenhouse front-end does it: utf8 flag, policy fields, key with
//     {timestamp}/{unique_id} substituted, a dummy authenticity_token, a
//     blanket Content-Type, then the file).
//  3. S3 answers 201 + a <PostResponse> XML whose <Location> is the file URL
//     the application JSON later references as resume_url / cover_letter_url.
//
// It returns the public file URL and the base file name to send as
// resume_url / resume_url_filename.
func uploadAttachment(ctx context.Context, client *http.Client, fieldName, filePath string) (fileURL, fileName string, err error) {
	s3URL, target, err := fetchPresignedFields(ctx, client, fieldName)
	if err != nil {
		return "", "", err
	}

	key := strings.NewReplacer(
		"{timestamp}", strconv.FormatInt(time.Now().UnixMilli(), 10),
		"{unique_id}", randomID(14),
	).Replace(target.Key)

	f, err := os.Open(filePath)
	if err != nil {
		return "", "", fmt.Errorf("open %q: %w", filePath, err)
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("utf8", "✓"); err != nil {
		return "", "", err
	}
	// Stable field order doesn't matter to S3, but policy fields must all be present.
	for k, v := range target.Fields {
		if err := w.WriteField(k, v); err != nil {
			return "", "", err
		}
	}
	for k, v := range map[string]string{
		"key":                key,
		"authenticity_token": "1234", // ignored by S3; the front-end sends it
		"Content-Type":       "application/octet-stream",
	} {
		if err := w.WriteField(k, v); err != nil {
			return "", "", err
		}
	}
	part, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", "", err
	}
	if err := w.Close(); err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s3URL, &body)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", rendererUA)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("s3 upload: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("s3 upload: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	// Prefer the canonical <Location> from the S3 PostResponse; fall back to
	// bucket URL + key (identical in practice).
	var pr s3PostResponse
	if err := xml.Unmarshal(respBody, &pr); err == nil && pr.Location != "" {
		return pr.Location, filepath.Base(filePath), nil
	}
	return strings.TrimSuffix(s3URL, "/") + "/" + key, filepath.Base(filePath), nil
}

type s3PostResponse struct {
	XMLName  xml.Name `xml:"PostResponse"`
	Location string   `xml:"Location"`
	Key      string   `xml:"Key"`
}

// fetchPresignedFields retrieves the S3 presigned POST data for one form
// field ("resume", "cover_letter", or a custom attachment field name).
func fetchPresignedFields(ctx context.Context, client *http.Client, fieldName string) (s3URL string, target *presignedTarget, err error) {
	u := fmt.Sprintf("%s/uncacheable_attributes/presigned_fields?fields%%5B%%5D=%s",
		boardsBaseURL, url.QueryEscape(fieldName))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", rendererUA)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("presigned fields %q: HTTP %d", fieldName, resp.StatusCode)
	}

	// Response shape: {"url": "<s3 bucket url>", "<fieldName>": {"fields": {...}, "key": "..."}}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", nil, fmt.Errorf("presigned fields %q: decode: %w", fieldName, err)
	}
	if err := json.Unmarshal(raw["url"], &s3URL); err != nil || s3URL == "" {
		return "", nil, fmt.Errorf("presigned fields %q: missing bucket url", fieldName)
	}
	var t presignedTarget
	if err := json.Unmarshal(raw[fieldName], &t); err != nil || t.Key == "" || len(t.Fields) == 0 {
		return "", nil, fmt.Errorf("presigned fields %q: missing policy data", fieldName)
	}
	return s3URL, &t, nil
}

// randomID returns n lowercase hex chars — the front-end uses a base-36
// Math.random string; any unique suffix satisfies the S3 key policy.
func randomID(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)[:n]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
