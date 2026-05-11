# Re:Earth CMS Chat CLI

A terminal chatbot for querying [Re:Earth CMS](https://cms.reearth.io) in natural language. Type a question, get a formatted answer — no API knowledge required.

Uses only the Go standard library. The only optional external dependency is a Groq API key for natural language understanding; without it the tool falls back to keyword matching.

---

## Project Structure

```
.
├── main.go          # CLI entry point
├── chat/
│   └── handler.go   # Intent classification, Groq integration, output formatting
├── client/
│   └── cms.go       # Re:Earth CMS HTTP client
├── go.mod
└── .env.example
```

---

## Setup

```bash
# 1. Clone and enter the repo
git clone https://github.com/mimoham24/reearth-cms-chatbot
cd reearth-cms-chatbot

# 2. Configure
cp .env.example .env
# edit .env with your values

# 3. Build
CGO_ENABLED=0 go build -o cms-chat .

# 4. Run
./cms-chat
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `CMS_TOKEN` | Yes | Re:Earth CMS Integration Token |
| `CMS_WORKSPACE` | Yes | Workspace ID or alias |
| `CMS_PROJECT` | Yes | Project ID or alias |
| `CMS_MODEL` | Yes | Model ID or key |
| `GROQ_API_KEY` | No | Enables natural language understanding (llama-3.3-70b). Without it, keyword matching is used. |

> Free Groq API keys at [console.groq.com](https://console.groq.com).

---

## Using the Chatbot

Once running, you'll see a prompt where you can type freely in natural language:

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
  exit

You: █
```

### Listing items

```
You: show all items

Fetched 10 / 10 items:

1. abc123def   [published]
   name:                Senso-ji Temple
   category:            temple
   city:                Tokyo
   description:         Tokyo's oldest temple, founded in 628 AD

2. ghi456jkl   [published]
   name:                Himeji Castle
   category:            castle
   city:                Himeji
   ...
```

### Listing models and schema

```
You: what models do we have?

2 model(s):

  landmarks  key: landmarks
    name:                text
    category:            text
    city:                text
    description:         textArea
    location:            geometryObject
```

### Searching

```
You: find items in Kyoto

Search "Kyoto" — 2 / 10 items:

1. def789abc   [published]
   name:                Fushimi Inari Shrine
   city:                Kyoto
   ...
```

### Exiting

```
You: exit
Goodbye!
```

You can also type `quit` or `q`.

---

## Supported Queries

The chatbot understands natural language variations. Some examples:

| Goal | Example phrases |
|---|---|
| List all items | "show all items", "what data do we have?", "list records" |
| Show schema | "list models", "what fields exist?", "show schema" |
| Search | "search for Tokyo", "find shrines", "items in Kyoto" |

The chat CLI is **query-only**. Data is managed through the [Re:Earth CMS](https://cms.reearth.io) web interface.

---

## How Intent Classification Works

When `GROQ_API_KEY` is set, user input is sent to Groq's API to determine intent. If the key is missing or the API call fails, the tool falls back to keyword matching in Go — no external calls, no latency.

| Mode | How it works |
|---|---|
| Groq | `llama-3.3-70b-versatile` classifies intent, `temperature=0` for deterministic output |
| Fallback | Keyword scan — "search"→`SEARCH_ITEMS`, "model"→`LIST_MODELS`, default→`LIST_ITEMS` |

---

## API Reference

Base URL: `https://api.cms.reearth.io/api`  
Auth: `Authorization: Bearer {CMS_TOKEN}`

| Method | Path | Used for |
|---|---|---|
| `GET` | `/{ws}/projects/{proj}/models` | LIST_MODELS + connection test |
| `GET` | `/{ws}/projects/{proj}/models/{model}/items` | LIST_ITEMS |
| `GET` | `/{ws}/projects/{proj}/models/{model}/items?keyword=` | SEARCH_ITEMS |

---

## Security

- Never commit `.env` — it is listed in `.gitignore`
- `GROQ_API_KEY` is optional and only read from the environment at runtime
- No credentials are hardcoded in the source
