"""Mobile creation space and keyboard regressions with fake authenticated APIs."""
import json
import mimetypes
import unittest
from pathlib import Path
from urllib.parse import urlparse
from playwright.async_api import async_playwright

DIST = Path(__file__).resolve().parents[2] / "frontend" / "dist"

@unittest.skipUnless((DIST / "index.html").exists(), "build frontend first")
class MobileCreationLayoutTests(unittest.IsolatedAsyncioTestCase):
    async def exercise(self, keyboard=None, running=False, width=390, height=844):
        async with async_playwright() as pw:
            browser = await pw.chromium.launch(headless=True, args=["--disable-dev-shm-usage"])
            context = await browser.new_context(viewport={"width": width, "height": height}, is_mobile=True, has_touch=True)
            page = await context.new_page()
            page.set_default_timeout(7000)
            detail = {
                "creation": {"id": "work", "title": "海边旅行的漫长夏天", "current_version": "v1"},
                "photo_ids": [1, 2],
                "state": {},
                "messages": [{"id": str(i), "role": "assistant" if i % 2 else "user", "content": "聊聊这组旅行照片，留住旅途中的故事。"} for i in range(20)],
                "versions": [{"id": "v1", "verified": True, "report": "{}", "used_photo_ids": "[1,2]", "created_at": "2026-10-11T08:00:00Z"}],
                "runs": [{"id": "run", "status": "running", "created_at": "2026-10-11T08:00:00Z", "report": "{}"}] if running else [],
            }
            async def serve(route):
                path = urlparse(route.request.url).path
                data = None
                if path == "/api/v1/auth/me":
                    data = {"id": 7, "username": "test-guest", "role": "guest"}
                elif path == "/api/v1/creations/capabilities":
                    data = {"enabled": True, "sharing_enabled": True, "max_photos": 200}
                elif path == "/api/v1/creations/work":
                    data = detail
                elif path.endswith("/events"):
                    await route.fulfill(content_type="text/event-stream", body='retry: 10000\n\nevent: progress\ndata: {"stage":"generating"}\n\n')
                    return
                elif path.endswith("/preview"):
                    await route.fulfill(content_type="text/html", body="<h1>旅行作品</h1>")
                    return
                elif path.startswith("/api/"):
                    await route.fulfill(status=404, content_type="application/json", body='{"error":"测试资源不存在"}')
                    return
                if data is not None:
                    await route.fulfill(content_type="application/json", body=json.dumps(data))
                    return
                file = (DIST / path.lstrip("/")).resolve() if path != "/" else DIST / "index.html"
                if file.is_file() and DIST.resolve() in file.parents:
                    await route.fulfill(content_type=mimetypes.guess_type(str(file))[0] or "application/octet-stream", body=file.read_bytes())
                else:
                    await route.abort()
            await context.route("http://mobile-layout.test/**", serve)
            await page.goto("http://mobile-layout.test/#/creations/work")
            composer = page.get_by_role("textbox", name="给创作助手的消息", exact=True)
            await composer.wait_for()
            artifacts = Path("tmp/mobile-creation-layout")
            artifacts.mkdir(parents=True, exist_ok=True)
            if keyboard:
                await composer.tap()
                await page.evaluate("""(both) => {
                    const original = window.visualViewport;
                    window.__originalViewport = original;
                    window.__originalHeight = Object.getOwnPropertyDescriptor(window, 'innerHeight');
                    Object.defineProperty(window, 'visualViewport', {configurable: true, value: new Proxy(original, {
                        get(target, key) {
                            if (key === 'height') return 360;
                            if (key === 'offsetTop') return 0;
                            const value = Reflect.get(target, key, target);
                            return typeof value === 'function' ? value.bind(target) : value;
                        }
                    })});
                    if (both) Object.defineProperty(window, 'innerHeight', {configurable: true, value: 360});
                    window.dispatchEvent(new Event('resize'));
                }""", keyboard == "both")
                await page.wait_for_timeout(150)
                await page.screenshot(path=str(artifacts / ("keyboard-" + keyboard + ".png")))
                self.assertFalse(await page.get_by_role("navigation", name="主导航", exact=True).is_visible(), "bottom navigation rises above the keyboard")
                box = await composer.bounding_box()
                self.assertGreaterEqual(box["y"], 0)
                self.assertLessEqual(box["y"] + box["height"], 360)
                self.assertGreater((await page.get_by_role("log").bounding_box())["height"], 190)
                await page.evaluate("""() => {
                    Object.defineProperty(window, 'visualViewport', {configurable: true, value: window.__originalViewport});
                    if (window.__originalHeight) Object.defineProperty(window, 'innerHeight', window.__originalHeight);
                    window.dispatchEvent(new Event('resize'));
                }""")
                await page.get_by_role("navigation", name="主导航", exact=True).wait_for(state="visible")
                await page.get_by_role("button", name="更多作品操作", exact=True).wait_for(state="visible")
                self.assertTrue(await composer.evaluate("el => el === document.activeElement"), "keyboard dismissal should not require losing input focus")
                await composer.evaluate("el => el.blur()")
            else:
                header = await page.locator(".creation-page > header").bounding_box()
                self.assertLessEqual(header["height"], 48)
                self.assertGreater((await page.get_by_role("log").bounding_box())["height"], height * (0.55 if running or height < 700 else 0.6))
                self.assertFalse(await page.locator(".conversation-heading").is_visible())
                self.assertFalse(await page.get_by_role("button", name="还原作品", exact=True).is_visible())
                await page.get_by_role("button", name="更多作品操作", exact=True).tap()
                menu = page.get_by_role("dialog", name="作品操作", exact=True)
                await menu.get_by_role("button", name="版本记录", exact=True).tap()
                await page.get_by_role("dialog", name="版本记录", exact=True).get_by_role("heading", name="版本记录", exact=True).wait_for()
                await page.get_by_role("button", name="返回预览", exact=True).tap()
                if running:
                    progress = page.locator(".creation-progress")
                    self.assertLessEqual((await progress.bounding_box())["height"], 72)
                    await page.get_by_role("button", name="展开生成进度", exact=True).tap()
                    await progress.get_by_text("关闭页面不会取消任务", exact=False).wait_for()
                    await page.get_by_role("button", name="收起生成进度", exact=True).tap()
                    self.assertLessEqual((await progress.bounding_box())["height"], 72)
                await page.screenshot(path=str(artifacts / ("compact-" + str(width) + ("-running" if running else "") + ".png")))
                self.assertFalse(await page.evaluate("document.documentElement.scrollWidth > innerWidth"))
                await page.get_by_role("button", name="预览", exact=True).tap()
                self.assertGreater((await page.locator('iframe[title="作品沙箱预览"]').bounding_box())["height"], height * 0.6)
            await browser.close()

    async def test_keyboard_resizes_both_viewports(self):
        await self.exercise(keyboard="both")

    async def test_keyboard_resizes_visual_viewport(self):
        await self.exercise(keyboard="visual")

    async def test_compact_saved_work(self):
        await self.exercise()

    async def test_compact_running_progress(self):
        await self.exercise(running=True)

    async def test_compact_small_phone(self):
        await self.exercise(width=360, height=640)
