"""Client and tracing primitives for Obsevo.

The SDK is deliberately dependency-free: it uses only the standard library, so
installing it cannot drag a transitive dependency tree into an application that
is being traced. Events are buffered in memory and flushed by a background
thread, so instrumenting a call adds no network round trip to the request path.
"""

from __future__ import annotations

import atexit
import base64
import json
import logging
import os
import threading
import time
import urllib.error
import urllib.request
import uuid
from contextlib import contextmanager
from datetime import datetime, timezone
from typing import Any, Dict, Iterator, List, Optional, Sequence

logger = logging.getLogger("obsevo")

__all__ = [
    "Langfuse",
    "Trace",
    "Span",
    "Generation",
    "get_client",
    "configure",
    "observe",
]


def _now() -> str:
    """Return the current time as an RFC 3339 timestamp in UTC."""
    return datetime.now(timezone.utc).isoformat()


def _new_id() -> str:
    return str(uuid.uuid4())


class LangfuseError(Exception):
    """Raised when the API rejects a request outright."""


class Langfuse:
    """Entry point for tracing.

    Args:
        public_key: project public key (``pk-lf-...``); falls back to
            ``LANGFUSE_PUBLIC_KEY``.
        secret_key: project secret key (``sk-lf-...``); falls back to
            ``LANGFUSE_SECRET_KEY``.
        host: base URL of the server; falls back to ``LANGFUSE_HOST``.
        flush_at: flush once this many events are buffered.
        flush_interval: flush at least this often, in seconds.
        enabled: set False to make every call a no-op, which is the clean way
            to disable tracing in tests without branching at each call site.
        timeout: HTTP timeout in seconds.
        max_retries: attempts per flush before the batch is dropped.
    """

    def __init__(
        self,
        public_key: Optional[str] = None,
        secret_key: Optional[str] = None,
        host: Optional[str] = None,
        flush_at: int = 50,
        flush_interval: float = 2.0,
        enabled: bool = True,
        timeout: float = 10.0,
        max_retries: int = 3,
    ) -> None:
        self.public_key = public_key or os.getenv("LANGFUSE_PUBLIC_KEY", "")
        self.secret_key = secret_key or os.getenv("LANGFUSE_SECRET_KEY", "")
        self.host = (host or os.getenv("LANGFUSE_HOST", "http://localhost:3001")).rstrip("/")
        self.flush_at = max(1, flush_at)
        self.flush_interval = flush_interval
        self.timeout = timeout
        self.max_retries = max_retries

        self.enabled = enabled and bool(self.public_key and self.secret_key)
        if enabled and not self.enabled:
            logger.warning(
                "Obsevo disabled: set LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY"
            )

        self._queue: List[Dict[str, Any]] = []
        self._lock = threading.Lock()
        self._flush_now = threading.Event()
        self._shutdown = threading.Event()
        self._worker: Optional[threading.Thread] = None

        if self.enabled:
            self._worker = threading.Thread(
                target=self._run, name="obsevo-flush", daemon=True
            )
            self._worker.start()
            atexit.register(self.shutdown)

    # -- public API --------------------------------------------------------

    def trace(
        self,
        *,
        id: Optional[str] = None,
        name: Optional[str] = None,
        user_id: Optional[str] = None,
        session_id: Optional[str] = None,
        input: Any = None,
        output: Any = None,
        metadata: Any = None,
        tags: Optional[Sequence[str]] = None,
        release: Optional[str] = None,
        version: Optional[str] = None,
        environment: Optional[str] = None,
    ) -> "Trace":
        """Start a trace, the root of one application request."""
        trace = Trace(
            client=self,
            id=id or _new_id(),
            name=name,
            user_id=user_id,
            session_id=session_id,
        )
        trace.update(
            input=input,
            output=output,
            metadata=metadata,
            tags=tags,
            release=release,
            version=version,
            environment=environment,
            _initial=True,
        )
        return trace

    def score(
        self,
        *,
        name: str,
        value: Any = None,
        trace_id: Optional[str] = None,
        observation_id: Optional[str] = None,
        session_id: Optional[str] = None,
        dataset_run_id: Optional[str] = None,
        comment: Optional[str] = None,
        data_type: Optional[str] = None,
        id: Optional[str] = None,
        metadata: Any = None,
    ) -> str:
        """Attach a score to a trace, an observation, a session or a run."""
        score_id = id or _new_id()
        body: Dict[str, Any] = {"id": score_id, "name": name, "value": value}
        if trace_id:
            body["traceId"] = trace_id
        if observation_id:
            body["observationId"] = observation_id
        if session_id:
            body["sessionId"] = session_id
        if dataset_run_id:
            body["datasetRunId"] = dataset_run_id
        if comment is not None:
            body["comment"] = comment
        if data_type:
            body["dataType"] = data_type
        if metadata is not None:
            body["metadata"] = metadata

        self._enqueue("score-create", body)
        return score_id

    def get_prompt(
        self,
        name: str,
        *,
        version: Optional[int] = None,
        label: Optional[str] = None,
    ) -> "Prompt":
        """Fetch a managed prompt, by version or label (default: production)."""
        params = []
        if version is not None:
            params.append(f"version={version}")
        if label:
            params.append(f"label={label}")
        query = ("?" + "&".join(params)) if params else ""

        payload = self._request("GET", f"/api/public/v2/prompts/{name}{query}")
        return Prompt(payload)

    def flush(self) -> None:
        """Send everything buffered and wait for it to be delivered."""
        if not self.enabled:
            return
        self._flush_now.set()
        # Give the worker a moment to pick the signal up, then drain inline so
        # the caller can rely on the buffer being empty when this returns.
        self._deliver(self._take_batch())

    def shutdown(self) -> None:
        """Flush and stop the background worker."""
        if not self.enabled or self._shutdown.is_set():
            return
        self._shutdown.set()
        self._flush_now.set()
        if self._worker is not None:
            self._worker.join(timeout=self.timeout + 1)
        self._deliver(self._take_batch())

    # -- internals ---------------------------------------------------------

    def _enqueue(self, event_type: str, body: Dict[str, Any]) -> None:
        if not self.enabled:
            return

        event = {
            "id": _new_id(),
            "type": event_type,
            "timestamp": _now(),
            "body": body,
        }

        with self._lock:
            self._queue.append(event)
            should_flush = len(self._queue) >= self.flush_at

        if should_flush:
            self._flush_now.set()

    def _take_batch(self) -> List[Dict[str, Any]]:
        with self._lock:
            batch, self._queue = self._queue, []
        return batch

    def _run(self) -> None:
        while not self._shutdown.is_set():
            self._flush_now.wait(timeout=self.flush_interval)
            self._flush_now.clear()
            batch = self._take_batch()
            if batch:
                self._deliver(batch)

    def _deliver(self, batch: List[Dict[str, Any]]) -> None:
        if not batch:
            return

        for attempt in range(self.max_retries):
            try:
                self._request("POST", "/api/public/ingestion", {"batch": batch})
                return
            except LangfuseError as exc:
                # A 4xx means the payload will never be accepted; retrying it
                # only delays the events behind it.
                logger.warning("Langfuse Light rejected a batch: %s", exc)
                return
            except Exception as exc:  # noqa: BLE001 - transport errors are retried
                if attempt == self.max_retries - 1:
                    logger.warning(
                        "Langfuse Light dropped %d event(s) after %d attempts: %s",
                        len(batch),
                        self.max_retries,
                        exc,
                    )
                    return
                time.sleep(2**attempt * 0.2)

    def _request(self, method: str, path: str, body: Any = None) -> Any:
        data = json.dumps(body).encode("utf-8") if body is not None else None
        credentials = base64.b64encode(
            f"{self.public_key}:{self.secret_key}".encode("utf-8")
        ).decode("ascii")

        request = urllib.request.Request(
            self.host + path,
            data=data,
            method=method,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Basic {credentials}",
                "User-Agent": "obsevo-python",
            },
        )

        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                payload = response.read()
                return json.loads(payload) if payload else None
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode("utf-8", "replace")[:500]
            if 400 <= exc.code < 500:
                raise LangfuseError(f"HTTP {exc.code}: {detail}") from exc
            raise


