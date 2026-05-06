# imgproxy-web

Sidecar HTTP service that puts a UI on top of [imgproxy](https://imgproxy.net)
and adds bulk image conversion.

imgproxy itself is a GET-only URL processor — it does not accept uploads.
`imgproxy-web` fills the gap: it serves a single-page UI, accepts multi-file
uploads (or a list of remote URLs), drops each file in a directory shared with
imgproxy via the `local://` source scheme, fans out parallel requests to
imgproxy, and streams a ZIP of processed results back to the browser.

The UI surfaces every imgproxy processing option (resize, crop, gravity,
rotation, filters, watermark, metadata, presets, security overrides, …) plus a
"raw URL options" escape hatch so future or undocumented options stay reachable
without a code change.

## Layout

```
web/
  cmd/imgproxy-web/    entrypoint (single binary, stdlib-only, no cgo)
  config/              env loader
  client/              imgproxy URL builder + HMAC signer + HTTP client
  schema/              option catalog served at /api/options
  server/              HTTP server, /api/* handlers, ZIP streaming
  static/              vanilla HTML/JS/CSS (embedded via embed.FS)
```

## Run locally (bare metal)

Two processes, one shared directory:

```bash
make           # builds imgproxy
make web       # builds imgproxy-web

mkdir -p /tmp/uploads

IMGPROXY_BIND=:8080 \
  IMGPROXY_LOCAL_FILESYSTEM_ROOT=/tmp/uploads \
  IMGPROXY_ALLOW_ORIGIN='*' \
  ./imgproxy &

IMGPROXY_WEB_BIND=:8081 \
  IMGPROXY_WEB_IMGPROXY_URL=http://localhost:8080 \
  IMGPROXY_WEB_UPLOAD_DIR=/tmp/uploads \
  ./imgproxy-web &

open http://localhost:8081
```

## Run with Docker Compose

```bash
docker compose -f docker-compose.example.yml up
open http://localhost:8081
```

The compose file mounts a shared volume on `/data/uploads` and points both
services at it.

## HTTP API

| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/` | — | HTML UI |
| GET | `/static/*` | — | embedded JS / CSS |
| GET | `/api/healthz` | — | `{status, upstream}` JSON |
| GET | `/api/options` | — | full option catalog (drives the form) |
| POST | `/api/convert` | multipart: `file` (×N), `spec` (JSON) | ZIP |
| POST | `/api/convert-url` | JSON: `{urls:[…], spec:{…}}` | ZIP |

### Spec shape

```jsonc
{
  // Either:
  "options": {
    "format": "webp",
    "quality": 85,
    "width": 1200,
    "resizing_type": "fit",
    "strip_metadata": true,
    "gravity": { "type": "fp", "x": 0.5, "y": 0.3 },
    "format_quality": [
      { "format": "jpeg", "quality": 90 },
      { "format": "webp", "quality": 80 }
    ]
  },
  // Or, fully formed (wins over `options` when both are present):
  "raw": "rs:fit:800:600/q:85/f:avif"
}
```

The server validates each `options` key against the schema and rejects unknown
keys. `raw` is passed through verbatim, so it covers any imgproxy option the
schema doesn't yet model.

### CLI smoke

```bash
# Bulk upload conversion
curl -F 'file=@a.jpg' -F 'file=@b.png' \
  -F 'spec={"options":{"format":"webp","quality":85,"width":800}}' \
  http://localhost:8081/api/convert -o out.zip
unzip -l out.zip   # → a.webp, b.webp

# Remote URLs (no upload)
curl -X POST http://localhost:8081/api/convert-url \
  -H 'Content-Type: application/json' \
  -d '{"urls":["https://picsum.photos/800"],"spec":{"options":{"format":"avif"}}}' \
  -o url.zip
```

## Configuration

All settings come from environment variables. Defaults work for local dev.

| Env | Default | Effect |
|---|---|---|
| `IMGPROXY_WEB_BIND` | `:8081` | bind address |
| `IMGPROXY_WEB_IMGPROXY_URL` | `http://localhost:8080` | upstream imgproxy URL |
| `IMGPROXY_WEB_UPLOAD_DIR` | `/tmp/imgproxy-web` | directory shared with imgproxy (must match `IMGPROXY_LOCAL_FILESYSTEM_ROOT`) |
| `IMGPROXY_WEB_MAX_UPLOAD_SIZE` | `104857600` (100 MiB) | per-file upload limit |
| `IMGPROXY_WEB_MAX_BATCH` | `200` | max files per request |
| `IMGPROXY_WEB_CONCURRENCY` | `runtime.NumCPU()` | parallel imgproxy requests per batch |
| `IMGPROXY_WEB_KEY` | `IMGPROXY_KEY` if set | HMAC key (hex), space-separated for multi-pair |
| `IMGPROXY_WEB_SALT` | `IMGPROXY_SALT` if set | HMAC salt (hex), space-separated for multi-pair |
| `IMGPROXY_WEB_SIGNATURE_SIZE` | `32` | bytes of signature to keep |
| `IMGPROXY_WEB_BEARER` | `IMGPROXY_SECRET` if set | bearer token sent to imgproxy |
| `IMGPROXY_WEB_TIMEOUT_SEC` | `60` | per-request upstream timeout |
| `IMGPROXY_WEB_ALLOW_ORIGIN` | empty | adds `Access-Control-Allow-Origin` |

## Security

- The sidecar trusts whoever can reach it; put it behind a reverse proxy + auth
  if exposed.
- Uploaded files land in `IMGPROXY_WEB_UPLOAD_DIR` with random 8-byte hex names
  and the original extension; the sidecar deletes them after the response is
  sent.
- When imgproxy requires HMAC signatures, set `IMGPROXY_WEB_KEY` /
  `IMGPROXY_WEB_SALT` to the same hex values used by imgproxy. The sidecar
  signs each generated URL.

## Tests

```bash
make web-test
```

Covers:

- HMAC signature parity with imgproxy (using its `signature_test.go` known
  vector: `test-key` / `test-salt` / `asd` →
  `dtLwhdnPPiu_epMl1LrzheLpvHas-4mwvY6L3Z8WwlY`).
- URL-builder encoding for every compound option (gravity, crop, extend, trim,
  padding, watermark, format_quality, flip, filename, list, scalars).
