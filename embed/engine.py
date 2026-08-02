"""ShutterSeek embedding engine — 全离线 ONNX 推理（BGE-M3 INT8）。"""
import os

import numpy as np
import onnxruntime as ort

os.environ.setdefault("HF_HUB_OFFLINE", "1")
os.environ.setdefault("TRANSFORMERS_OFFLINE", "1")

MODEL_NAME = "bge-m3-int8"
EMBED_DIM = 1024
DEFAULT_SMOKE_TEXT = "hello world"


class EmbedEngine:
    def __init__(self, model_dir, model_file="model.onnx", ort_threads=3, max_tokens=512):
        self.model_dir = model_dir
        self.model_path = os.path.join(model_dir, model_file)
        self.ort_threads = ort_threads
        self.max_tokens = max_tokens
        self.tokenizer = None
        self.tokenizer_kind = None  # "fast" | "transformers"
        self.session = None
        self.input_names = []
        self.output_name = ""

    def load(self):
        if not os.path.isfile(self.model_path):
            raise FileNotFoundError(f"model not found: {self.model_path}")
        self._load_tokenizer()
        so = ort.SessionOptions()
        so.intra_op_num_threads = self.ort_threads
        so.inter_op_num_threads = 1
        so.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        self.session = ort.InferenceSession(
            self.model_path, sess_options=so, providers=["CPUExecutionProvider"]
        )
        self.input_names = [i.name for i in self.session.get_inputs()]
        self.output_name = self.session.get_outputs()[0].name
        self._smoke_test()

    def _load_tokenizer(self):
        tokenizer_json = os.path.join(self.model_dir, "tokenizer.json")
        try:
            from tokenizers import Tokenizer

            tok = Tokenizer.from_file(tokenizer_json)
            tok.enable_truncation(max_length=self.max_tokens)
            self.tokenizer = tok
            self.tokenizer_kind = "fast"
        except Exception as fast_err:
            from transformers import AutoTokenizer

            self.tokenizer = AutoTokenizer.from_pretrained(
                self.model_dir, local_files_only=True
            )
            self.tokenizer_kind = "transformers"
            self._fast_err = str(fast_err)

    def _encode(self, text):
        if self.tokenizer_kind == "fast":
            enc = self.tokenizer.encode(text)
            return (
                np.asarray(enc.ids, dtype=np.int64),
                np.asarray(enc.attention_mask, dtype=np.int64),
            )
        enc = self.tokenizer(
            text, truncation=True, max_length=self.max_tokens, return_tensors="np"
        )
        return (
            enc["input_ids"].astype(np.int64),
            enc["attention_mask"].astype(np.int64),
        )

    def _build_feed(self, input_ids, attention_mask):
        if input_ids.ndim == 1:
            input_ids = input_ids[None, :]
            attention_mask = attention_mask[None, :]
        feed = {}
        for name in self.input_names:
            if name == "input_ids":
                feed[name] = input_ids
            elif name == "attention_mask":
                feed[name] = attention_mask
            elif "token_type" in name:
                feed[name] = np.zeros_like(input_ids)
            else:
                raise RuntimeError(f"unexpected model input: {name}")
        return feed

    def embed(self, text):
        input_ids, attention_mask = self._encode(text)
        feed = self._build_feed(input_ids, attention_mask)
        out = self.session.run([self.output_name], feed)[0]
        vec = np.asarray(out, dtype=np.float32)[0]
        if vec.shape[0] != EMBED_DIM:
            raise ValueError(f"unexpected dim {vec.shape[0]}, want {EMBED_DIM}")
        norm = float(np.linalg.norm(vec))
        if norm > 0:
            vec = vec / norm
        return vec

    def _smoke_test(self):
        input_ids, _ = self._encode(DEFAULT_SMOKE_TEXT)
        if input_ids.size == 0:
            raise RuntimeError("smoke test: empty token ids")
        vec = self.embed(DEFAULT_SMOKE_TEXT)
        if vec.shape[0] != EMBED_DIM:
            raise RuntimeError("smoke test: wrong dim")

    def smoke_test(self, text=DEFAULT_SMOKE_TEXT):
        input_ids, _ = self._encode(text)
        if input_ids.size == 0:
            raise RuntimeError("smoke test: empty token ids")
        vec = self.embed(text)
        if vec.shape[0] != EMBED_DIM:
            raise RuntimeError("smoke test: wrong dim")
        return input_ids.tolist()