class _Observation:
    """Shared behaviour for spans, generations and events."""

    _event_prefix = "span"

    def __init__(
        self,
        client: Langfuse,
        trace_id: str,
        *,
        id: Optional[str] = None,
        name: Optional[str] = None,
        parent_id: Optional[str] = None,
    ) -> None:
        self._client = client
        self.trace_id = trace_id
        self.id = id or _new_id()
        self.name = name
        self.parent_id = parent_id
        self._ended = False

    def _body(self, **fields: Any) -> Dict[str, Any]:
        body: Dict[str, Any] = {"id": self.id, "traceId": self.trace_id}
        if self.name:
            body["name"] = self.name
        if self.parent_id:
            body["parentObservationId"] = self.parent_id
        body.update({k: v for k, v in fields.items() if v is not None})
        return body

    def update(self, **fields: Any) -> "_Observation":
        """Send a partial update for this observation."""
        self._client._enqueue(f"{self._event_prefix}-update", self._body(**fields))
        return self

    def end(self, **fields: Any) -> "_Observation":
        """Close the observation, recording its end time."""
        if self._ended:
            return self
        self._ended = True
        fields.setdefault("endTime", _now())
        self._client._enqueue(f"{self._event_prefix}-update", self._body(**fields))
        return self

    def span(self, **kwargs: Any) -> "Span":
        """Start a child span."""
        return _start_span(self._client, self.trace_id, parent_id=self.id, **kwargs)

    def generation(self, **kwargs: Any) -> "Generation":
        """Start a child generation."""
        return _start_generation(self._client, self.trace_id, parent_id=self.id, **kwargs)

    def event(self, **kwargs: Any) -> "Event":
        """Record a child point-in-time event."""
        return _record_event(self._client, self.trace_id, parent_id=self.id, **kwargs)

    def score(self, *, name: str, value: Any = None, **kwargs: Any) -> str:
        """Attach a score to this observation."""
        return self._client.score(
            name=name, value=value, trace_id=self.trace_id, observation_id=self.id, **kwargs
        )

    def __enter__(self) -> "_Observation":
        return self

    def __exit__(self, exc_type, exc, traceback) -> bool:
        if exc_type is not None:
            self.end(level="ERROR", statusMessage=f"{exc_type.__name__}: {exc}")
        else:
            self.end()
        return False


