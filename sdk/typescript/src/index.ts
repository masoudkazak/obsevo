/**
 * Langfuse Light — lightweight TypeScript/JavaScript client.
 *
 * Dependency-free: it uses the global `fetch`, available in Node 18+, Deno,
 * Bun and browsers. Events are buffered and flushed in the background, so
 * instrumenting a call adds no network round trip to the request path.
 *
 *   const lf = new Langfuse({ publicKey, secretKey, host });
 *
 *   const trace = lf.trace({ name: "qa", userId: "u1", input: question });
 *   const gen = trace.generation({ name: "answer", model: "gpt-4o", input: question });
 *   gen.end({ output: reply, usage: { input: 120, output: 45 } });
 *   trace.update({ output: reply });
 *   trace.score({ name: "helpfulness", value: 0.9 });
 *   await lf.shutdown();
 */

export type Json = unknown;

/** Token usage in any of the shapes the server normalises. */
export interface Usage {
  input?: number;
  output?: number;
  total?: number;
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  [key: string]: unknown;
}

export type ObservationLevel = "DEBUG" | "DEFAULT" | "WARNING" | "ERROR";
export type ScoreDataType = "NUMERIC" | "CATEGORICAL" | "BOOLEAN";

export interface LangfuseOptions {
  publicKey?: string;
  secretKey?: string;
  host?: string;
  /** Flush once this many events are buffered. Default 50. */
  flushAt?: number;
  /** Flush at least this often, in milliseconds. Default 2000. */
  flushInterval?: number;
  /** Set false to make every call a no-op. */
  enabled?: boolean;
  /** Per-request timeout in milliseconds. Default 10000. */
  timeout?: number;
  /** Attempts per flush before the batch is dropped. Default 3. */
  maxRetries?: number;
  /** Receives transport failures; defaults to console.warn. */
  onError?: (error: unknown) => void;
}

interface IngestionEvent {
  id: string;
  type: string;
  timestamp: string;
  body: Record<string, Json>;
}

function newId(): string {
  const cryptoRef = (globalThis as { crypto?: Crypto }).crypto;
  if (cryptoRef && typeof cryptoRef.randomUUID === "function") {
    return cryptoRef.randomUUID();
  }
  // Fallback for runtimes without WebCrypto. Ids only need to be unique
  // within a project, not unguessable.
  return `${Date.now().toString(16)}-${Math.random().toString(16).slice(2, 14)}`;
}

function now(): string {
  return new Date().toISOString();
}

function env(name: string): string | undefined {
  const proc = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process;
  return proc?.env?.[name];
}

function base64(input: string): string {
  const g = globalThis as { btoa?: (s: string) => string; Buffer?: { from(s: string, e: string): { toString(e: string): string } } };
  if (typeof g.btoa === "function") return g.btoa(input);
  if (g.Buffer) return g.Buffer.from(input, "utf-8").toString("base64");
  throw new Error("No base64 encoder available in this runtime");
}

/** Drops keys whose value is undefined, so absent fields are not sent as null. */
function compact(fields: Record<string, Json>): Record<string, Json> {
  const out: Record<string, Json> = {};
  for (const [key, value] of Object.entries(fields)) {
    if (value !== undefined) out[key] = value;
  }
  return out;
}

export class LangfuseError extends Error {}

export class Langfuse {
  readonly publicKey: string;
  readonly secretKey: string;
  readonly host: string;
  readonly enabled: boolean;

  private readonly flushAt: number;
  private readonly flushInterval: number;
  private readonly timeout: number;
  private readonly maxRetries: number;
  private readonly onError: (error: unknown) => void;

  private queue: IngestionEvent[] = [];
  private timer: ReturnType<typeof setInterval> | undefined;
  private inFlight: Promise<void> = Promise.resolve();

