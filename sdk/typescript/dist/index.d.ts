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
export declare class LangfuseError extends Error {
}
export declare class Langfuse {
    readonly publicKey: string;
    readonly secretKey: string;
    readonly host: string;
    readonly enabled: boolean;
    private readonly flushAt;
    private readonly flushInterval;
    private readonly timeout;
    private readonly maxRetries;
    private readonly onError;
    private queue;
    private timer;
    private inFlight;
    constructor(options?: LangfuseOptions);
    /** Start a trace, the root of one application request. */
    trace(options?: TraceOptions): Trace;
    /** Attach a score to a trace, an observation, a session or a dataset run. */
    score(options: ScoreOptions): string;
    /** Fetch a managed prompt, by version or label (default: production). */
    getPrompt(name: string, options?: {
        version?: number;
        label?: string;
    }): Promise<Prompt>;
    /** Send everything buffered. */
    flush(): Promise<void>;
    /** Flush and stop the background timer. */
    shutdown(): Promise<void>;
    /** @internal */
    enqueue(type: string, body: Record<string, Json>): void;
    private deliver;
    private request;
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
declare abstract class Observation {
    protected readonly client: Langfuse;
    readonly traceId: string;
    readonly id: string;
    readonly name?: string | undefined;
    readonly parentObservationId?: string | undefined;
    protected abstract readonly eventPrefix: string;
    private ended;
    constructor(client: Langfuse, traceId: string, id: string, name?: string | undefined, parentObservationId?: string | undefined);
    protected body(fields: Record<string, Json>): Record<string, Json>;
    /** Send a partial update for this observation. */
    update(fields: Record<string, Json>): this;
    /** Close the observation, recording its end time. */
    end(fields?: Record<string, Json>): this;
    /** Start a child span. */
    span(options?: ObservationOptions): Span;
    /** Start a child generation. */
    generation(options?: GenerationOptions): Generation;
    /** Record a child point-in-time event. */
    event(options?: ObservationOptions): LangfuseEvent;
    /** Attach a score to this observation. */
    score(options: Omit<ScoreOptions, "traceId" | "observationId">): string;
}
export declare class Span extends Observation {
    protected readonly eventPrefix = "span";
}
export declare class LangfuseEvent extends Observation {
    protected readonly eventPrefix = "event";
}
export declare class Generation extends Observation {
    protected readonly eventPrefix = "generation";
    /** Close the generation with its output, token usage and optional cost. */
    end(options?: GenerationEndOptions): this;
}
export declare class Trace {
    private readonly client;
    readonly id: string;
    private options;
    constructor(client: Langfuse, id: string, options: TraceOptions);
    /** Create or update the trace record. */
    update(fields?: TraceOptions & {
        timestamp?: string;
    }, initial?: boolean): this;
    /** Start a span under this trace. */
    span(options?: ObservationOptions): Span;
    /** Start a generation under this trace. */
    generation(options?: GenerationOptions): Generation;
    /** Record an event under this trace. */
    event(options?: ObservationOptions): LangfuseEvent;
    /** Attach a score to this trace. */
    score(options: Omit<ScoreOptions, "traceId">): string;
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
export declare class Prompt {
    readonly name: string;
    readonly version: number;
    readonly type: string;
    readonly labels: string[];
    readonly config: Record<string, unknown>;
    readonly prompt: unknown;
    readonly variables: string[];
    constructor(payload: PromptPayload);
    /**
     * Substitute `{{variable}}` placeholders. Returns a string for a text prompt
     * and a message list for a chat prompt, ready to pass to a model client.
     */
    compile(variables?: Record<string, unknown>): string | ChatMessage[];
}
/**
 * Wrap a function so each call becomes a span (or generation) on a trace.
 *
 * The outermost wrapped call opens the trace; nested calls become its children.
 */
export declare function observe<Args extends unknown[], Result>(client: Langfuse, fn: (...args: Args) => Promise<Result> | Result, options?: {
    name?: string;
    asType?: "span" | "generation";
}): (...args: Args) => Promise<Result>;
export {};
