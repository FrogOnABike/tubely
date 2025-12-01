# Tubely - AI Coding Agent Instructions

Tubely is a video platform backend service (similar to a shorter-form video app) with file storage and CDN integration. It's an educational project for learning Go, SQLite, S3 storage, and CloudFront distribution.

## Architecture Overview

**Monolithic HTTP API** (Go 1.23+ with stdlib `net/http`):
- Single package-based design: handlers at package root, domain logic in `internal/database` and `internal/auth`
- SQLite database with automatic migrations on startup (`internal/database/database.go`)
- File storage: local assets directory + S3 integration (env vars: `S3_BUCKET`, `S3_REGION`, `S3_CF_DISTRO`)
- Frontend served from `app/` directory (static HTML/CSS/JS)

**Request Flow**:
1. HTTP requests routed by `main.go` using Go 1.22+ route patterns (e.g., `POST /api/login`)
2. Handlers extract JWT from Authorization header and validate against `jwtSecret`
3. Database layer (`internal/database/`) executes queries and manages entities (User, Video, RefreshToken)
4. Responses use standardized `respondWithJSON()` / `respondWithError()` functions

## Key Patterns

### Authentication & Authorization
- **JWT Access Tokens**: 30-day expiry, issuer = `"tubely-access"`, subject = user UUID (see `internal/auth/auth.go`)
- **Refresh Tokens**: 60-day expiry, stored in DB, can be revoked
- **Pattern**: Extract bearer token → validate JWT → get userID → check ownership for writes
  ```go
  token, err := auth.GetBearerToken(r.Header)
  userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
  ```
- Password hashing uses Argon2id (`alexedwards/argon2id`)

### Database
- **SQLite** with schema auto-created on app startup (see `autoMigrate()`)
- **Entities**: User (email/password), Video (title/description/URLs), RefreshToken (with revoke support)
- **Patterns**:
  - Use UUIDs for all IDs (`google/uuid` package)
  - Always scan into temp variables then parse (e.g., `id` string → `uuid.Parse()`)
  - Embed `CreateXyzParams` struct in response structs for DRY
  - Timestamps: `created_at`, `updated_at` auto-set via SQL `CURRENT_TIMESTAMP`
  - Use pointer returns for queries that might return nil (e.g., `GetUser()` returns `*User`)

### Handlers
- Pattern: `func (cfg *apiConfig) handler[Name](w http.ResponseWriter, r *http.Request)`
- Inline request struct → decode JSON → validate auth → call DB → respond
- Error responses: use `respondWithError(w, statusCode, message, err)` (logs if err != nil)
- Success responses: use `respondWithJSON(w, statusCode, payload)`
- Example: `/POST /api/videos/{videoID}/thumbnail_upload` - has TODO for S3 upload implementation

### File Handling
- **Assets directory**: served at `/assets/` with caching middleware (`handler_get_thumbnail.go` retrieves from cache map)
- **In-memory thumbnail cache**: `videoThumbnails` map in `main.go` stores processed thumbnails
- **Expected tools**: ffmpeg (both `ffmpeg` and `ffprobe`) for video processing - must be in PATH
- **Future**: S3 integration for video/thumbnail storage using CloudFront URLs

### Video processing & ffprobe (important)

- The code uses `ffprobe` to inspect uploaded videos and extract stream metadata (file: `video.go`). The correct ffprobe flag is `-print_format json -show_streams` (note the underscore — `-print_format`, not `-print-format`).
- `getVideoAspectRatio(filePath)` runs ffprobe and then calls `parseVideoAspectFromJSON` to classify videos as `landscape`, `portrait`, or `other`. The parser checks (in order): `display_aspect_ratio`, `sample_aspect_ratio * (width/height)`, `coded_width/coded_height` and finally `width/height`.
- When ffprobe fails (missing binary, corrupted file, or bad args), the helper captures stderr and returns a descriptive error — handler logs will include ffprobe's stderr to help debugging.
- `handler_upload_video.go` saves uploads to a temporary file, inspects the aspect, then generates an S3 key using the aspect as a prefix (e.g. `landscape/<random-id>.mp4`) before uploading and saving the S3 URL in the DB.

### Tests & CI notes

- Unit tests covering aspect parsing live in `video_test.go` and validate the `parseVideoAspectFromJSON` logic (display/sample/coded/width+height cases) plus safeguards for invalid JSON.
- There are tests that exercise `getVideoAspectRatio` behavior with missing files; note that any test that actually runs `ffprobe` will need ffprobe in PATH. CI may skip or fail ffprobe-dependent integration tests if ffprobe is not installed — prefer unit-testing `parseVideoAspectFromJSON` where possible.


## Critical Environment Variables
All must be set (see `.env.example`):
- `DB_PATH`: SQLite database file path
- `JWT_SECRET`: Secret for signing/validating JWTs
- `PLATFORM`: Current platform identifier (dev/prod)
- `FILEPATH_ROOT`: Path to frontend app directory (e.g., `./app`)
- `ASSETS_ROOT`: Path where processed assets are stored
- `S3_BUCKET`, `S3_REGION`, `S3_CF_DISTRO`: AWS S3 configuration
- `PORT`: Server port

### S3 storage conventions (private buckets)

- The app now stores video storage references in the DB using a canonical `bucket,key` string in `video_url` for private S3 buckets (e.g. `my-bucket,landscape/<uuid>.mp4`). This keeps the DB independent of region/host format and allows the server to generate presigned URLs on demand.
- `dbVideoToSignedVideo` extracts `bucket` and `key` (supports `bucket,key`, host-style `https://<bucket>.s3.<region>.amazonaws.com/<key>`, and path-style `https://s3.amazonaws.com/<bucket>/<key>`), then generates a presigned URL using the configured S3 client.

## Development Workflow

**Setup**:
```bash
go mod download
./samplesdownload.sh  # Download test media
cp .env.example .env  # Configure environment
go run .              # Start server at http://localhost:PORT/app/
```

**Dependencies**:
- Go 1.23+, FFmpeg, SQLite3, AWS CLI
- Key Go packages: `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`, `github.com/lib/pq` (unused), `github.com/mattn/go-sqlite3`

**Testing**: Reset endpoint `/POST /admin/reset` clears all tables - useful for test suite setup

## Code Organization Checklist
- ✓ New handlers: follow `internal/auth` pattern for token extraction
- ✓ New DB queries: use `uuid.UUID` for IDs, return `*Type` for optional results
- ✓ Responses: always use `respondWithJSON/respondWithError` utilities
- ✓ File operations: assume ffmpeg available; stage files in `ASSETS_ROOT` before S3 upload
- ✓ Errors: distinguish 4XX (validation/auth) vs 5XX (server errors) in handler responses
