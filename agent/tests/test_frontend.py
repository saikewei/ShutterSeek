"""Production-build UI acceptance with mocked APIs; never touches the real app."""
import base64
import json
import html
import mimetypes
import sys
import unittest
from pathlib import Path
from urllib.parse import urlparse
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from browser import CSP,prepare_html
from test_browser import PNG
from playwright.async_api import async_playwright

DIST=Path(__file__).resolve().parents[2]/"frontend"/"dist"


@unittest.skipUnless((DIST/"index.html").exists(),"build frontend before browser UI acceptance")
class FrontendTests(unittest.IsolatedAsyncioTestCase):
    async def exercise(self,width,height,touch=False):
        async with async_playwright() as pw:
            browser=await pw.chromium.launch(headless=True,args=["--disable-dev-shm-usage"])
            context=await browser.new_context(viewport={"width":width,"height":height},has_touch=touch)
            page=await context.new_page()
            requested=[];submitted=[];current="";run_state=None
            def version(id):
                return {"id":id,"verified":id=="v2","report":json.dumps({
                    "rounds":1,"issues":[],"summary":"Inspected both viewports","screenshots":[]}),
                    "used_photo_ids":"[1]","created_at":"2026-10-11T08:00:00Z"}
            def detail():
                return {"creation":{"id":"work","title":"旅行故事","current_version":current},
                        "photo_ids":[1,2],"state":{"proposal":{"structure":"A journey",
                        "style":"Minimal","used_photo_ids":[1,2]},"questions":["在哪里？"],
                        "unknowns":["地点"],"observations":[],"user_facts":[]},
                        "messages":[],"runs":([{"id":"run","status":run_state,"report":"{}"}] if run_state else []),
                        "versions":([version("v2"),version("v1")] if current else [])}
            async def route_handler(route):
                nonlocal current,run_state
                request=route.request;path=urlparse(request.url).path
                requested.append((request.method,path))
                value=None
                if path=="/api/v1/auth/me":value={"id":1,"username":"test-admin","role":"admin"}
                elif path=="/api/v1/creations/capabilities":value={"enabled":True,"max_photos":200}
                elif path=="/api/v1/creations/work":value=detail()
                elif path=="/api/v1/creations/work/runs":
                    submitted.append(request.post_data_json)
                    run_state="queued";value={"id":"run","status":"queued","report":"{}"}
                elif path.endswith("/events"):
                    current="v2";run_state="completed"
                    await route.fulfill(status=200,content_type="text/event-stream",
                        body='event: progress\ndata: {"stage":"reviewing"}\n\nevent: done\ndata: {"status":"completed"}\n\n')
                    return
                elif path.endswith("/restore"):
                    current=path.split("/")[-2]
                    submitted.append(request.post_data_json)
                    await route.fulfill(status=204);return
                elif path.endswith("/preview"):
                    await route.fulfill(content_type="text/html",headers={
                        "Content-Security-Policy":CSP.replace("frame-src 'none'","frame-src about:")+"; sandbox allow-scripts"},
                        body='<!doctype html><style>html,body{margin:0;height:100%;overflow:hidden}</style><iframe title="Generated content" sandbox="allow-scripts" style="width:100%;height:100%;border:0" srcdoc="'+html.escape(prepare_html('<h1>Saved story</h1><button id="open" onclick="document.getElementById(\'extra\').hidden=false">Open</button><p id="extra" hidden>Interactive content</p>',{}),quote=True)+'"></iframe>')
                    return
                elif path.startswith("/api/v1/thumbnails/"):
                    await route.fulfill(content_type="image/png",body=base64.b64decode(PNG.split(",")[1]));return
                elif path.startswith("/api/"):
                    await route.fulfill(status=404,content_type="application/json",body='{"error":"unmocked route"}');return
                if value is not None:
                    await route.fulfill(content_type="application/json",body=json.dumps(value,ensure_ascii=False));return
                file=(DIST/path.lstrip("/")).resolve() if path!="/" else DIST/"index.html"
                if DIST.resolve() not in file.parents:
                    await route.abort();return
                if file.is_file():
                    await route.fulfill(content_type=mimetypes.guess_type(str(file))[0] or "application/octet-stream",body=file.read_bytes())
                else:
                    await route.fulfill(status=404,body="missing fixture asset")
            await page.route("http://creation-ui.test/**",route_handler)
            await page.goto("http://creation-ui.test/#/creations/work")
            await page.get_by_role("button",name="确认并生成").wait_for()
            await page.get_by_label("内容结构",exact=True).fill("A confirmed journey")
            await page.get_by_label("使用照片 2",exact=True).uncheck()
            await page.get_by_role("button",name="确认并生成").click()
            await page.wait_for_function("document.querySelector('iframe') !== null")
            if width<768 or touch:
                await page.get_by_role("button",name="预览",exact=True).click()
            frame=page.frame_locator('iframe[title="作品沙箱预览"]').frame_locator('iframe[title="Generated content"]')
            await frame.get_by_role("heading",name="Saved story").wait_for()
            await frame.get_by_role("button",name="Open",exact=True).click()
            await frame.get_by_text("Interactive content").wait_for(state="visible")
            self.assertEqual(submitted[0]["action"],"confirm")
            self.assertEqual(submitted[0]["proposal"]["used_photo_ids"],[1])
            self.assertEqual(submitted[0]["proposal"]["structure"],"A confirmed journey")
            self.assertEqual(await page.locator("iframe").get_attribute("sandbox"),"allow-scripts")
            await page.get_by_role("combobox").select_option("v1")
            await page.get_by_role("button",name="恢复此版本").click()
            self.assertEqual(submitted[-1]["base_version"],"v2")
            self.assertTrue(any(path.endswith("/v1/restore") for method,path in requested))
            artifact=Path("tmp/creation-ui");artifact.mkdir(parents=True,exist_ok=True)
            await page.screenshot(path=str(artifact/("tablet.png" if touch else "mobile.png" if width<768 else "desktop.png")))
            overflow=await page.evaluate("document.documentElement.scrollWidth > innerWidth")
            self.assertFalse(overflow,"creation UI overflows horizontally")
            if width<768 or touch:
                preview_width=await page.locator("iframe").bounding_box()
                self.assertGreater(preview_width["width"],width*.8)
            await browser.close()

    async def test_desktop_confirm_interact_restore(self):
        await self.exercise(1440,900)

    async def test_tablet_coarse_pointer_uses_full_width_mobile_panel(self):
        await self.exercise(900,1000,touch=True)

    async def test_mobile_panel_confirm_interact_restore(self):
        await self.exercise(390,844)
