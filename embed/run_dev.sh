#!/usr/bin/env bash
# 一键启动 ShutterSeek embedding sidecar（开发环境）
#
# 用法:
#   ./embed/run_dev.sh            # 默认 127.0.0.1:8000
#   ./embed/run_dev.sh 8001       # 指定端口
#
# 环境变量（可选）:
#   MODEL_DIR    模型目录，默认 <仓库根>/models
#   EMBED_TOKEN  Bearer 令牌，默认空（开发免鉴权）
#   ORT_THREADS  推理线程数，默认 3
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

MODEL_DIR="${MODEL_DIR:-$REPO_ROOT/models}"
PORT="${1:-${PORT:-8000}}"
VENV="$SCRIPT_DIR/.venv"
PY="$VENV/bin/python"
PIP="$VENV/bin/pip"
STAMP="$VENV/.deps_stamp"

# 1. 虚拟环境（不存在则创建）
if [ ! -x "$PY" ]; then
  echo "→ 创建 venv: $VENV"
  python3 -m venv "$VENV"
fi

# 2. 依赖（requirements.txt 有更新或首次运行则安装）
if [ ! -f "$STAMP" ] || [ "$SCRIPT_DIR/requirements.txt" -nt "$STAMP" ]; then
  echo "→ 安装 Python 依赖..."
  "$PIP" install --quiet -r "$SCRIPT_DIR/requirements.txt"
  touch "$STAMP"
fi

# 3. 校验模型文件
if [ ! -f "$MODEL_DIR/model.onnx" ]; then
  echo "✗ 未找到模型: $MODEL_DIR/model.onnx" >&2
  echo "  请将 model.onnx 及分词文件放入 $MODEL_DIR" >&2
  exit 1
fi

# 4. 启动 sidecar
echo "→ MODEL_DIR=$MODEL_DIR"
echo "→ ORT_THREADS=${ORT_THREADS:-3}  EMBED_TOKEN=${EMBED_TOKEN:+<已设置>}"
echo "→ 启动 uvicorn 于 127.0.0.1:$PORT (workers=1, 串行推理)"
export MODEL_DIR
export HF_HUB_OFFLINE=1
export TRANSFORMERS_OFFLINE=1
exec "$VENV/bin/uvicorn" app:app \
  --app-dir "$SCRIPT_DIR" \
  --host 127.0.0.1 \
  --port "$PORT" \
  --workers 1
