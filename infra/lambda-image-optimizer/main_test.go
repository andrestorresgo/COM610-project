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
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// createTestImage creates a simple 10x10 red image.
func createTestImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	return img
}

func TestImageDecodingAndEncoding(t *testing.T) {
	srcImg := createTestImage()

	tests := []struct {
		name   string
		encode func(img image.Image) ([]byte, error)
	}{
		{
			name: "PNG",
			encode: func(img image.Image) ([]byte, error) {
				var buf bytes.Buffer
				err := png.Encode(&buf, img)
				return buf.Bytes(), err
			},
		},
		{
			name: "JPEG",
			encode: func(img image.Image) ([]byte, error) {
				var buf bytes.Buffer
				err := jpeg.Encode(&buf, img, nil)
				return buf.Bytes(), err
			},
		},
		{
			name: "GIF",
			encode: func(img image.Image) ([]byte, error) {
				var buf bytes.Buffer
				err := gif.Encode(&buf, img, nil)
				return buf.Bytes(), err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Encode image to the test format bytes
			imgBytes, err := tc.encode(srcImg)
			if err != nil {
				t.Fatalf("failed to encode: %v", err)
			}

			// 2. Decode using standard library image.Decode
			decodedImg, format, err := image.Decode(bytes.NewReader(imgBytes))
			if err != nil {
				t.Fatalf("failed to decode format %s: %v", tc.name, err)
			}

			if format != tc.name && !(tc.name == "JPEG" && format == "jpeg") && !(tc.name == "PNG" && format == "png") && !(tc.name == "GIF" && format == "gif") {
				t.Errorf("expected format %s, got %s", tc.name, format)
			}

			// 3. Test WebP encoding
			webpBytes, err := encodeWebP(decodedImg)
			if err != nil {
				t.Fatalf("failed to encode to WebP: %v", err)
			}

			if len(webpBytes) == 0 {
				t.Error("encoded WebP bytes are empty")
			}
		})
	}
}

func TestReplaceExtension(t *testing.T) {
	tests := []struct {
		key      string
		newExt   string
		expected string
	}{
		{"uploads/image.png", ".webp", "uploads/image.webp"},
		{"uploads/image.jpeg", ".webp", "uploads/image.webp"},
		{"uploads/image.jpg", ".webp", "uploads/image.webp"},
		{"uploads/image.gif", ".webp", "uploads/image.webp"},
		{"uploads/no-extension", ".webp", "uploads/no-extension.webp"},
		{"nested/path/to/image.png", ".webp", "nested/path/to/image.webp"},
	}

	for _, tc := range tests {
		got := replaceExtension(tc.key, tc.newExt)
		if got != tc.expected {
			t.Errorf("replaceExtension(%q, %q) = %q; expected %q", tc.key, tc.newExt, got, tc.expected)
		}
	}
}

func TestEscapeS3Key(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"uploads/image.png", "uploads/image.png"},
		{"uploads/hello world.png", "uploads/hello%20world.png"},
		{"uploads/a/b/c.png", "uploads/a/b/c.png"},
		{"uploads/special@char.png", "uploads/special@char.png"},
	}

	for _, tc := range tests {
		got := escapeS3Key(tc.key)
		if got != tc.expected {
			t.Errorf("escapeS3Key(%q) = %q; expected %q", tc.key, got, tc.expected)
		}
	}
}

func TestTryDecodeBase64(t *testing.T) {
	// A simple valid PNG (1x1 transparent pixel) encoded as base64
	pngBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	pngDataURI := "data:image/png;base64," + pngBase64

	// Test data URI decoding
	decoded, err := tryDecodeBase64([]byte(pngDataURI))
	if err != nil {
		t.Fatalf("failed to decode data URI: %v", err)
	}
	if _, _, err := image.Decode(bytes.NewReader(decoded)); err != nil {
		t.Errorf("decoded data URI is not a valid image: %v", err)
	}

	// Test raw base64 decoding
	decoded2, err := tryDecodeBase64([]byte(pngBase64))
	if err != nil {
		t.Fatalf("failed to decode raw base64: %v", err)
	}
	if _, _, err := image.Decode(bytes.NewReader(decoded2)); err != nil {
		t.Errorf("decoded raw base64 is not a valid image: %v", err)
	}

	// Test invalid base64
	_, err = tryDecodeBase64([]byte("this is not base64!"))
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}

func TestTryExtractMultipart(t *testing.T) {
	// Generate valid PNG bytes
	srcImg := createTestImage()
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, srcImg); err != nil {
		t.Fatalf("failed to encode PNG for test: %v", err)
	}
	pngBytes := pngBuf.Bytes()

	// Construct a multipart form-data body
	boundary := "----WebKitFormBoundaryTest123"
	var body bytes.Buffer
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"test.png\"\r\n")
	body.WriteString("Content-Type: image/png\r\n\r\n")
	body.Write(pngBytes)
	body.WriteString("\r\n--" + boundary + "--\r\n")

	extracted, err := tryExtractMultipart(body.Bytes())
	if err != nil {
		t.Fatalf("failed to extract multipart: %v", err)
	}

	if !bytes.Equal(extracted, pngBytes) {
		t.Error("extracted bytes do not match original PNG bytes")
	}

	// Test non-multipart input
	_, err = tryExtractMultipart([]byte("just some plain bytes"))
	if err == nil {
		t.Error("expected error for non-multipart input, got nil")
	}
}

