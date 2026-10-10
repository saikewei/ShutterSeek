FROM python:3.12-slim
WORKDIR /app
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/playwright
COPY requirements.txt requirements.lock .
RUN pip install --no-cache-dir -r requirements.txt && playwright install --with-deps chromium --only-shell
RUN useradd --create-home --uid 10001 agent
COPY . .
USER agent
CMD ["python", "server.py"]
