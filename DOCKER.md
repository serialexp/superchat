# Docker Deployment Guide

SuperChat provides two Docker images:
- **Server**: `aeolun/superchat:latest` - The chat server
- **Website**: `aeolun/superchat-website:latest` - The marketing website

## Quick Start

### Using Docker Run

```bash
# Run the server
docker run -d \
  --name superchat \
  -p 6465:6465 \
  -v superchat-data:/data \
  aeolun/superchat:latest

# View logs
docker logs -f superchat

# Stop the server
docker stop superchat
```

### Using Docker Compose

The `docker-compose.yml` runs both server and website:

```bash
docker-compose up -d
```

Services:
- **superchat** - Chat server on port 6465
- **website** - Marketing site on port 8080

**Note:** SuperChat uses a custom binary TCP protocol (not HTTP), so port 6465 must be exposed directly. HTTP reverse proxies like Caddy/Nginx won't work without their TCP streaming modules.

Clients connect to: `superchat.win:6465` or `your-server-ip:6465`
Website: http://localhost:8080

## Configuration

The server auto-creates a default config at `/data/config.toml` on first run.

### Option 1: Environment Variables (Recommended for Docker)

Configuration can be overridden using environment variables:

```bash
docker run -d \
  --name superchat \
  -p 6465:6465 \
  -e SUPERCHAT_SERVER_TCP_PORT=8080 \
  -e SUPERCHAT_LIMITS_MAX_MESSAGE_LENGTH=8192 \
  -v superchat-data:/data \
  aeolun/superchat:latest
```

**Available environment variables:**

**Server Section:**
- `SUPERCHAT_SERVER_TCP_PORT` - Main TCP port (default: 6465)
- `SUPERCHAT_SERVER_SSH_PORT` - SSH port (default: 6466)
- `SUPERCHAT_SERVER_SSH_HOST_KEY` - Path to SSH host key
- `SUPERCHAT_SERVER_DATABASE_PATH` - Database file path

**Limits Section:**
- `SUPERCHAT_LIMITS_MAX_CONNECTIONS_PER_IP` - Max connections per IP (default: 10)
- `SUPERCHAT_LIMITS_MESSAGE_RATE_LIMIT` - Messages per minute (default: 10)
- `SUPERCHAT_LIMITS_MAX_MESSAGE_LENGTH` - Max message bytes (default: 4096)
- `SUPERCHAT_LIMITS_MAX_NICKNAME_LENGTH` - Max nickname length (default: 20)
- `SUPERCHAT_LIMITS_SESSION_TIMEOUT_SECONDS` - Session timeout (default: 120)

**Retention Section:**
- `SUPERCHAT_RETENTION_DEFAULT_RETENTION_HOURS` - Message retention hours (default: 168 = 7 days)
- `SUPERCHAT_RETENTION_CLEANUP_INTERVAL_MINUTES` - Cleanup interval (default: 60)

**Discovery Section:**
- `SUPERCHAT_DISCOVERY_DIRECTORY_ENABLED` - Accept server registrations (default: true)
- `SUPERCHAT_DISCOVERY_PUBLIC_HOSTNAME` - Public hostname/IP for clients (auto-detect if empty)
- `SUPERCHAT_DISCOVERY_SERVER_NAME` - Display name in directory (default: "SuperChat Server")
- `SUPERCHAT_DISCOVERY_SERVER_DESCRIPTION` - Description in directory (default: "A SuperChat community server")
- `SUPERCHAT_DISCOVERY_MAX_USERS` - User limit, 0 = unlimited (default: 0)

### Option 2: Config File

To customize via config file:

1. Run the container once to generate the default config
2. Copy it out: `docker cp superchat:/data/config.toml ./config.toml`
3. Edit `config.toml`
4. Mount it back: `docker run -v ./config.toml:/data/config.toml ...`

Or create your own config and mount it:

```bash
docker run -d \
  --name superchat \
  -p 6465:6465 \
  -v superchat-data:/data \
  -v ./my-config.toml:/data/config.toml \
  aeolun/superchat:latest
```

**Note:** Environment variables override config file values.

## Data Persistence

All data is stored in `/data` inside the container:
- `/data/superchat.db` - SQLite database (messages, channels, sessions)
- `/data/config.toml` - Server configuration
- `/data/superchat.db-wal` - Write-Ahead Log (SQLite WAL mode)
- `/data/superchat.db-shm` - Shared memory file (SQLite WAL mode)

**Important:** Always use a named volume or bind mount for `/data` to persist data.

## Building the Images

SuperChat uses [Depot](https://depot.dev) for fast multi-platform builds via `depot bake`.

### Build all images (server + website)
```bash
make docker-build        # Build all images locally
make docker-build-push   # Build and push all images to Docker Hub
```

### Build individual images
```bash
make docker-build-server   # Build only server
make docker-build-website  # Build only website
```

### Manual build without Depot
```bash
# Server
docker build -t aeolun/superchat:latest .

# Website (run from repo root to include shared docs)
docker build -f website/Dockerfile -t aeolun/superchat-website:latest .
```

## Publishing to Docker Hub

```bash
# Login (first time only)
docker login

# Build and push all images
make docker-build-push
```

Images are built for both `linux/amd64` and `linux/arm64` platforms.

## Image Details

### Server Image (`aeolun/superchat`)
- **Base Image:** Alpine Linux (minimal)
- **Size:** ~30MB
- **User:** Runs as non-root user `superchat` (UID 1000)
- **Port:** 6465 (TCP)
- **Volume:** `/data`
- **Platforms:** linux/amd64, linux/arm64

### Website Image (`aeolun/superchat-website`)
- **Base Image:** nginx:alpine
- **Size:** ~25MB
- **User:** Runs as nginx default user
- **Port:** 80 (HTTP)
- **Platforms:** linux/amd64, linux/arm64

## Security

The container:
- Runs as non-root user (`superchat`)
- Uses Alpine Linux for minimal attack surface
- Only exposes port 6465
- All data isolated in `/data` volume

## Troubleshooting

### Check if container is running
```bash
docker ps | grep superchat
```

### View logs
```bash
docker logs superchat
docker logs -f superchat  # Follow logs
```

### Access container shell
```bash
docker exec -it superchat sh
```

### Inspect database
```bash
docker exec superchat ls -la /data
```

### Port already in use
If you get "address already in use", either:
1. Stop the conflicting service on port 6465
2. Use a different port: `-p 8070:6465`

## Build Configuration

The build process uses `docker-bake.hcl` to build both images simultaneously:

```hcl
group "default" {
  targets = ["server", "website"]
}

target "server" {
  context = "."
  dockerfile = "Dockerfile"
  platforms = ["linux/amd64", "linux/arm64"]
  tags = ["aeolun/superchat:latest", "aeolun/superchat:${VERSION}"]
}

target "website" {
  context = "./website"
  dockerfile = "Dockerfile"
  platforms = ["linux/amd64", "linux/arm64"]
  tags = ["aeolun/superchat-website:latest", "aeolun/superchat-website:${VERSION}"]
}
```

Version is automatically set from git tags. The build produces:
- `aeolun/superchat:latest` and `aeolun/superchat:<version>`
- `aeolun/superchat-website:latest` and `aeolun/superchat-website:<version>`
