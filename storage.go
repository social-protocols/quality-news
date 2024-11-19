package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"os"
)

type StorageClient struct {
	minioClient *minio.Client
	bucket      string
}

func NewStorageClient() (*StorageClient, error) {
	endpoint := os.Getenv("R2_ENDPOINT")
	if endpoint == "" {
		return nil, fmt.Errorf("R2_ENDPOINT environment variable not set")
	}

	accessKeyID := os.Getenv("R2_ACCESS_KEY_ID")
	if accessKeyID == "" {
		return nil, fmt.Errorf("R2_ACCESS_KEY_ID environment variable not set")
	}

	secretAccessKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	if secretAccessKey == "" {
		return nil, fmt.Errorf("R2_SECRET_ACCESS_KEY environment variable not set")
	}

	useSSL := os.Getenv("R2_USE_SSL")
	if useSSL == "" {
		return nil, fmt.Errorf("R2_USE_SSL environment variable not set")
	}

	bucket := os.Getenv("R2_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("R2_BUCKET environment variable not set")
	}

	// Convert useSSL to boolean
	var ssl bool
	if useSSL == "true" || useSSL == "1" {
		ssl = true
	} else {
		ssl = false
	}

	// Remove "https://" or "http://" prefix from endpoint if present
	endpoint = trimEndpointScheme(endpoint)

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: ssl,
		Region: "auto", // For Cloudflare R2
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}

	return &StorageClient{minioClient: minioClient, bucket: bucket}, nil
}

// UploadFile uploads a file compressed with gzip and sets the correct headers
func (sc *StorageClient) UploadFile(ctx context.Context, objectName string, content []byte) error {
	// Compress the content using gzip
	var compressedContent bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedContent)
	_, err := gzipWriter.Write(content)
	if err != nil {
		return fmt.Errorf("failed to compress content: %v", err)
	}
	gzipWriter.Close() // Make sure to close the writer to flush the data

	// Upload the compressed content with the appropriate headers
	_, err = sc.minioClient.PutObject(ctx, sc.bucket, objectName,
		&compressedContent, int64(compressedContent.Len()), minio.PutObjectOptions{
			ContentType:     "text/html", // Original content type
			ContentEncoding: "gzip",      // Inform the browser that the content is gzipped
		})
	if err != nil {
		return fmt.Errorf("failed to upload object %s: %v", objectName, err)
	}

	return nil
}

func (sc *StorageClient) FileExists(ctx context.Context, objectName string) (bool, error) {
	// Attempt to get object information
	_, err := sc.minioClient.StatObject(ctx, sc.bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		// If the error is because the object does not exist, return false
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		// Otherwise, return the error
		return false, fmt.Errorf("error checking if object %s exists: %v", objectName, err)
	}
	// If no error, the object exists
	return true, nil
}

// Helper function to trim scheme from endpoint
func trimEndpointScheme(endpoint string) string {
	if len(endpoint) >= 8 && endpoint[:8] == "https://" {
		return endpoint[8:]
	}
	if len(endpoint) >= 7 && endpoint[:7] == "http://" {
		return endpoint[7:]
	}
	return endpoint
}
