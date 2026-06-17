package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const ensureTimeout = 30 * time.Second

type s3Config struct {
	Bucket          string
	Region          string
	EndpointURL     string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	ForcePathStyle  bool
	PublicRead      bool
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("ensure hoppify s3 bucket: %v", err)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ensureTimeout)
	defer cancel()

	client, err := newClient(ctx, &cfg)
	if err != nil {
		return err
	}

	return ensureBucket(ctx, client, &cfg)
}

func loadConfig() (s3Config, error) {
	forcePathStyle, err := strconv.ParseBool(envDefault("HOPPIFY_S3_FORCE_PATH_STYLE", "false"))
	if err != nil {
		return s3Config{}, fmt.Errorf("parse HOPPIFY_S3_FORCE_PATH_STYLE: %w", err)
	}
	publicRead, err := strconv.ParseBool(envDefault("HOPPIFY_S3_BUCKET_PUBLIC_READ", "true"))
	if err != nil {
		return s3Config{}, fmt.Errorf("parse HOPPIFY_S3_BUCKET_PUBLIC_READ: %w", err)
	}

	cfg := s3Config{
		Bucket:          os.Getenv("HOPPIFY_S3_BUCKET"),
		Region:          os.Getenv("HOPPIFY_S3_REGION"),
		EndpointURL:     os.Getenv("HOPPIFY_S3_ENDPOINT_URL"),
		AccessKeyID:     os.Getenv("HOPPIFY_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("HOPPIFY_S3_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("HOPPIFY_S3_SESSION_TOKEN"),
		ForcePathStyle:  forcePathStyle,
		PublicRead:      publicRead,
	}
	if cfg.Bucket == "" {
		return s3Config{}, errors.New("HOPPIFY_S3_BUCKET is required")
	}
	if cfg.Region == "" {
		return s3Config{}, errors.New("HOPPIFY_S3_REGION is required")
	}
	if cfg.AccessKeyID == "" {
		return s3Config{}, errors.New("HOPPIFY_S3_ACCESS_KEY_ID is required")
	}
	if cfg.SecretAccessKey == "" {
		return s3Config{}, errors.New("HOPPIFY_S3_SECRET_ACCESS_KEY is required")
	}

	return cfg, nil
}

func newClient(ctx context.Context, cfg *s3Config) (*s3.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.EndpointURL != "" {
			options.BaseEndpoint = aws.String(cfg.EndpointURL)
		}
		options.UsePathStyle = cfg.ForcePathStyle
	})

	return client, nil
}

func ensureBucket(ctx context.Context, client *s3.Client, cfg *s3Config) error {
	bucket := cfg.Bucket
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		fmt.Printf("hoppify s3 bucket exists: %s\n", bucket)
		return ensureBucketVisibility(ctx, client, cfg)
	}
	if !isNotFound(err) {
		return fmt.Errorf("head bucket %q: %w", bucket, err)
	}

	acl := types.BucketCannedACLPrivate
	if cfg.PublicRead {
		acl = types.BucketCannedACLPublicRead
	}
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
		ACL:    acl,
	})
	if err != nil && !isAlreadyOwned(err) {
		return fmt.Errorf("create bucket %q: %w", bucket, err)
	}

	fmt.Printf("hoppify s3 bucket ready: %s\n", bucket)
	return ensureBucketVisibility(ctx, client, cfg)
}

func ensureBucketVisibility(ctx context.Context, client *s3.Client, cfg *s3Config) error {
	if !cfg.PublicRead {
		return nil
	}

	policy, err := publicReadPolicy(cfg.Bucket)
	if err != nil {
		return err
	}
	_, err = client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(cfg.Bucket),
		Policy: aws.String(policy),
	})
	if err != nil {
		return fmt.Errorf("set public read bucket policy %q: %w", cfg.Bucket, err)
	}

	fmt.Printf("hoppify s3 bucket public read enabled: %s\n", cfg.Bucket)
	return nil
}

func publicReadPolicy(bucket string) (string, error) {
	policy := struct {
		Version   string            `json:"Version"`
		Statement []policyStatement `json:"Statement"`
	}{
		Version: "2012-10-17",
		Statement: []policyStatement{{
			Sid:       "PublicReadGetObject",
			Effect:    "Allow",
			Principal: "*",
			Action:    []string{"s3:GetObject"},
			Resource:  []string{fmt.Sprintf("arn:aws:s3:::%s/*", bucket)},
		}},
	}

	data, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("marshal public read bucket policy: %w", err)
	}

	return string(data), nil
}

type policyStatement struct {
	Sid       string   `json:"Sid"`
	Effect    string   `json:"Effect"`
	Principal string   `json:"Principal"`
	Action    []string `json:"Action"`
	Resource  []string `json:"Resource"`
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.ErrorCode() {
	case "404", "NotFound", "NoSuchBucket":
		return true
	default:
		return false
	}
}

func isAlreadyOwned(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.ErrorCode() {
	case "BucketAlreadyOwnedByYou":
		return true
	default:
		return false
	}
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