class Span(_Observation):
    """A unit of work with a duration: a retrieval, a tool call, an agent step."""

    _event_prefix = "span"


class Event(_Observation):
    """A point in time with no duration."""

    _event_prefix = "event"


class Generation(_Observation):
    """An LLM call, carrying model, parameters, usage and cost."""

    _event_prefix = "generation"

    def end(
        self,
        *,
        output: Any = None,
        usage: Any = None,
        cost: Optional[float] = None,
        **fields: Any,
    ) -> "Generation":
        """Close the generation with its output and token usage.

        `usage` accepts any of the shapes the server normalises:
        ``{"input": n, "output": n}``, ``{"promptTokens": n, "completionTokens": n}``
        or ``{"input_tokens": n, "output_tokens": n}``.
        """
        if output is not None:
            fields["output"] = output
        if usage is not None:
            fields["usage"] = usage
        if cost is not None:
            fields["cost"] = cost
        super().end(**fields)
        return self


class Trace:
    """The root of one application request."""

    def __init__(
        self,
        client: Langfuse,
        id: str,
        *,
        name: Optional[str] = None,
        user_id: Optional[str] = None,
        session_id: Optional[str] = None,
    ) -> None:
        self._client = client
        self.id = id
        self.name = name
        self.user_id = user_id
        self.session_id = session_id

    def update(
        self,
        *,
        input: Any = None,
        output: Any = None,
        metadata: Any = None,
        tags: Optional[Sequence[str]] = None,
        release: Optional[str] = None,
        version: Optional[str] = None,
        environment: Optional[str] = None,
        name: Optional[str] = None,
        user_id: Optional[str] = None,
        session_id: Optional[str] = None,
        _initial: bool = False,
    ) -> "Trace":
        """Create or update the trace record."""
        if name is not None:
            self.name = name
        if user_id is not None:
            self.user_id = user_id
        if session_id is not None:
            self.session_id = session_id

        body: Dict[str, Any] = {"id": self.id}
        if self.name:
            body["name"] = self.name
        if self.user_id:
            body["userId"] = self.user_id
        if self.session_id:
            body["sessionId"] = self.session_id
        if input is not None:
            body["input"] = input
        if output is not None:
            body["output"] = output
        if metadata is not None:
            body["metadata"] = metadata
        if tags:
            body["tags"] = list(tags)
        if release:
            body["release"] = release
        if version:
            body["version"] = version
        if environment:
            body["environment"] = environment
        if _initial:
            body["timestamp"] = _now()

        self._client._enqueue("trace-create" if _initial else "trace-update", body)
        return self

    def span(self, **kwargs: Any) -> Span:
        """Start a span under this trace."""
        return _start_span(self._client, self.id, **kwargs)

    def generation(self, **kwargs: Any) -> Generation:
        """Start a generation under this trace."""
        return _start_generation(self._client, self.id, **kwargs)

    def event(self, **kwargs: Any) -> Event:
        """Record an event under this trace."""
        return _record_event(self._client, self.id, **kwargs)

    def score(self, *, name: str, value: Any = None, **kwargs: Any) -> str:
        """Attach a score to this trace."""
        return self._client.score(name=name, value=value, trace_id=self.id, **kwargs)

    def __enter__(self) -> "Trace":
        return self

    def __exit__(self, exc_type, exc, traceback) -> bool:
        if exc_type is not None:
            self.update(metadata={"error": f"{exc_type.__name__}: {exc}"})
        return False


