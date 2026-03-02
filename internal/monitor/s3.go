package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fabiant7t/eddie/internal/spec"
)

func validateS3Spec(ctx context.Context, parsedSpec spec.Spec) error {
	s3Spec := parsedSpec.S3
	if s3Spec == nil {
		return fmt.Errorf("missing s3 spec")
	}
	if s3Spec.List == nil {
		return fmt.Errorf("missing s3.list")
	}

	listTimeout := s3Spec.List.Timeout
	if listTimeout <= 0 {
		listTimeout = 5 * time.Second
	}
	listCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	client, err := newS3Client(listCtx, s3Spec)
	if err != nil {
		return err
	}

	prefix := expandS3Template(s3Spec.List.Prefix)
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(strings.TrimSpace(s3Spec.List.Bucket)),
		Prefix: aws.String(prefix),
	}
	if s3Spec.List.MaxKeys > 0 {
		input.MaxKeys = aws.Int32(s3Spec.List.MaxKeys)
	}

	output, err := client.ListObjectsV2(listCtx, input)
	if err != nil {
		return fmt.Errorf("list s3 objects: %w", err)
	}
	objectCount := int(aws.ToInt32(output.KeyCount))
	expect := s3Spec.List.Expect
	if expect.CountGT != nil && !(objectCount > *expect.CountGT) {
		return fmt.Errorf("unexpected object count: got %d, want > %d", objectCount, *expect.CountGT)
	}
	if expect.CountGTE != nil && !(objectCount >= *expect.CountGTE) {
		return fmt.Errorf("unexpected object count: got %d, want >= %d", objectCount, *expect.CountGTE)
	}
	if expect.CountEQ != nil && objectCount != *expect.CountEQ {
		return fmt.Errorf("unexpected object count: got %d, want == %d", objectCount, *expect.CountEQ)
	}

	return nil
}

func newS3Client(ctx context.Context, s3Spec *spec.S3Spec) (*s3.Client, error) {
	if s3Spec == nil {
		return nil, fmt.Errorf("s3 config is required")
	}

	region := strings.TrimSpace(s3Spec.Region)
	if region == "" {
		region = "us-east-1"
	}

	loadOptions := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	switch strings.ToLower(strings.TrimSpace(s3Spec.Auth.Mode)) {
	case "", "env", "role":
	case "static":
		loadOptions = append(loadOptions, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			strings.TrimSpace(s3Spec.Auth.AccessKeyID),
			strings.TrimSpace(s3Spec.Auth.SecretAccessKey),
			strings.TrimSpace(s3Spec.Auth.SessionToken),
		)))
	default:
		return nil, fmt.Errorf("unsupported s3.auth.mode %q", s3Spec.Auth.Mode)
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	clientOptions := []func(*s3.Options){}
	if endpoint := strings.TrimSpace(s3Spec.Endpoint); endpoint != "" {
		clientOptions = append(clientOptions, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	if s3Spec.PathStyle {
		clientOptions = append(clientOptions, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return s3.NewFromConfig(awsConfig, clientOptions...), nil
}

func expandS3Template(raw string) string {
	replaced := raw
	utcNow := time.Now().UTC()
	replaced = strings.ReplaceAll(replaced, "{utc_hour_minus_1}", utcNow.Add(-1*time.Hour).Format("2006-01-02-15"))
	replaced = strings.ReplaceAll(replaced, "{utc_hour}", utcNow.Format("2006-01-02-15"))
	return replaced
}