  constructor(options: LangfuseOptions = {}) {
    this.publicKey = options.publicKey ?? env("LANGFUSE_PUBLIC_KEY") ?? "";
    this.secretKey = options.secretKey ?? env("LANGFUSE_SECRET_KEY") ?? "";
    this.host = (options.host ?? env("LANGFUSE_HOST") ?? "http://localhost:3001").replace(/\/+$/, "");
    this.flushAt = Math.max(1, options.flushAt ?? 50);
    this.flushInterval = options.flushInterval ?? 2000;
    this.timeout = options.timeout ?? 10000;
    this.maxRetries = options.maxRetries ?? 3;
    this.onError = options.onError ?? ((error) => console.warn("[langfuse-light]", error));

    this.enabled = (options.enabled ?? true) && Boolean(this.publicKey && this.secretKey);
    if ((options.enabled ?? true) && !this.enabled) {
      console.warn("[langfuse-light] disabled: set LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY");
    }

    if (this.enabled && this.flushInterval > 0) {
      this.timer = setInterval(() => void this.flush(), this.flushInterval);
      // Do not hold the Node event loop open on account of the flush timer.
      (this.timer as unknown as { unref?: () => void }).unref?.();
    }
  }

  /** Start a trace, the root of one application request. */
  trace(options: TraceOptions = {}): Trace {
    const trace = new Trace(this, options.id ?? newId(), options);
    trace.update({ ...options, timestamp: now() }, true);
    return trace;
  }

  /** Attach a score to a trace, an observation, a session or a dataset run. */
  score(options: ScoreOptions): string {
    const id = options.id ?? newId();
    this.enqueue("score-create", compact({
      id,
      name: options.name,
      value: options.value,
      traceId: options.traceId,
      observationId: options.observationId,
      sessionId: options.sessionId,
      datasetRunId: options.datasetRunId,
      comment: options.comment,
      dataType: options.dataType,
      metadata: options.metadata,
    }));
    return id;
  }

  /** Fetch a managed prompt, by version or label (default: production). */
  async getPrompt(name: string, options: { version?: number; label?: string } = {}): Promise<Prompt> {
    const params = new URLSearchParams();
    if (options.version !== undefined) params.set("version", String(options.version));
    if (options.label) params.set("label", options.label);
    const query = params.toString() ? `?${params.toString()}` : "";

    const payload = await this.request("GET", `/api/public/v2/prompts/${encodeURIComponent(name)}${query}`);
    return new Prompt(payload as PromptPayload);
  }

  /** Send everything buffered. */
  async flush(): Promise<void> {
    if (!this.enabled) return;

    const batch = this.queue;
    this.queue = [];
    if (batch.length === 0) return;

    // Serialise deliveries so events reach the server in the order produced.
    this.inFlight = this.inFlight.then(() => this.deliver(batch));
    return this.inFlight;
  }

  /** Flush and stop the background timer. */
  async shutdown(): Promise<void> {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = undefined;
    }
    await this.flush();
    await this.inFlight;
  }

  /** @internal */
  enqueue(type: string, body: Record<string, Json>): void {
    if (!this.enabled) return;

    this.queue.push({ id: newId(), type, timestamp: now(), body });
    if (this.queue.length >= this.flushAt) void this.flush();
  }

  private async deliver(batch: IngestionEvent[]): Promise<void> {
    for (let attempt = 0; attempt < this.maxRetries; attempt++) {
      try {
        await this.request("POST", "/api/public/ingestion", { batch });
        return;
      } catch (error) {
        // A 4xx will never be accepted; retrying only delays what follows.
        if (error instanceof LangfuseError) {
          this.onError(error);
          return;
        }
        if (attempt === this.maxRetries - 1) {
          this.onError(new Error(`dropped ${batch.length} event(s): ${String(error)}`));
          return;
        }
        await new Promise((resolve) => setTimeout(resolve, 2 ** attempt * 200));
      }
    }
  }

  private async request(method: string, path: string, body?: unknown): Promise<unknown> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(this.host + path, {
        method,
        headers: {
          "Content-Type": "application/json",
          Authorization: `Basic ${base64(`${this.publicKey}:${this.secretKey}`)}`,
          "User-Agent": "langfuse-light-typescript",
        },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
      });

      if (!response.ok) {
        const detail = (await response.text()).slice(0, 500);
        if (response.status >= 400 && response.status < 500) {
          throw new LangfuseError(`HTTP ${response.status}: ${detail}`);
        }
        throw new Error(`HTTP ${response.status}: ${detail}`);
      }

      const text = await response.text();
      return text ? JSON.parse(text) : undefined;
    } finally {
      clearTimeout(timer);
    }
  }
}

export interface TraceOptions {
  id?: string;
  name?: string;
  userId?: string;
  sessionId?: string;
  input?: Json;
  output?: Json;
  metadata?: Json;
  tags?: string[];
  release?: string;
  version?: string;
  environment?: string;
}

