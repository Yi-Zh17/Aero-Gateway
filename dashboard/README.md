# Aero Gateway — Dashboard

Control panel for the Aero Gateway. Provides plugin upload and live metrics display.

> **Note:** This frontend was implemented with the assistance of Claude Code (AI agent). The Go proxy, Rust plugin, and overall system architecture were written by hand.

## Features

- Upload a `.wasm` plugin file to hot-swap the active filter without restarting the proxy
- Live metrics panel polling `GET /admin/metrics` every 2 seconds

## Development

```bash
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

The dashboard expects the Go admin server at `http://localhost:8081`. To use a different address, set the environment variable:

```bash
NEXT_PUBLIC_GATEWAY_ADMIN_URL=http://your-host:8081 npm run dev
```

## Admin API contract

The dashboard communicates with two Go endpoints:

| Endpoint | Method | Purpose |
|---|---|---|
| `/admin/upload` | `POST` | Multipart form upload, field name `plugin` |
| `/admin/metrics` | `GET` | Returns `{ total_requests, blocked_requests, last_execution_ns }` |
