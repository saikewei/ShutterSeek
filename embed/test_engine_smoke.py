import os

import numpy as np
import pytest

from engine import EMBED_DIM, DEFAULT_SMOKE_TEXT, EmbedEngine

MODEL_DIR = os.environ.get("MODEL_DIR", os.path.join(os.path.dirname(__file__), "..", "models"))


@pytest.fixture(scope="module")
def engine():
    eng = EmbedEngine(os.path.abspath(MODEL_DIR))
    eng.load()
    return eng


def test_smoke_tokens(engine):
    ids = engine.smoke_test(DEFAULT_SMOKE_TEXT)
    assert len(ids) > 0


def test_embed_dim_and_norm(engine):
    vec = engine.embed("海边日落")
    assert vec.shape == (EMBED_DIM,)
    assert abs(float(np.linalg.norm(vec)) - 1.0) < 1e-4


def test_embed_deterministic(engine):
    a = engine.embed("海边日落")
    b = engine.embed("海边日落")
    assert np.array_equal(a, b)
