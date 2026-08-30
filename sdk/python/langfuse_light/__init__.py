"""Langfuse Light — lightweight LLM observability client.

    from langfuse_light import Langfuse

    lf = Langfuse(public_key="pk-lf-...", secret_key="sk-lf-...", host="http://localhost:3001")

    trace = lf.trace(name="qa", user_id="u1", input=question)
    with trace.generation(name="answer", model="gpt-4o", input=question) as gen:
        reply = call_model(question)
        gen.end(output=reply, usage={"input": 120, "output": 45})
    trace.update(output=reply)
    trace.score(name="helpfulness", value=0.9)
    lf.flush()
"""

from .client import (
    Event,
    Generation,
    Langfuse,
    LangfuseError,
    Prompt,
    Span,
    Trace,
    configure,
    get_client,
    observe,
)

__version__ = "0.1.0"

__all__ = [
    "Langfuse",
    "LangfuseError",
    "Trace",
    "Span",
    "Generation",
    "Event",
    "Prompt",
    "observe",
    "configure",
    "get_client",
    "__version__",
]
