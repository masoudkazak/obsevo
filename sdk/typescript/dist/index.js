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
function newId() {
    const cryptoRef = globalThis.crypto;
    if (cryptoRef && typeof cryptoRef.randomUUID === "function") {
        return cryptoRef.randomUUID();
    }
    // Fallback for runtimes without WebCrypto. Ids only need to be unique
    // within a project, not unguessable.
    return `${Date.now().toString(16)}-${Math.random().toString(16).slice(2, 14)}`;
}
function now() {
    return new Date().toISOString();
}
function env(name) {
    const proc = globalThis.process;
    return proc?.env?.[name];
}
function base64(input) {
    const g = globalThis;
    if (typeof g.btoa === "function")
        return g.btoa(input);
    if (g.Buffer)
        return g.Buffer.from(input, "utf-8").toString("base64");
    throw new Error("No base64 encoder available in this runtime");
}
/** Drops keys whose value is undefined, so absent fields are not sent as null. */
function compact(fields) {
    const out = {};
    for (const [key, value] of Object.entries(fields)) {
        if (value !== undefined)
            out[key] = value;
    }
    return out;
}
export class LangfuseError extends Error {
}
export class Langfuse {
    publicKey;
    secretKey;
    host;
    enabled;
    flushAt;
    flushInterval;
    timeout;
    maxRetries;
    onError;
    queue = [];
    timer;
    inFlight = Promise.resolve();
    constructor(options = {}) {
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
            this.timer.unref?.();
        }
    }
    /** Start a trace, the root of one application request. */
    trace(options = {}) {
        const trace = new Trace(this, options.id ?? newId(), options);
        trace.update({ ...options, timestamp: now() }, true);
        return trace;
    }
    /** Attach a score to a trace, an observation, a session or a dataset run. */
    score(options) {
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
    async getPrompt(name, options = {}) {
        const params = new URLSearchParams();
        if (options.version !== undefined)
            params.set("version", String(options.version));
        if (options.label)
            params.set("label", options.label);
        const query = params.toString() ? `?${params.toString()}` : "";
        const payload = await this.request("GET", `/api/public/v2/prompts/${encodeURIComponent(name)}${query}`);
        return new Prompt(payload);
    }
    /** Send everything buffered. */
    async flush() {
        if (!this.enabled)
            return;
        const batch = this.queue;
        this.queue = [];
        if (batch.length === 0)
            return;
        // Serialise deliveries so events reach the server in the order produced.
        this.inFlight = this.inFlight.then(() => this.deliver(batch));
        return this.inFlight;
    }
    /** Flush and stop the background timer. */
    async shutdown() {
        if (this.timer) {
            clearInterval(this.timer);
            this.timer = undefined;
        }
        await this.flush();
        await this.inFlight;
    }
    /** @internal */
    enqueue(type, body) {
        if (!this.enabled)
            return;
        this.queue.push({ id: newId(), type, timestamp: now(), body });
        if (this.queue.length >= this.flushAt)
            void this.flush();
    }
    async deliver(batch) {
        for (let attempt = 0; attempt < this.maxRetries; attempt++) {
            try {
                await this.request("POST", "/api/public/ingestion", { batch });
                return;
            }
            catch (error) {
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
    async request(method, path, body) {
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
        }
        finally {
            clearTimeout(timer);
        }
    }
}
/** Shared behaviour for spans, generations and events. */
class Observation {
    client;
    traceId;
    id;
    name;
    parentObservationId;
    ended = false;
    constructor(client, traceId, id, name, parentObservationId) {
        this.client = client;
        this.traceId = traceId;
        this.id = id;
        this.name = name;
        this.parentObservationId = parentObservationId;
    }
    body(fields) {
        return compact({
            id: this.id,
            traceId: this.traceId,
            name: this.name,
            parentObservationId: this.parentObservationId,
            ...fields,
        });
    }
    /** Send a partial update for this observation. */
    update(fields) {
        this.client.enqueue(`${this.eventPrefix}-update`, this.body(fields));
        return this;
    }
    /** Close the observation, recording its end time. */
    end(fields = {}) {
        if (this.ended)
            return this;
        this.ended = true;
        this.client.enqueue(`${this.eventPrefix}-update`, this.body({ endTime: now(), ...fields }));
        return this;
    }
    /** Start a child span. */
    span(options = {}) {
        return startSpan(this.client, this.traceId, options, this.id);
    }
    /** Start a child generation. */
    generation(options = {}) {
        return startGeneration(this.client, this.traceId, options, this.id);
    }
    /** Record a child point-in-time event. */
    event(options = {}) {
        return recordEvent(this.client, this.traceId, options, this.id);
    }
    /** Attach a score to this observation. */
    score(options) {
        return this.client.score({ ...options, traceId: this.traceId, observationId: this.id });
    }
}
export class Span extends Observation {
    eventPrefix = "span";
}
export class LangfuseEvent extends Observation {
    eventPrefix = "event";
}
export class Generation extends Observation {
    eventPrefix = "generation";
    /** Close the generation with its output, token usage and optional cost. */
    end(options = {}) {
        return super.end(compact({ ...options }));
    }
}
export class Trace {
    client;
    id;
    options;
    constructor(client, id, options) {
        this.client = client;
        this.id = id;
        this.options = options;
    }
    /** Create or update the trace record. */
    update(fields = {}, initial = false) {
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
    span(options = {}) {
        return startSpan(this.client, this.id, options);
    }
    /** Start a generation under this trace. */
    generation(options = {}) {
        return startGeneration(this.client, this.id, options);
    }
    /** Record an event under this trace. */
    event(options = {}) {
        return recordEvent(this.client, this.id, options);
    }
    /** Attach a score to this trace. */
    score(options) {
        return this.client.score({ ...options, traceId: this.id });
    }
}
export class Prompt {
    name;
    version;
    type;
    labels;
    config;
    prompt;
    variables;
    constructor(payload) {
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
    compile(variables = {}) {
        if (this.type === "chat" && Array.isArray(this.prompt)) {
            return this.prompt.map((message) => ({
                ...message,
                content: substitute(message.content ?? "", variables),
            }));
        }
        return substitute(typeof this.prompt === "string" ? this.prompt : "", variables);
    }
}
function substitute(template, variables) {
    return Object.entries(variables).reduce((text, [key, value]) => text.split(`{{${key}}}`).join(String(value)), template);
}
function startSpan(client, traceId, options, parentId) {
    const span = new Span(client, traceId, options.id ?? newId(), options.name, parentId);
    client.enqueue("span-create", span["body"]({ startTime: now(), ...compact(options) }));
    return span;
}
function startGeneration(client, traceId, options, parentId) {
    const generation = new Generation(client, traceId, options.id ?? newId(), options.name, parentId);
    client.enqueue("generation-create", generation["body"]({ startTime: now(), ...compact(options) }));
    return generation;
}
function recordEvent(client, traceId, options, parentId) {
    const event = new LangfuseEvent(client, traceId, options.id ?? newId(), options.name, parentId);
    client.enqueue("event-create", event["body"]({ startTime: now(), ...compact(options) }));
    return event;
}
/**
 * Wrap a function so each call becomes a span (or generation) on a trace.
 *
 * The outermost wrapped call opens the trace; nested calls become its children.
 */
export function observe(client, fn, options = {}) {
    const label = options.name ?? fn.name ?? "anonymous";
    return async (...args) => {
        if (!client.enabled)
            return await fn(...args);
        const parent = currentObservation;
        const trace = parent ? undefined : client.trace({ name: label, input: args });
        const owner = parent ?? trace;
        const node = options.asType === "generation"
            ? owner.generation({ name: label, input: args })
            : owner.span({ name: label, input: args });
        currentObservation = node;
        try {
            const result = await fn(...args);
            node.end({ output: result });
            trace?.update({ output: result });
            return result;
        }
        catch (error) {
            node.end({ level: "ERROR", statusMessage: String(error) });
            trace?.update({ metadata: { error: String(error) } });
            throw error;
        }
        finally {
            currentObservation = parent;
        }
    };
}
// Tracks the enclosing observation so nested observe() calls nest correctly.
// This is a module-level variable rather than AsyncLocalStorage to keep the
// package runtime-agnostic; concurrent independent traces should pass their
// parent explicitly instead of relying on it.
let currentObservation;
