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

## UX features

- **Live preview (sticky panel)** — re-renders the first sample on every option
  change (debounced 300ms, cancellable). Pick any uploaded file or pasted URL
  as the preview sample.
- **Saved presets** — name and reuse option combinations. Stored in
  `localStorage`; export/import as JSON.
- **Share link** — every option change is mirrored to `location.hash` (base64url
  JSON). Copy the URL to share the exact configuration (no files included).
- **Drag reorder + per-file rename + filename template** — drag handle on each
  file card; rename inline; global template supports `{name}`, `{ext}`, `{i}`,
  `{i:02d}` (zero-padded index).
- **Local history** — last 20 batches are kept in `localStorage`. Click any
  entry to reload its spec.

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

## Run with Docker Compose (Tailscale-only access)

The example compose file does not publish ports to the host. Access is via a
[Tailscale](https://tailscale.com) sidecar — only members of your tailnet can
reach the UI.

1. Create a reusable auth key in the
   [Tailscale admin console](https://login.tailscale.com/admin/settings/keys)
   (Reusable + Pre-approved + tagged).
2. Copy `.env.example` to `.env` and fill in `TS_AUTHKEY`:
   ```bash
   cp .env.example .env
   $EDITOR .env
   ```
3. Bring everything up:
   ```bash
   docker compose -f docker-compose.example.yml --env-file .env up -d --build
   ```
4. Find the tailnet IP and open the UI:
   ```bash
   tailscale status | grep imgproxy-web
   open http://<tailnet-ip>:8081
   ```

The three containers share the `tailscale` sidecar's network namespace
(`network_mode: "service:tailscale"`). `imgproxy` binds `127.0.0.1:8080`
(loopback only inside that namespace), and `imgproxy-web` binds the tailscale
interface on `:8081`. There is no `ports:` mapping, so the host never exposes
anything publicly.

To run without Tailscale (loopback dev only), see "Run locally (bare metal)"
above.

## HTTP API

| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/` | — | HTML UI |
| GET | `/static/*` | — | embedded JS / CSS |
| GET | `/api/healthz` | — | `{status, upstream}` JSON |
| GET | `/api/options` | — | full option catalog (drives the form) |
| POST | `/api/convert` | multipart: `file` (×N), `spec` (JSON) | ZIP |
| POST | `/api/convert-url` | JSON: `{urls:[…], spec:{…}}` | ZIP |
| POST | `/api/preview` | multipart (1 file + `spec`) **or** JSON `{url, spec}` | single processed image bytes |

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
  "raw": "rs:fit:800:600/q:85/f:avif",
  // Optional: rename ZIP entries with placeholders {name}, {ext}, {i}, {i:02d}.
  "filename_template": "{i:03d}-{name}.{ext}"
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
