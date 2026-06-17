package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

const checksumMetadataKey = "checksum-sha256"

type S3Config struct {
	Region          string
	EndpointURL     string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	ForcePathStyle  bool
}

type S3Storage struct {
	client *s3.Client
	log    *zap.Logger
}

func NewS3Storage(ctx context.Context, cfg S3Config, log *zap.Logger) (*S3Storage, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" || cfg.SessionToken != "" {
		provider := credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(provider))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.EndpointURL != "" {
			options.BaseEndpoint = aws.String(cfg.EndpointURL)
		}
		options.UsePathStyle = cfg.ForcePathStyle
	})

	return &S3Storage{client: client, log: loggerOrNop(log)}, nil
}

func (storage *S3Storage) PutObject(ctx context.Context, object capturemodel.Object) error {
	started := time.Now()
	storage.log.Info(
		"s3 put object started",
		zap.String("bucket", object.Bucket),
		zap.String("object_key", object.ObjectKey),
		zap.Int("size_bytes", len(object.Body)),
	)

	_, err := storage.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(object.Bucket),
		Key:         aws.String(object.ObjectKey),
		Body:        bytes.NewReader(object.Body),
		ContentType: aws.String(object.ContentType),
		Metadata: map[string]string{
			checksumMetadataKey: object.ChecksumSHA256,
		},
	})
	if err == nil {
		storage.log.Info(
			"s3 put object completed",
			zap.String("bucket", object.Bucket),
			zap.String("object_key", object.ObjectKey),
			zap.Duration("duration", time.Since(started)),
		)
		return nil
	}

	storage.log.Warn(
		"s3 put object failed; checking head fallback",
		zap.String("bucket", object.Bucket),
		zap.String("object_key", object.ObjectKey),
		zap.Error(err),
	)
	if storage.headChecksumMatches(ctx, object) {
		storage.log.Info(
			"s3 put object recovered by head checksum",
			zap.String("bucket", object.Bucket),
			zap.String("object_key", object.ObjectKey),
			zap.Duration("duration", time.Since(started)),
		)
		return nil
	}

	storage.log.Error(
		"s3 put object failed",
		zap.String("bucket", object.Bucket),
		zap.String("object_key", object.ObjectKey),
		zap.Error(err),
		zap.Duration("duration", time.Since(started)),
	)

	return fmt.Errorf("put s3 object %s/%s: %w", object.Bucket, object.ObjectKey, err)
}

func (storage *S3Storage) GetObject(
	ctx context.Context,
	bucket string,
	objectKey string,
	maxBytes int64,
) (capturemodel.Object, error) {
	started := time.Now()
	storage.log.Info("s3 get object started", zap.String("bucket", bucket), zap.String("object_key", objectKey))

	response, err := storage.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		storage.log.Error(
			"s3 get object failed",
			zap.String("bucket", bucket),
			zap.String("object_key", objectKey),
			zap.Error(err),
			zap.Duration("duration", time.Since(started)),
		)
		return capturemodel.Object{}, fmt.Errorf("get s3 object %s/%s: %w", bucket, objectKey, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := readObjectBody(response.Body, maxBytes)
	if err != nil {
		storage.log.Error(
			"s3 read object failed",
			zap.String("bucket", bucket),
			zap.String("object_key", objectKey),
			zap.Error(err),
			zap.Duration("duration", time.Since(started)),
		)
		return capturemodel.Object{}, err
	}

	storage.log.Info(
		"s3 get object completed",
		zap.String("bucket", bucket),
		zap.String("object_key", objectKey),
		zap.Int("size_bytes", len(body)),
		zap.Duration("duration", time.Since(started)),
	)

	return capturemodel.Object{
		Bucket:         bucket,
		ObjectKey:      objectKey,
		ContentType:    aws.ToString(response.ContentType),
		ChecksumSHA256: response.Metadata[checksumMetadataKey],
		Body:           body,
	}, nil
}

func (storage *S3Storage) DeleteObject(ctx context.Context, bucket, objectKey string) error {
	started := time.Now()
	storage.log.Info("s3 delete object started", zap.String("bucket", bucket), zap.String("object_key", objectKey))

	_, err := storage.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		storage.log.Error(
			"s3 delete object failed",
			zap.String("bucket", bucket),
			zap.String("object_key", objectKey),
			zap.Error(err),
			zap.Duration("duration", time.Since(started)),
		)
		return fmt.Errorf("delete s3 object %s/%s: %w", bucket, objectKey, err)
	}

	storage.log.Info(
		"s3 delete object completed",
		zap.String("bucket", bucket),
		zap.String("object_key", objectKey),
		zap.Duration("duration", time.Since(started)),
	)

	return nil
}

func readObjectBody(body io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("max object size must be positive")
	}

	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read s3 object body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("s3 object exceeds maximum size")
	}

	return data, nil
}

func (storage *S3Storage) headChecksumMatches(ctx context.Context, object capturemodel.Object) bool {
	response, err := storage.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(object.Bucket),
		Key:    aws.String(object.ObjectKey),
	})
	if err != nil {
		return false
	}

	for key, value := range response.Metadata {
		if strings.EqualFold(key, checksumMetadataKey) && value == object.ChecksumSHA256 {
			return true
		}
	}

	return false
}

func loggerOrNop(log *zap.Logger) *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}

	return log
}
