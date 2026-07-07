package backup

import (
	"bytes"
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// uploadS3 stores data under name in an S3-compatible bucket. cfg keys:
// endpoint, bucket, accessKey, secretKey, region, secure ("true"/"false").
func uploadS3(ctx context.Context, cfg map[string]string, name string, data []byte) (string, error) {
	endpoint := cfg["endpoint"]
	bucket := cfg["bucket"]
	if endpoint == "" || bucket == "" {
		return "", fmt.Errorf("s3 destination needs endpoint and bucket")
	}
	secure := cfg["secure"] != "false"
	cl, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg["accessKey"], cfg["secretKey"], ""),
		Secure: secure,
		Region: cfg["region"],
	})
	if err != nil {
		return "", err
	}
	// Create the bucket on first use.
	if exists, _ := cl.BucketExists(ctx, bucket); !exists {
		if err := cl.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: cfg["region"]}); err != nil {
			return "", fmt.Errorf("s3 make bucket: %w", err)
		}
	}
	key := "kapibara-backups/" + name
	_, err = cl.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return "", fmt.Errorf("s3 upload: %w", err)
	}
	return fmt.Sprintf("s3://%s/%s", bucket, key), nil
}
