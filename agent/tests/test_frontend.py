"""Production-build UI acceptance with mocked APIs; never touches the real app."""
import asyncio
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
            creations=[];album_fetches=0;stream_attempts=0;run_report="{}"
            release_failure=asyncio.Event()
            failure_text="模型未保留你确认的选图。自动纠正未成功，请重试生成。"
            def version(id):
                return {"id":id,"verified":id=="v2","report":json.dumps({
                    "rounds":1,"issues":[],"summary":"Inspected both viewports","screenshots":[]}),
                    "used_photo_ids":"[1]","created_at":"2026-10-11T08:00:00Z"}
            def detail():
                return {"creation":{"id":"work","title":"旅行故事","current_version":current},
                        "photo_ids":[1,2],"state":{"proposal":{"structure":"A journey",
                        "style":"Minimal","used_photo_ids":[1,2]},"questions":["在哪里？"],
                        "unknowns":["地点"],"observations":[],"user_facts":[]},
                        "messages":[],"runs":([{"id":"run","status":run_state,"report":run_report}] if run_state else []),
                        "versions":([version("v2"),version("v1")] if current else [])}
            async def route_handler(route):
                nonlocal current,run_state,album_fetches,stream_attempts,run_report
                request=route.request;path=urlparse(request.url).path
                requested.append((request.method,path))
                value=None
                if path=="/api/v1/auth/me":value={"id":1,"username":"test-admin","role":"admin"}
                elif path=="/api/v1/creations/capabilities":value={"enabled":True,"max_photos":200}
                elif path=="/api/v1/creations":
                    if request.method=="POST":
                        creations.append(request.post_data_json)
                        if len(creations)==1:
                            await route.fulfill(status=500,content_type="application/json",
                                                body=json.dumps({"error":"创建作品失败，请重试"},ensure_ascii=False))
                            return
                        value={"id":"work","title":"旅行相册","current_version":""}
                    else:value=[]
                elif path=="/api/v1/albums":
                    album_fetches+=1
                    if album_fetches==1:
                        await route.fulfill(status=503,content_type="application/json",
                                            body='{"error":"unavailable"}')
                        return
                    value={"items":[
                        {"id":10,"title":"旅行相册","photo_count":2,"cover_url":"/api/v1/thumbnails/1.webp"},
                        {"id":11,"title":"空相册","photo_count":0,"cover_url":""},
                        {"id":12,"title":"大相册","photo_count":201,"cover_url":""}],
                        "total":3}
                elif path=="/api/v1/creations/work":value=detail()
                elif path=="/api/v1/creations/work/runs":
                    submitted.append(request.post_data_json)
                    run_state="queued";run_report="{}";value={"id":"run","status":"queued","report":"{}"}
                elif path.endswith("/events"):
                    stream_attempts+=1
                    if len(submitted)==1:
                        if stream_attempts==1:
                            run_state="running"
                            await route.fulfill(status=200,content_type="text/event-stream",
                                body='retry: 50\n\nevent: progress\ndata: {"stage":"generating"}\n\n')
                        else:
                            await release_failure.wait()
                            run_state="failed";run_report=json.dumps({"error":failure_text})
                            await route.fulfill(status=200,content_type="text/event-stream",
                                body='event: done\ndata: {"status":"failed"}\n\n')
                        return
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
            await page.goto("http://creation-ui.test/#/creations")
            choose=page.get_by_role("button",name="选择相册",exact=True)
            await choose.wait_for()
            if width>=768 and not touch:
                nav=page.get_by_role("link",name="AI 创作",exact=True)
                self.assertEqual(await nav.get_attribute("aria-current"),"page")
                self.assertIn("bg-accent-soft",(await nav.get_attribute("class")).split())
                self.assertEqual(await nav.locator("span").count(),1)
            await choose.click()
            dialog=page.get_by_role("dialog",name="选择创作相册",exact=True)
            await dialog.wait_for(state="attached")
            await dialog.get_by_role("heading",name="选择创作相册",exact=True).wait_for()
            artifact=Path("tmp/creation-ui");artifact.mkdir(parents=True,exist_ok=True)
            await page.screenshot(path=str(artifact/f"album-picker-{width}.png"))
            self.assertTrue(page.url.endswith("#/creations"))
            await dialog.get_by_role("alert").wait_for()
            await dialog.get_by_role("button",name="重新加载",exact=True).click()
            valid=dialog.get_by_role("button",name="选择相册：旅行相册",exact=True)
            await valid.wait_for()
            self.assertTrue(await dialog.get_by_role("button",name="选择相册：空相册",exact=True).is_disabled())
            self.assertTrue(await dialog.get_by_role("button",name="选择相册：大相册",exact=True).is_disabled())
            create=dialog.get_by_role("button",name="创建作品",exact=True)
            self.assertTrue(await create.is_disabled())
            await valid.click()
            self.assertEqual(await valid.get_attribute("aria-pressed"),"true")
            self.assertFalse(await create.is_disabled())
            await dialog.get_by_role("button",name="取消",exact=True).click()
            await dialog.wait_for(state="detached")
            self.assertEqual(creations,[])
            self.assertTrue(page.url.endswith("#/creations"))
            await choose.click()
            await valid.wait_for()
            self.assertTrue(await create.is_disabled())
            await valid.click()
            await create.click()
            await dialog.get_by_role("alert").wait_for()
            self.assertTrue(page.url.endswith("#/creations"))
            self.assertEqual(await valid.get_attribute("aria-pressed"),"true")
            await create.click()
            await page.get_by_role("button",name="确认并生成").wait_for()
            await dialog.wait_for(state="detached")
            self.assertTrue(page.url.endswith("#/creations/work"))
            self.assertEqual(creations,[{"album_id":10,"title":"旅行相册"}]*2)
            self.assertFalse(any(path.startswith("/api/v1/albums/") for method,path in requested))
            if width>=768 and not touch:
                self.assertEqual(await nav.get_attribute("aria-current"),"page")
                self.assertIn("bg-accent-soft",(await nav.get_attribute("class")).split())
            await page.get_by_label("内容结构",exact=True).fill("A confirmed journey")
            await page.get_by_label("使用照片 2",exact=True).uncheck()
            await page.get_by_role("button",name="确认并生成").click()
            status=page.get_by_role("status")
            await status.get_by_text("正在生成网页，请稍候…",exact=True).wait_for()
            self.assertTrue(await page.get_by_role("button",name="取消任务",exact=True).is_visible())
            await page.screenshot(path=str(artifact/f"generating-{width}.png"))
            release_failure.set()
            alert=page.get_by_role("alert")
            await alert.get_by_text(failure_text,exact=True).wait_for()
            self.assertEqual(await page.locator("iframe").count(),0)
            if width<768 or touch:
                self.assertEqual(await page.get_by_role("button",name="对话",exact=True).get_attribute("aria-pressed"),"true")
            await page.reload()
            await alert.get_by_text(failure_text,exact=True).wait_for()
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
            run_state="failed";run_report=json.dumps({"error":failure_text})
            await page.reload()
            await page.get_by_role("alert").get_by_text(failure_text,exact=True).wait_for()
            self.assertEqual(await page.locator('iframe[title="作品沙箱预览"]').count(),1)
            await browser.close()

    async def test_desktop_confirm_interact_restore(self):
        await self.exercise(1440,900)

    async def test_tablet_coarse_pointer_uses_full_width_mobile_panel(self):
        await self.exercise(900,1000,touch=True)

    async def test_mobile_panel_confirm_interact_restore(self):
        await self.exercise(390,844)