type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestProcessRecordWithMockS3(t *testing.T) {
	// 1. Generate valid test image
	srcImg := createTestImage()
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, srcImg); err != nil {
		t.Fatalf("failed to encode PNG: %v", err)
	}
	pngBytes := pngBuf.Bytes()

	// 2. Set up global config overrides for local offline test
	destBucket = "test-dest-bucket"
	webhookURL = "http://test-webhook-api.localhost/images/webhook"
	webhookSecret = "testsecret123"
	awsRegion = "us-east-2"

	// Keep track of S3 operations
	var getObjectCalled bool
	var putObjectCalled bool
	var putObjectBody []byte
	var putObjectContentType string
	var putObjectMetadata map[string]string

	// Mock S3 client using custom HTTP Client in Options
	mockS3Client := s3.New(s3.Options{
		Region: awsRegion,
		HTTPClient: &mockRoundTripper{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.Method == "GET" && strings.Contains(req.URL.Path, "/uploads/test-image.png") {
					getObjectCalled = true
					resp := &http.Response{
						StatusCode: 200,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(pngBytes)),
					}
					// Return S3 metadata headers (SDK strips "x-amz-meta-" prefix and converts to lowercase in main.go)
					resp.Header.Set("x-amz-meta-entity-type", "restaurant")
					resp.Header.Set("x-amz-meta-entity-id", "123")
					return resp, nil
				}

				if req.Method == "PUT" && strings.Contains(req.URL.Path, "/uploads/test-image.webp") {
					putObjectCalled = true
					body, err := io.ReadAll(req.Body)
					if err != nil {
						return nil, err
					}
					putObjectBody = body
					putObjectContentType = req.Header.Get("Content-Type")

					// Parse metadata from request headers
					putObjectMetadata = make(map[string]string)
					for k, v := range req.Header {
						if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
							key := strings.TrimPrefix(strings.ToLower(k), "x-amz-meta-")
							putObjectMetadata[key] = v[0]
						}
					}

					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader("")),
					}, nil
				}

				return nil, fmt.Errorf("unexpected S3 HTTP request: %s %s", req.Method, req.URL.String())
			},
		},
	})

	// Inject our mock S3 client
	s3Client = mockS3Client

	// 3. Mock Webhook Client
	var webhookCalled bool
	var webhookPayload WebhookPayload
	var webhookSignature string

	httpClient = &http.Client{
		Transport: &mockRoundTripper{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.Method == "POST" && req.URL.String() == webhookURL {
					webhookCalled = true
					body, err := io.ReadAll(req.Body)
					if err != nil {
						return nil, err
					}
					webhookSignature = req.Header.Get("X-Webhook-Signature")

					if err := json.Unmarshal(body, &webhookPayload); err != nil {
						return nil, err
					}

					// Verify signature
					mac := hmac.New(sha256.New, []byte(webhookSecret))
					mac.Write(body)
					expectedSig := hex.EncodeToString(mac.Sum(nil))
					if webhookSignature != expectedSig {
						t.Errorf("webhook signature mismatch: got %s, expected %s", webhookSignature, expectedSig)
					}

					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
					}, nil
				}
				return nil, fmt.Errorf("unexpected Webhook HTTP request: %s %s", req.Method, req.URL.String())
			},
		},
	}

	// 4. Run processRecord
	record := events.S3EventRecord{
		S3: events.S3Entity{
			Bucket: events.S3Bucket{
				Name: "test-src-bucket",
			},
			Object: events.S3Object{
				Key: "uploads/test-image.png",
			},
		},
	}

	err := processRecord(context.Background(), record)
	if err != nil {
		t.Fatalf("processRecord failed: %v", err)
	}

	// 5. Assertions
	if !getObjectCalled {
		t.Error("expected GetObject to be called, but it was not")
	}
	if !putObjectCalled {
		t.Error("expected PutObject to be called, but it was not")
	}
	if putObjectContentType != "image/webp" {
		t.Errorf("expected PUT Content-Type to be image/webp, got %s", putObjectContentType)
	}
	if putObjectMetadata["entity-type"] != "restaurant" || putObjectMetadata["entity-id"] != "123" {
		t.Errorf("expected metadata keys entity-type and entity-id in PutObject, got %v", putObjectMetadata)
	}

	// Verify WebP encoding succeeded
	if len(putObjectBody) == 0 {
		t.Error("uploaded WebP body is empty")
	}
	// Try decoding the uploaded WebP to verify validity
	if len(putObjectBody) < 12 || string(putObjectBody[8:12]) != "WEBP" {
		t.Errorf("uploaded body is not a valid WebP file: magic header %q", string(putObjectBody[8:12]))
	}

	if !webhookCalled {
		t.Error("expected webhook notification, but was not called")
	}
	if webhookPayload.SourceKey != "uploads/test-image.png" {
		t.Errorf("expected sourceKey in webhook payload to be uploads/test-image.png, got %s", webhookPayload.SourceKey)
	}
	if webhookPayload.OptimizedKey != "uploads/test-image.webp" {
		t.Errorf("expected optimizedKey to be uploads/test-image.webp, got %s", webhookPayload.OptimizedKey)
	}
	if webhookPayload.Metadata["entity-type"] != "restaurant" || webhookPayload.Metadata["entity-id"] != "123" {
		t.Errorf("expected metadata in webhook payload to contain entity info, got %v", webhookPayload.Metadata)
	}
}

