import asyncio
import os
import time
from contextlib import asynccontextmanager

from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel, Field
from starlette.concurrency import run_in_threadpool

from engine import EMBED_DIM, MODEL_NAME, EmbedEngine


class EmbedRequest(BaseModel):
    text: str = Field(..., min_length=1, max_length=512)


class EmbedResponse(BaseModel):
    dim: int
    vector: list
    model: str


def create_app(engine=None):
    token = os.environ.get("EMBED_TOKEN", "")
    lock = asyncio.Lock()

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        app.state.engine = engine if engine is not None else _load_engine_from_env()
        yield

    app = FastAPI(title="shutterseek-embedding", lifespan=lifespan)

    def require_auth(authorization: str):
        if token and authorization != f"Bearer {token}":
            raise HTTPException(status_code=401, detail="unauthorized")

    @app.get("/healthz")
    def healthz():
        return {"status": "ok", "model": MODEL_NAME}

    @app.post("/embed", response_model=EmbedResponse)
    async def embed(req: EmbedRequest, authorization: str = Header(default="")):
        require_auth(authorization)
        text = req.text.strip()
        if not text:
            raise HTTPException(status_code=422, detail="empty text")
        started = time.perf_counter()
        async with lock:
            vec = await run_in_threadpool(app.state.engine.embed, text)
        elapsed_ms = (time.perf_counter() - started) * 1000
        print(f"inference_ms={elapsed_ms:.1f} text_len={len(text)}")
        return EmbedResponse(
            dim=int(vec.shape[0]),
            vector=[float(x) for x in vec.tolist()],
            model=MODEL_NAME,
        )

    return app


def _load_engine_from_env():
    engine = EmbedEngine(
        model_dir=os.environ["MODEL_DIR"],
        model_file=os.environ.get("MODEL_FILE", "model.onnx"),
        ort_threads=int(os.environ.get("ORT_THREADS", "3")),
    )
    engine.load()
    return engine


app = create_app()