export interface ScoreOptions {
  name: string;
  value?: number | string;
  id?: string;
  traceId?: string;
  observationId?: string;
  sessionId?: string;
  datasetRunId?: string;
  comment?: string;
  dataType?: ScoreDataType;
  metadata?: Json;
}

export interface ObservationOptions {
  id?: string;
  name?: string;
  input?: Json;
  output?: Json;
  metadata?: Json;
  level?: ObservationLevel;
  statusMessage?: string;
  version?: string;
  environment?: string;
}

export interface GenerationOptions extends ObservationOptions {
  model?: string;
  modelParameters?: Json;
  promptName?: string;
  promptVersion?: number;
  completionStartTime?: string;
}

export interface GenerationEndOptions extends ObservationOptions {
  usage?: Usage;
  cost?: number;
  completionStartTime?: string;
}

/** Shared behaviour for spans, generations and events. */
abstract class Observation {
  protected abstract readonly eventPrefix: string;
  private ended = false;

  constructor(
    protected readonly client: Langfuse,
    readonly traceId: string,
    readonly id: string,
    readonly name?: string,
    readonly parentObservationId?: string,
  ) {}

  protected body(fields: Record<string, Json>): Record<string, Json> {
    return compact({
      id: this.id,
      traceId: this.traceId,
      name: this.name,
      parentObservationId: this.parentObservationId,
      ...fields,
    });
  }

  /** Send a partial update for this observation. */
  update(fields: Record<string, Json>): this {
    this.client.enqueue(`${this.eventPrefix}-update`, this.body(fields));
    return this;
  }

  /** Close the observation, recording its end time. */
  end(fields: Record<string, Json> = {}): this {
    if (this.ended) return this;
    this.ended = true;
    this.client.enqueue(`${this.eventPrefix}-update`, this.body({ endTime: now(), ...fields }));
    return this;
  }

  /** Start a child span. */
  span(options: ObservationOptions = {}): Span {
    return startSpan(this.client, this.traceId, options, this.id);
  }

  /** Start a child generation. */
  generation(options: GenerationOptions = {}): Generation {
    return startGeneration(this.client, this.traceId, options, this.id);
  }

  /** Record a child point-in-time event. */
  event(options: ObservationOptions = {}): LangfuseEvent {
    return recordEvent(this.client, this.traceId, options, this.id);
  }

  /** Attach a score to this observation. */
  score(options: Omit<ScoreOptions, "traceId" | "observationId">): string {
    return this.client.score({ ...options, traceId: this.traceId, observationId: this.id });
  }
}

export class Span extends Observation {
  protected readonly eventPrefix = "span";
}

export class LangfuseEvent extends Observation {
  protected readonly eventPrefix = "event";
}

export class Generation extends Observation {
  protected readonly eventPrefix = "generation";

  /** Close the generation with its output, token usage and optional cost. */
  end(options: GenerationEndOptions = {}): this {
    return super.end(compact({ ...options } as Record<string, Json>));
  }
}

export class Trace {
  constructor(
    private readonly client: Langfuse,
    readonly id: string,
    private options: TraceOptions,
  ) {}

  /** Create or update the trace record. */
  update(fields: TraceOptions & { timestamp?: string } = {}, initial = false): this {
    this.options = { ...this.options, ...fields };

    const body = compact({
      id: this.id,
      name: this.options.name,
      userId: this.options.userId,
      sessionId: this.options.sessionId,
      input: fields.input,
      output: fields.output,
      metadata: fields.metadata,
      tags: this.options.tags,
      release: this.options.release,
      version: this.options.version,
      environment: this.options.environment,
      timestamp: fields.timestamp,
    });

    this.client.enqueue(initial ? "trace-create" : "trace-update", body);
    return this;
  }

  /** Start a span under this trace. */
  span(options: ObservationOptions = {}): Span {
    return startSpan(this.client, this.id, options);
  }

  /** Start a generation under this trace. */
  generation(options: GenerationOptions = {}): Generation {
    return startGeneration(this.client, this.id, options);
  }

  /** Record an event under this trace. */
  event(options: ObservationOptions = {}): LangfuseEvent {
    return recordEvent(this.client, this.id, options);
  }