class Prompt:
    """A managed prompt fetched from the server."""

    def __init__(self, payload: Dict[str, Any]) -> None:
        self._payload = payload or {}
        self.name: str = self._payload.get("name", "")
        self.version: int = self._payload.get("version", 0)
        self.type: str = self._payload.get("type", "text")
        self.labels: List[str] = self._payload.get("labels", []) or []
        self.config: Dict[str, Any] = self._payload.get("config", {}) or {}
        self.prompt = self._payload.get("prompt")
        self.variables: List[str] = self._payload.get("variables", []) or []

    def compile(self, **variables: Any) -> Any:
        """Substitute ``{{variable}}`` placeholders.

        Returns a string for a text prompt and a list of messages for a chat
        prompt, so the result can be passed straight to a model client.
        """
        if self.type == "chat" and isinstance(self.prompt, list):
            return [
                {**message, "content": _substitute(message.get("content", ""), variables)}
                for message in self.prompt
            ]
        return _substitute(self.prompt if isinstance(self.prompt, str) else "", variables)


def _substitute(template: str, variables: Dict[str, Any]) -> str:
    result = template
    for key, value in variables.items():
        result = result.replace("{{" + key + "}}", str(value))
    return result


def _start_span(
    client: Langfuse,
    trace_id: str,
    *,
    id: Optional[str] = None,
    name: Optional[str] = None,
    parent_id: Optional[str] = None,
    input: Any = None,
    metadata: Any = None,
    **fields: Any,
) -> Span:
    span = Span(client, trace_id, id=id, name=name, parent_id=parent_id)
    client._enqueue(
        "span-create",
        span._body(startTime=_now(), input=input, metadata=metadata, **fields),
    )
    return span


def _start_generation(
    client: Langfuse,
    trace_id: str,
    *,
    id: Optional[str] = None,
    name: Optional[str] = None,
    parent_id: Optional[str] = None,
    model: Optional[str] = None,
    model_parameters: Any = None,
    input: Any = None,
    metadata: Any = None,
    prompt_name: Optional[str] = None,
    prompt_version: Optional[int] = None,
    **fields: Any,
) -> Generation:
    generation = Generation(client, trace_id, id=id, name=name, parent_id=parent_id)
    client._enqueue(
        "generation-create",
        generation._body(
            startTime=_now(),
            model=model,
            modelParameters=model_parameters,
            input=input,
            metadata=metadata,
            promptName=prompt_name,
            promptVersion=prompt_version,
            **fields,
        ),
    )
    return generation


