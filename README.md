# Re:Earth CMS Toolset

A Go-based toolset for interacting with [Re:Earth CMS](https://cms.reearth.io) from the terminal. It includes two standalone tools:

1. **`cms-chat`** — a conversational CLI that lets you query and manage CMS data in natural language
2. **`cms-seed`** — a one-shot seeder that imports a predefined dataset into your CMS model via the Import API

Both tools use only the Go standard library — no external dependencies.

---

## Project Structure

```
.
├── main.go              # Chat CLI entry point
├── chat/
│   └── handler.go       # Intent classification, Groq integration, output formatting
├── client/
│   └── cms.go           # Re:Earth CMS HTTP client (all API calls)
├── cmd/
│   └── seed/
│       └── main.go      # GeoJSON seeder (Import API)
├── go.mod
└── .env.example
```

---

## Tool 1 — Chat CLI (`cms-chat`)

### What it does

Type natural language into your terminal and the tool routes your request to the correct CMS API endpoint, then renders the response in a clean, color-coded format.

```
 Re:Earth CMS Chat
────────────────────────────────────────

Config:  Workspace: my-ws | Project: my-proj | Model: landmarks
AI:      Groq (llama-3.3-70b-versatile)
Connect: OK

Commands you can try:
  show all items
  list models
  search for Tokyo
  create item title="New Place" description="A location"
  exit

You: show all items
Thinking...

Fetched 3 / 3 items:

1. abc123def   [published]
   name:                Senso-ji Temple
   category:            temple
   city:                Tokyo

2. ghi456jkl   [published]
   name:                Himeji Castle
   ...
```

### Intent Classification

When `GROQ_API_KEY` is set, the CLI sends the user's input to Groq's API (`llama-3.3-70b-versatile`) to determine intent. If no key is provided, it falls back to keyword matching in Go — no API call needed.

| User says | Intent | CMS action |
|---|---|---|
| "show items", "list data", "what do we have?" | `LIST_ITEMS` | `GET /models/{model}/items` |
| "list models", "show schema", "what fields exist?" | `LIST_MODELS` | `GET /projects/{project}/models` |
| "search for Tokyo", "find items about shrines" | `SEARCH_ITEMS` | `GET /items?keyword=...` |
| "create item title=\"X\"", "add new record" | `CREATE_ITEM` | `POST /models/{model}/items` |

For `CREATE_ITEM`, when Groq is available the tool fetches the model's schema and asks the LLM to extract field values from the user's sentence. Without Groq, it parses `key="value"` pairs directly from the input using regex.

### Supported Commands

| Input | Result |
|---|---|
| `show all items` | Lists items with field values and status badges |
| `list models` | Shows all models with their field schemas |
| `search for Kyoto` | Filters items by keyword |
| `create item name="X" category="temple"` | Creates a new CMS item |
| `exit` / `quit` / `q` | Exits the CLI |

### Setup

```bash
# 1. Copy and fill in the config
cp .env.example .env

# 2. Build
go build -o cms-chat .

# 3. Run
./cms-chat
```

### Environment Variables

| Variable | Required | Description |
|---|---|---|
| `CMS_TOKEN` | Yes | Re:Earth CMS Integration Token |
| `CMS_WORKSPACE` | Yes | Workspace ID or alias |
| `CMS_PROJECT` | Yes | Project ID or alias |
| `CMS_MODEL` | Yes | Model ID or key |
| `GROQ_API_KEY` | No | Groq API key — enables natural language understanding. Without it, keyword matching is used. |

> Get a free Groq API key at [console.groq.com](https://console.groq.com).

---

## Tool 2 — GeoJSON Seeder (`cms-seed`)

### What it does

Seeds a Re:Earth CMS model with 10 Japanese heritage sites using the **Import API** (`PUT /import`). Instead of making one `POST` request per item, it builds a GeoJSON `FeatureCollection` in memory and uploads it in a single `multipart/form-data` request.

### Seed Data — Japanese Heritage Sites

| Name | Category | City |
|---|---|---|
| Senso-ji Temple | temple | Tokyo |
| Fushimi Inari Shrine | shrine | Kyoto |
| Himeji Castle | castle | Himeji |
| Kenroku-en Garden | garden | Kanazawa |
| Meiji Shrine | shrine | Tokyo |
| Kinkaku-ji Temple | temple | Kyoto |
| Matsumoto Castle | castle | Matsumoto |
| Shinjuku Gyoen | garden | Tokyo |
| Itsukushima Shrine | shrine | Hiroshima |
| Todai-ji Temple | temple | Nara |

Each record includes: `name`, `category`, `city`, `description`, and a GeoJSON `Point` geometry (`lng`, `lat`).

### How the Import API is used

```
PUT /{workspace}/projects/{project}/models/{model}/import?geometryFieldKey={field}
Authorization: Bearer {token}
Content-Type: multipart/form-data

file      → landmarks.geojson  (binary)
format    → "geoJson"
strategy  → "insert" | "update" | "upsert"
```

The response shows how many items were inserted, updated, or ignored:

```
──────────────────────────────────────────
Model ID:  abc123
Total:     10
Inserted:  10
```

### Usage

```bash
# Build
go build -o cms-seed ./cmd/seed/

# Run with defaults (strategy=insert, geometry field="location")
./cms-seed

# Change import strategy
./cms-seed --strategy=upsert

# Use a different geometry field name
./cms-seed --geometry-field=geo

# Dry run — prints GeoJSON to stdout without calling the API
./cms-seed --dry-run

# Pipe dry-run output to jq for inspection
./cms-seed --dry-run | jq '.features[0]'
```

### Required Model Fields

Your CMS model must have fields with these exact keys:

| Key | CMS Type |
|---|---|
| `name` | text |
| `category` | text |
| `city` | text |
| `description` | textArea |
| `location` (or your `--geometry-field` value) | geometryObject / geometryEditor |

### Environment Variables

Same as the chat CLI — only `CMS_TOKEN`, `CMS_WORKSPACE`, `CMS_PROJECT`, and `CMS_MODEL` are required. `GROQ_API_KEY` is not used by the seeder.

---

## Building Both Tools

```bash
go build -o cms-chat .
go build -o cms-seed ./cmd/seed/
```

Or run without building:

```bash
go run .                   # chat CLI
go run ./cmd/seed/         # seeder
go run ./cmd/seed/ --dry-run
```

---

## API Reference

Base URL: `https://api.cms.reearth.io`

| Method | Path | Used by |
|---|---|---|
| `GET` | `/{ws}/projects/{proj}/models` | LIST_MODELS, connection test |
| `GET` | `/{ws}/projects/{proj}/models/{model}` | CREATE_ITEM (schema fetch) |
| `GET` | `/{ws}/projects/{proj}/models/{model}/items` | LIST_ITEMS |
| `GET` | `/{ws}/projects/{proj}/models/{model}/items?keyword=` | SEARCH_ITEMS |
| `POST` | `/{ws}/projects/{proj}/models/{model}/items` | CREATE_ITEM |
| `PUT` | `/{ws}/projects/{proj}/models/{model}/import` | seeder |

All requests use `Authorization: Bearer {CMS_TOKEN}`.

---

## Security

- Never commit `.env` — it is listed in `.gitignore`
- The `GROQ_API_KEY` is optional and read only from the environment or `.env` at runtime
- No API keys are hardcoded anywhere in the source