  /** Attach a score to this trace. */
  score(options: Omit<ScoreOptions, "traceId">): string {
    return this.client.score({ ...options, traceId: this.id });
  }
}

interface PromptPayload {
  name?: string;
  version?: number;
  type?: string;
  labels?: string[];
  config?: Record<string, unknown>;
  prompt?: unknown;
  variables?: string[];
}

export interface ChatMessage {
  role: string;
  content: string;
  [key: string]: unknown;
}

export class Prompt {
  readonly name: string;
  readonly version: number;
  readonly type: string;
  readonly labels: string[];
  readonly config: Record<string, unknown>;
  readonly prompt: unknown;
  readonly variables: string[];

  constructor(payload: PromptPayload) {
    this.name = payload.name ?? "";
    this.version = payload.version ?? 0;
    this.type = payload.type ?? "text";
    this.labels = payload.labels ?? [];
    this.config = payload.config ?? {};
    this.prompt = payload.prompt;
    this.variables = payload.variables ?? [];
  }

  /**
   * Substitute `{{variable}}` placeholders. Returns a string for a text prompt
   * and a message list for a chat prompt, ready to pass to a model client.
   */
  compile(variables: Record<string, unknown> = {}): string | ChatMessage[] {
    if (this.type === "chat" && Array.isArray(this.prompt)) {
      return (this.prompt as ChatMessage[]).map((message) => ({
        ...message,
        content: substitute(message.content ?? "", variables),
      }));
    }
    return substitute(typeof this.prompt === "string" ? this.prompt : "", variables);
  }
}

function substitute(template: string, variables: Record<string, unknown>): string {
  return Object.entries(variables).reduce(
    (text, [key, value]) => text.split(`{{${key}}}`).join(String(value)),
    template,
  );
}

function startSpan(client: Langfuse, traceId: string, options: ObservationOptions, parentId?: string): Span {
  const span = new Span(client, traceId, options.id ?? newId(), options.name, parentId);
  client.enqueue("span-create", span["body"]({ startTime: now(), ...compact(options as Record<string, Json>) }));
  return span;
}

function startGeneration(client: Langfuse, traceId: string, options: GenerationOptions, parentId?: string): Generation {
  const generation = new Generation(client, traceId, options.id ?? newId(), options.name, parentId);
  client.enqueue("generation-create", generation["body"]({ startTime: now(), ...compact(options as Record<string, Json>) }));
  return generation;
}

function recordEvent(client: Langfuse, traceId: string, options: ObservationOptions, parentId?: string): LangfuseEvent {
  const event = new LangfuseEvent(client, traceId, options.id ?? newId(), options.name, parentId);
  client.enqueue("event-create", event["body"]({ startTime: now(), ...compact(options as Record<string, Json>) }));
  return event;
}

/**
 * Wrap a function so each call becomes a span (or generation) on a trace.
 *
 * The outermost wrapped call opens the trace; nested calls become its children.
 */
export function observe<Args extends unknown[], Result>(
  client: Langfuse,
  fn: (...args: Args) => Promise<Result> | Result,
  options: { name?: string; asType?: "span" | "generation" } = {},
): (...args: Args) => Promise<Result> {
  const label = options.name ?? fn.name ?? "anonymous";

  return async (...args: Args): Promise<Result> => {
    if (!client.enabled) return await fn(...args);

    const parent = currentObservation;
    const trace = parent ? undefined : client.trace({ name: label, input: args });
    const owner: Trace | Observation = parent ?? (trace as Trace);
    const node =
      options.asType === "generation"
        ? owner.generation({ name: label, input: args as unknown as Json })
        : owner.span({ name: label, input: args as unknown as Json });

    currentObservation = node;
    try {
      const result = await fn(...args);
      node.end({ output: result as Json });
      trace?.update({ output: result as Json });
      return result;
    } catch (error) {
      node.end({ level: "ERROR", statusMessage: String(error) });
      trace?.update({ metadata: { error: String(error) } });
      throw error;
    } finally {
      currentObservation = parent;
    }
  };
}

// Tracks the enclosing observation so nested observe() calls nest correctly.
// This is a module-level variable rather than AsyncLocalStorage to keep the
// package runtime-agnostic; concurrent independent traces should pass their
// parent explicitly instead of relying on it.
let currentObservation: Observation | undefined;