def _record_event(
    client: Langfuse,
    trace_id: str,
    *,
    id: Optional[str] = None,
    name: Optional[str] = None,
    parent_id: Optional[str] = None,
    input: Any = None,
    output: Any = None,
    metadata: Any = None,
    level: Optional[str] = None,
    **fields: Any,
) -> Event:
    event = Event(client, trace_id, id=id, name=name, parent_id=parent_id)
    event._ended = True  # An event has no duration, so it is never closed.
    client._enqueue(
        "event-create",
        event._body(
            startTime=_now(),
            input=input,
            output=output,
            metadata=metadata,
            level=level,
            **fields,
        ),
    )
    return event


# --- module-level convenience ------------------------------------------------

_default_client: Optional[Langfuse] = None
_default_lock = threading.Lock()


def configure(**kwargs: Any) -> Langfuse:
    """Create the process-wide client used by ``observe`` and ``get_client``."""
    global _default_client
    with _default_lock:
        _default_client = Langfuse(**kwargs)
    return _default_client


def get_client() -> Langfuse:
    """Return the process-wide client, creating it from the environment."""
    global _default_client
    if _default_client is None:
        with _default_lock:
            if _default_client is None:
                _default_client = Langfuse()
    return _default_client


# Tracks the enclosing observation per thread, so nested @observe calls and
# manual spans form a correct parent/child tree without threading it by hand.
_context = threading.local()


def _current() -> Optional[Any]:
    return getattr(_context, "current", None)


@contextmanager
def _bind(node: Any) -> Iterator[None]:
    previous = _current()
    _context.current = node
    try:
        yield
    finally:
        _context.current = previous


def observe(
    _func: Any = None,
    *,
    name: Optional[str] = None,
    as_type: str = "span",
    capture_input: bool = True,
    capture_output: bool = True,
):
    """Trace a function call.

    The outermost decorated call opens a trace; nested calls become child
    observations of it. Use ``as_type="generation"`` for a function that calls
    a model.

        @observe()
        def handle(question):
            return answer(question)
    """

    def decorate(func):
        import functools

        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            client = get_client()
            if not client.enabled:
                return func(*args, **kwargs)

            label = name or func.__name__
            captured_input = _safe_input(args, kwargs) if capture_input else None

            parent = _current()
            if parent is None:
                trace = client.trace(name=label, input=captured_input)
                node: Any = (
                    trace.generation(name=label, input=captured_input)
                    if as_type == "generation"
                    else trace.span(name=label, input=captured_input)
                )
                root_trace: Optional[Trace] = trace
            else:
                node = (
                    parent.generation(name=label, input=captured_input)
                    if as_type == "generation"
                    else parent.span(name=label, input=captured_input)
                )
                root_trace = None

            try:
                with _bind(node):
                    result = func(*args, **kwargs)
            except Exception as exc:
                node.end(level="ERROR", statusMessage=f"{type(exc).__name__}: {exc}")
                if root_trace is not None:
                    root_trace.update(metadata={"error": str(exc)})
                raise

            output = _safe_value(result) if capture_output else None
            node.end(output=output)
            if root_trace is not None:
                root_trace.update(output=output)
            return result

        return wrapper

    return decorate(_func) if callable(_func) else decorate


def _safe_input(args: tuple, kwargs: dict) -> Any:
    payload: Dict[str, Any] = {}
    if args:
        payload["args"] = [_safe_value(a) for a in args]
    if kwargs:
        payload["kwargs"] = {k: _safe_value(v) for k, v in kwargs.items()}
    return payload or None


def _safe_value(value: Any) -> Any:
    """Render a value as something JSON-serialisable.

    Instrumentation must never raise because an argument was exotic, so
    anything unserialisable degrades to its repr.
    """
    try:
        json.dumps(value)
        return value
    except (TypeError, ValueError):
        return repr(value)[:2000]
