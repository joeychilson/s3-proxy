package origin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

var (
	ErrNotFound     = errors.New("object not found")
	ErrNotModified  = errors.New("object not modified")
	ErrPrecondition = errors.New("precondition failed")
)

type Client struct {
	s3      *s3.Client
	bucket  string
	timeout time.Duration
}

type Conditional struct {
	IfNoneMatch     string
	IfModifiedSince *time.Time
	Range           string
}

type Object struct {
	Body          io.ReadCloser
	Headers       http.Header
	StatusCode    int
	ContentLength int64
	ETag          string
	LastModified  *time.Time
	CacheControl  string
	AcceptRanges  string
	ContentType   string
	ContentRange  string
}

func New(ctx context.Context, endpoint, region, accessKey, secretKey, bucket string, timeout time.Duration) (*Client, error) {
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	awsConfig, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.UsePathStyle = true
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &Client{s3: client, bucket: bucket, timeout: timeout}, nil
}

func (c *Client) GetObject(ctx context.Context, key string, cond *Conditional) (*Object, error) {
	ctx, cancel := c.withTimeout(ctx)
	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	if cond != nil {
		if cond.IfNoneMatch != "" {
			input.IfNoneMatch = aws.String(cond.IfNoneMatch)
		}
		if cond.IfModifiedSince != nil {
			input.IfModifiedSince = cond.IfModifiedSince
		}
		if cond.Range != "" {
			input.Range = aws.String(cond.Range)
		}
	}

	resp, err := c.s3.GetObject(ctx, input)
	if err != nil {
		cancel()
		return nil, translateError(err)
	}

	obj := toObject(resp, http.StatusOK)
	obj.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
	return obj, nil
}

func (c *Client) HeadObject(ctx context.Context, key string, cond *Conditional) (*Object, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	input := &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}
	if cond != nil {
		if cond.IfNoneMatch != "" {
			input.IfNoneMatch = aws.String(cond.IfNoneMatch)
		}
		if cond.IfModifiedSince != nil {
			input.IfModifiedSince = cond.IfModifiedSince
		}
	}

	resp, err := c.s3.HeadObject(ctx, input)
	if err != nil {
		return nil, translateError(err)
	}

	return toHeadObject(resp), nil
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.timeout)
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	return c.ReadCloser.Close()
}

func toObject(resp *s3.GetObjectOutput, status int) *Object {
	headers := http.Header{}
	setHeader(headers, "Content-Type", aws.ToString(resp.ContentType))
	setHeader(headers, "Cache-Control", aws.ToString(resp.CacheControl))
	setHeader(headers, "Last-Modified", formatTime(resp.LastModified))
	setHeader(headers, "ETag", aws.ToString(resp.ETag))
	setHeader(headers, "Content-Encoding", aws.ToString(resp.ContentEncoding))
	setHeader(headers, "Accept-Ranges", aws.ToString(resp.AcceptRanges))
	setHeader(headers, "Content-Range", aws.ToString(resp.ContentRange))
	setHeader(headers, "Content-Disposition", aws.ToString(resp.ContentDisposition))
	setHeader(headers, "Content-Language", aws.ToString(resp.ContentLanguage))

	if exp := aws.ToString(resp.ExpiresString); exp != "" {
		setHeader(headers, "Expires", exp)
	}

	if resp.Metadata != nil {
		for k, v := range resp.Metadata {
			headers.Set("x-amz-meta-"+k, v)
		}
	}

	contentLength := aws.ToInt64(resp.ContentLength)
	if resp.ContentLength != nil {
		headers.Set("Content-Length", strconv.FormatInt(contentLength, 10))
	}

	if cr := aws.ToString(resp.ContentRange); cr != "" {
		status = http.StatusPartialContent
	}

	return &Object{
		Body:          resp.Body,
		Headers:       headers,
		StatusCode:    status,
		ContentLength: contentLength,
		ETag:          aws.ToString(resp.ETag),
		LastModified:  resp.LastModified,
		CacheControl:  aws.ToString(resp.CacheControl),
		AcceptRanges:  aws.ToString(resp.AcceptRanges),
		ContentType:   aws.ToString(resp.ContentType),
		ContentRange:  aws.ToString(resp.ContentRange),
	}
}

func toHeadObject(resp *s3.HeadObjectOutput) *Object {
	headers := http.Header{}
	setHeader(headers, "Content-Type", aws.ToString(resp.ContentType))
	setHeader(headers, "Cache-Control", aws.ToString(resp.CacheControl))
	setHeader(headers, "Last-Modified", formatTime(resp.LastModified))
	setHeader(headers, "ETag", aws.ToString(resp.ETag))
	setHeader(headers, "Content-Encoding", aws.ToString(resp.ContentEncoding))
	setHeader(headers, "Accept-Ranges", aws.ToString(resp.AcceptRanges))
	setHeader(headers, "Content-Disposition", aws.ToString(resp.ContentDisposition))
	setHeader(headers, "Content-Language", aws.ToString(resp.ContentLanguage))

	if exp := aws.ToString(resp.ExpiresString); exp != "" {
		setHeader(headers, "Expires", exp)
	}

	return &Object{
		Headers:       headers,
		StatusCode:    http.StatusOK,
		ContentLength: aws.ToInt64(resp.ContentLength),
		ETag:          aws.ToString(resp.ETag),
		LastModified:  resp.LastModified,
		CacheControl:  aws.ToString(resp.CacheControl),
		AcceptRanges:  aws.ToString(resp.AcceptRanges),
		ContentType:   aws.ToString(resp.ContentType),
	}
}

func setHeader(h http.Header, key string, value string) {
	if value == "" {
		return
	}
	h.Set(key, value)
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(http.TimeFormat)
}

func translateError(err error) error {
	var ae *aws.EndpointNotFoundError
	if errors.As(err, &ae) {
		return fmt.Errorf("endpoint: %w", err)
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket", "404":
			return ErrNotFound
		case "NotModified":
			return ErrNotModified
		case "PreconditionFailed":
			return ErrPrecondition
		default:
			return fmt.Errorf("s3 api: %w", err)
		}
	}

	return fmt.Errorf("s3: %w", err)
}
