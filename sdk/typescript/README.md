# obsevo

Lightweight TypeScript/JavaScript client for [Obsevo](https://github.com/obsevo/obsevo) — LLM tracing, prompts and scores.

## Install

```bash
npm install obsevo
# or
pnpm add obsevo
# or
yarn add obsevo
```

## Quick Start

```typescript
import { Langfuse } from "obsevo";

const client = new Langfuse({
  publicKey: "pk-lf-...",
  secretKey: "sk-lf-...",
  host: "http://localhost:3001",
});

const trace = client.trace({ name: "qa", userId: "user-1" });
const generation = trace.generation({ name: "llm", model: "gpt-4" });
generation.end({ output: "Hello!" });
await client.shutdown();
```

## Features

- **Tracing** — `trace()`, `span()`, `generation()`, `event()`
- **Scoring** — `trace.score()`, `generation.score()`
- **Prompt management** — `client.getPrompt()`
- **Auto-flush** — batches events and flushes in the background
- **Zero dependencies** — uses global `fetch` (Node 18+, Deno, Bun, browsers)
- **Langfuse compatible** — drop-in replacement for the official Langfuse SDK

## License

MIT
