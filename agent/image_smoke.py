"""Exercise packaged browser assets as the production non-root user."""
import asyncio
from pathlib import Path
from browser import Browser
from concurrency import FairGate
from model import CreationModel


async def main():
    if not Path("skills/photo-web-design/SKILL.md").is_file():
        raise RuntimeError("packaged design skill unavailable")
    result = await Browser(FairGate(1)).render(
        "<html><head><meta name='viewport' content='width=device-width'></head>"
        "<body><h1>容器浏览器验收</h1><button onclick='this.textContent=1'>查看</button></body></html>", {})
    if result["diagnostics"]:
        raise RuntimeError("packaged browser failed rendering checks")
    viewports = {shot["metadata"]["viewport"] for shot in result["screenshots"]}
    if viewports != {"desktop", "mobile"}:
        raise RuntimeError("packaged browser screenshot coverage incomplete")
    print("Sandbox rendering passed for desktop and mobile")


if __name__ == "__main__":
    asyncio.run(main())
