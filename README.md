# lfsd - Git LFS Server

A simple Git LFS server implementation in Go.

## Features

- Implements the [Git LFS Batch API](https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md)
- Supports basic transfer adapter for uploads and downloads
- File system based content storage with SHA256 verification
- In-memory metadata storage
- Resume download support with Range headers

## Installation

```bash
go install github.com/wzshiming/lfsd@latest
```

Or build from source:

```bash
git clone https://github.com/wzshiming/lfsd.git
cd lfsd
go build
```

## Usage

### Running the Server

```bash
# Run with default settings (listen on :8080)
./lfsd

# Run with custom settings
./lfsd -listen :9999 -host localhost:9999 -content-path /path/to/storage

# Show version
./lfsd -version
```

### Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-listen` | Address to listen on | `:8080` |
| `-host` | Host used when generating URLs | `localhost:8080` |
| `-scheme` | URL scheme (http or https) | `http` |
| `-content-path` | Path to store LFS objects | `lfs-content` |
| `-version` | Show version | |

### Environment Variables

The following environment variables can override command line options:

| Variable | Description |
|----------|-------------|
| `LFS_LISTEN` | Address to listen on |
| `LFS_HOST` | Host used when generating URLs |
| `LFS_SCHEME` | URL scheme |
| `LFS_CONTENTPATH` | Path to store LFS objects |

## Configuring Git LFS Client

Configure your Git repository to use this server:

```bash
# In your git repository
git config lfs.url http://localhost:8080
```

Or add to `.lfsconfig`:

```ini
[lfs]
    url = http://localhost:8080
```

## API Endpoints

### Batch API

```
POST /objects/batch
Content-Type: application/vnd.git-lfs+json
Accept: application/vnd.git-lfs+json
```

Supports both `download` and `upload` operations.

### Object Download

```
GET /objects/{oid}
```

Supports `Range` header for resume downloads.

### Object Upload

```
PUT /objects/{oid}
Content-Type: application/octet-stream
```

### Verify

```
POST /verify/{oid}
Content-Type: application/vnd.git-lfs+json
```

## Development

### Building

```bash
go build
```

### Testing

```bash
# Run all tests
go test -v ./...

# Run integration tests (requires git and git-lfs installed)
go test -v -run Integration ./...
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## References

- [Git LFS API Documentation](https://github.com/git-lfs/git-lfs/tree/main/docs/api)
- [Git LFS Test Server](https://github.com/git-lfs/lfs-test-server)
