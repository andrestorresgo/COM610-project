package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/chai2010/webp"
)

// ── Config ───────────────────────────────────────────────────────────────────

const (
	// WebP quality: 75 is a good balance of size vs visual fidelity.
	// Range: 0 (worst) – 100 (lossless). Tune per product requirements.
	webpQuality = 75

	// HTTP timeout for webhook calls to the core API.
	webhookTimeout = 10 * time.Second
)

// Resolved at cold-start from environment variables.
var (
	destBucket    string
	webhookURL    string
	webhookSecret string
	awsRegion     string
	s3Client      *s3.Client
	httpClient    = &http.Client{Timeout: webhookTimeout}
)

// ── Webhook payload ──────────────────────────────────────────────────────────

// WebhookPayload is the body POSTed to your core API after a successful conversion.
// The Metadata map carries the S3 user-defined metadata that was set at upload time
// (e.g. "entity-type", "entity-id").
type WebhookPayload struct {
	SourceKey    string            `json:"sourceKey"`
	OptimizedKey string            `json:"optimizedKey"`
	OptimizedURL string            `json:"optimizedUrl"`
	Metadata     map[string]string `json:"metadata"`
}

// ── Entry point ──────────────────────────────────────────────────────────────

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	s3Client = s3.NewFromConfig(cfg)

	destBucket = requireEnv("DEST_BUCKET")
	webhookURL = os.Getenv("WEBHOOK_URL")       // optional; skip webhook if empty
	webhookSecret = os.Getenv("WEBHOOK_SECRET") // optional; skip HMAC if empty

	// Lambda provides AWS_REGION; fallback to AWS_DEFAULT_REGION for local testing.
	awsRegion = os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = os.Getenv("AWS_DEFAULT_REGION")
	}
	if awsRegion == "" {
		awsRegion = "us-east-2"
	}

	lambda.Start(handler)
}

// ── Lambda handler ───────────────────────────────────────────────────────────

func handler(ctx context.Context, event events.S3Event) error {
	for _, record := range event.Records {
		if err := processRecord(ctx, record); err != nil {
			// Log and continue — don't abort the whole batch on a single failure.
			// In production, consider a DLQ on the Lambda event source mapping instead.
			log.Printf("ERROR processing s3://%s/%s: %v",
				record.S3.Bucket.Name, record.S3.Object.Key, err)
		}
	}
	return nil
}

// ── Per-record processing ────────────────────────────────────────────────────

func processRecord(ctx context.Context, record events.S3EventRecord) error {
	srcBucket := record.S3.Bucket.Name
	srcKey := record.S3.Object.Key

	log.Printf("Processing s3://%s/%s", srcBucket, srcKey)

	// 1. Download raw image + its metadata from S3.
	rawBytes, s3Meta, err := downloadObject(ctx, srcBucket, srcKey)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// 2. Decode the image (JPEG / PNG / GIF supported via stdlib decoders).
	//    NOTE: GIF → WebP drops animation; only the first frame is encoded.
	//    If animated GIFs matter to your product, reject them here or handle
	//    separately before this Lambda processes them.
	img, format, err := image.Decode(bytes.NewReader(rawBytes))
	if err != nil {
		return fmt.Errorf("decode (%s): %w", srcKey, err)
	}
	log.Printf("Decoded %s as %s (%dx%d)",
		srcKey, format, img.Bounds().Dx(), img.Bounds().Dy())

	// 3. Encode to WebP.
	webpBytes, err := encodeWebP(img)
	if err != nil {
		return fmt.Errorf("webp encode: %w", err)
	}
	log.Printf("WebP encoded: %d bytes → %d bytes (%.1f%% of original)",
		len(rawBytes), len(webpBytes), 100*float64(len(webpBytes))/float64(len(rawBytes)))

	// 4. Build destination key: replace extension with .webp.
	destKey := replaceExtension(srcKey, ".webp")

	// 5. Upload optimized image to destination bucket.
	optimizedURL, err := uploadObject(ctx, destBucket, destKey, webpBytes, s3Meta)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	log.Printf("Uploaded optimized image to s3://%s/%s", destBucket, destKey)

	// 6. Notify core API via webhook (fire-and-forget is acceptable here;
	//    treat failures as non-fatal so the image is still stored).
	if webhookURL != "" {
		if err := notifyWebhook(srcKey, destKey, optimizedURL, s3Meta); err != nil {
			// Non-fatal: the image was successfully stored. Log and move on.
			// The core API can reconcile via S3 event listing if needed.
			log.Printf("WARN webhook notification failed: %v", err)
		}
	}

	return nil
}

