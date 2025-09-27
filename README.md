# S3 Proxy

A high-performance, simple S3 proxy with intelligent caching. Built for serving static assets from S3 with CDN-like performance and Railway deployment in mind.

### Railway Deployment

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/deploy/s3-proxy?referralCode=NhCCIt&utm_medium=integration&utm_source=template&utm_campaign=generic)

## Features

- **Smart Caching**: LRU cache with TTL and stale-while-revalidate
- **HTTP Compliance**: Full support for ETags, Last-Modified, conditional requests
- **High Performance**: Async revalidation, connection pooling, streaming responses
- **Production Ready**: Metrics, health checks, rate limiting, graceful shutdown
- **Simple**: Single binary, environment-based configuration

## Quick Start

### Environment Variables

**Required:**

```bash
AUTH_TOKEN=your-admin-token
S3_ENDPOINT=https://s3.amazonaws.com
S3_BUCKET=your-bucket-name
S3_ACCESS_KEY=your-access-key
S3_SECRET_KEY=your-secret-key
```

**Optional:**

```bash
SERVER_ADDR=:8080
S3_REGION=auto
CACHE_CAPACITY=2048
CACHE_TTL=5m
CACHE_STALE_TTL=2m
MAX_OBJECT_SIZE=16777216
REQUEST_TIMEOUT=15s
READ_TIMEOUT=5s
WRITE_TIMEOUT=15s
IDLE_TIMEOUT=60s
RATE_LIMIT_RPS=0
```

### Build & Run

```bash
go build -o s3-proxy ./cmd/server
./s3-proxy
```

## API Endpoints

### Content Serving

```bash
GET  /path/to/file.jpg    # Serve file from S3
HEAD /path/to/file.jpg    # Get file metadata
```

### Admin (requires AUTH_TOKEN)

```bash
GET  /metrics             # Prometheus metrics
POST /cache/purge         # Purge cache entries
GET  /healthz             # Health check (public)
```

### Authentication

Admin endpoints require authentication via:

**Header:**

```bash
curl -H "X-Auth-Token: your-token" /metrics
```

**Bearer Token:**

```bash
curl -H "Authorization: Bearer your-token" /metrics
```

**Query Parameter:**

```bash
curl /metrics?token=your-token
```

## Cache Purging

```bash
curl -X POST \
  -H "X-Auth-Token: your-token" \
  -H "Content-Type: application/json" \
  -d '{"keys": ["file1.jpg", "path/to/file2.png"]}' \
  https://your-app.railway.app/cache/purge
```

## Configuration

### Cache Settings

- **CACHE_CAPACITY**: Maximum number of cached objects (default: 2048)
- **CACHE_TTL**: How long objects stay fresh (default: 5m)
- **CACHE_STALE_TTL**: How long stale objects can be served (default: 2m)
- **MAX_OBJECT_SIZE**: Maximum size of cacheable objects (default: 16MB)

### Performance Tuning

**For high-traffic:**

```bash
CACHE_CAPACITY=4096
CACHE_TTL=1h
CACHE_STALE_TTL=10m
MAX_OBJECT_SIZE=33554432  # 32MB
REQUEST_TIMEOUT=30s
RATE_LIMIT_RPS=100
```

**For development:**

```bash
CACHE_CAPACITY=512
CACHE_TTL=1m
CACHE_STALE_TTL=30s
RATE_LIMIT_RPS=0  # disabled
```

## Monitoring

### Health Check

```bash
curl https://your-app.railway.app/healthz
# Returns: 200 OK "ok"
```

### Metrics (Prometheus)

```bash
curl -H "X-Auth-Token: your-token" https://your-app.railway.app/metrics
```

**Key Metrics:**

- `proxy_cache_hits_total` - Cache hit count
- `proxy_cache_misses_total` - Cache miss count
- `proxy_cache_stale_total` - Stale cache serves
- `proxy_origin_errors_total` - S3 errors
- `proxy_origin_latency_seconds` - S3 response time
- `proxy_bytes_served_total` - Bandwidth served

## Architecture

```
Client → S3 Proxy → S3 Bucket
         ↓
      LRU Cache
```

**Cache Flow:**

1. **Cache Hit**: Serve from memory (<1ms)
2. **Cache Miss**: Fetch from S3, cache, serve
3. **Stale Hit**: Serve stale, async revalidate
4. **Conditional**: Use ETags/Last-Modified to minimize S3 bandwidth

## HTTP Features

- **Range Requests**: Partial content support
- **Conditional Requests**: If-None-Match, If-Modified-Since
- **Proper Headers**: Cache-Control, ETag, Last-Modified, Age
- **Status Codes**: 200, 206, 304, 404, 412, etc.
- **Compression**: Transparent (S3 handles gzip if configured)

## Deployment Tips

### Railway

- Set all required environment variables
- Use Railway's metrics integration with `/metrics`
- Monitor logs for cache hit ratios

### Resource Planning

```
Memory = (CACHE_CAPACITY × Average File Size) + ~100MB overhead
```

Example: 2048 capacity × 1MB average = ~2GB RAM recommended

### S3 Configuration

- Enable Transfer Acceleration for better global performance
- Set appropriate Cache-Control headers on S3 objects
- Consider CloudFront if you need global edge locations

## Development

```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Build
go build ./cmd/server

# Run with debug logging
LOG_LEVEL=debug ./s3-proxy
```

## License

MIT
