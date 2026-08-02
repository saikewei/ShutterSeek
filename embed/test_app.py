import numpy as np
from fastapi.testclient import TestClient

from app import create_app


class StubEngine:
    def __init__(self, fail=False):
        self.fail = fail
        self.calls = []

    def embed(self, text):
        self.calls.append(text)
        if self.fail:
            raise RuntimeError("boom")
        return np.full(1024, 0.03125, dtype=np.float32)  # L2 norm = 1


def test_healthz():
    with TestClient(create_app(StubEngine())) as client:
        r = client.get("/healthz")
        assert r.status_code == 200
        assert r.json()["status"] == "ok"


def test_embed_requires_token(monkeypatch):
    monkeypatch.setenv("EMBED_TOKEN", "s3cret")
    with TestClient(create_app(StubEngine())) as client:
        r = client.post("/embed", json={"text": "海边"})
        assert r.status_code == 401
        r2 = client.post(
            "/embed", json={"text": "海边"},
            headers={"Authorization": "Bearer s3cret"},
        )
        assert r2.status_code == 200


def test_embed_empty_text():
    with TestClient(create_app(StubEngine())) as client:
        r = client.post("/embed", json={"text": "   "})
        assert r.status_code == 422


def test_embed_success_shape():
    with TestClient(create_app(StubEngine())) as client:
        r = client.post("/embed", json={"text": "海边"})
        assert r.status_code == 200
        body = r.json()
        assert body["dim"] == 1024
        assert len(body["vector"]) == 1024
        assert body["model"] == "bge-m3-int8"