// ── S3 helpers ───────────────────────────────────────────────────────────────

// downloadObject fetches the raw image bytes and its user-defined metadata.
func downloadObject(ctx context.Context, bucket, key string) ([]byte, map[string]string, error) {
	out, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, nil, err
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, nil, err
	}

	// Normalise S3 user metadata keys to lowercase for consistent access.
	// S3 prefixes custom metadata with "x-amz-meta-" in HTTP headers,
	// but the SDK strips that prefix in out.Metadata.
	meta := make(map[string]string, len(out.Metadata))
	for k, v := range out.Metadata {
		meta[strings.ToLower(k)] = v
	}

	return data, meta, nil
}

// uploadObject writes the WebP bytes and carries forward the original S3 metadata.
// Returns the public-style HTTPS URL of the uploaded object.
func uploadObject(ctx context.Context, bucket, key string, data []byte, meta map[string]string) (string, error) {
	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("image/webp"),
		Metadata:    meta,
	})
	if err != nil {
		return "", err
	}

	// Build a standard S3 HTTPS URL with properly escaped key segments.
	// If you use CloudFront or a custom domain, swap this for your CDN base URL + key.
	escapedKey := escapeS3Key(key)
	objectURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, awsRegion, escapedKey)
	return objectURL, nil
}

// ── WebP encoding ────────────────────────────────────────────────────────────

func encodeWebP(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, &webp.Options{Quality: webpQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── Webhook ──────────────────────────────────────────────────────────────────

// notifyWebhook POSTs a JSON payload to the core API.
// The core API should use the metadata map to associate the image with
// the correct restaurant or menu item record.
//
// Expected metadata keys (set these in images.ts when generating the presigned URL):
//   - "entity-type" → "restaurant" | "menu-item"
//   - "entity-id"   → the entity's numeric ID as a string
//
// If WEBHOOK_SECRET is set, the request includes an X-Webhook-Signature header
// containing the HMAC-SHA256 hex digest of the JSON body.
func notifyWebhook(srcKey, destKey, optimizedURL string, metadata map[string]string) error {
	payload := WebhookPayload{
		SourceKey:    srcKey,
		OptimizedKey: destKey,
		OptimizedURL: optimizedURL,
		Metadata:     metadata,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Sign the payload with HMAC-SHA256 if a secret is configured.
	if webhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write(body)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", signature)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("core API returned %d", resp.StatusCode)
	}

	log.Printf("Webhook delivered → %s (%d)", webhookURL, resp.StatusCode)
	return nil
}

// ── Utilities ────────────────────────────────────────────────────────────────

// replaceExtension swaps the file extension on an S3 key.
// Uses path.Ext (POSIX) instead of filepath.Ext so it works correctly
// with S3 keys regardless of the build OS.
// e.g. "uploads/abc-123.png" → "uploads/abc-123.webp"
func replaceExtension(key, newExt string) string {
	ext := path.Ext(key)
	if ext == "" {
		return key + newExt
	}
	return strings.TrimSuffix(key, ext) + newExt
}

// escapeS3Key URL-encodes each segment of an S3 key individually,
// preserving the "/" separators.
func escapeS3Key(key string) string {
	segments := strings.Split(key, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// requireEnv panics at cold-start if a required env var is missing.
// This surfaces misconfiguration immediately rather than failing silently at runtime.
func requireEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		log.Fatalf("required environment variable %q is not set", name)
	}
	return v
}
