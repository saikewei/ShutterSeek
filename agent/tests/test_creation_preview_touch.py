"""Touch regression coverage for the production editor and full preview."""
import asyncio
import html
import json
import mimetypes
import sys
import unittest
from pathlib import Path
from urllib.parse import urlparse
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from browser import CSP, prepare_html
from playwright.async_api import async_playwright

DIST=Path(__file__).resolve().parents[2]/"frontend"/"dist"
SOURCE="""<style>body{margin:0}button{min-height:44px}#pad{height:70px;background:#ddd;touch-action:none}main{height:2400px}</style>
<button id="open" onclick="document.getElementById('extra').hidden=false">展开</button><p id="extra" hidden>可交互内容</p>
<div id="pad" role="slider" aria-label="照片位置"></div><p id="dragged" hidden>照片已移动</p><main>长页面</main>
<script>
window.previewState={width:innerWidth,height:innerHeight,nonce:Math.random(),pointer:null};
const width=innerWidth, pad=document.getElementById('pad');let start=null;
pad.addEventListener('pointerdown',e=>{start=e.clientX;pad.setPointerCapture(e.pointerId);window.previewState.pointer=e.pointerType});
pad.addEventListener('pointermove',e=>{if(start!==null){const position=(e.clientX-start)/width;if(Number.isFinite(position)&&position>.1)document.getElementById('dragged').hidden=false}});
pad.addEventListener('pointerup',()=>start=null);
</script>"""


@unittest.skipUnless((DIST/"index.html").exists(),"build frontend first")
class CreationPreviewTouchTests(unittest.IsolatedAsyncioTestCase):
    async def swipe(self,page,x,start_y,end_y):
        session=await page.context.new_cdp_session(page)
        try:
            await session.send("Input.dispatchTouchEvent",{"type":"touchStart","touchPoints":[{"x":x,"y":start_y}]})
            for step in range(1,9):
                await session.send("Input.dispatchTouchEvent",{"type":"touchMove","touchPoints":[{"x":x,"y":start_y+(end_y-start_y)*step/8}]})
                await asyncio.sleep(.025)
            await session.send("Input.dispatchTouchEvent",{"type":"touchEnd","touchPoints":[]})
        finally:await session.detach()

    async def drag(self,page,bounds):
        session=await page.context.new_cdp_session(page)
        x=bounds["x"]+20;y=bounds["y"]+bounds["height"]/2
        try:
            await session.send("Input.dispatchTouchEvent",{"type":"touchStart","touchPoints":[{"x":x,"y":y}]})
            for step in range(1,7):
                await session.send("Input.dispatchTouchEvent",{"type":"touchMove","touchPoints":[{"x":x+step*20,"y":y}]})
                await asyncio.sleep(.025)
            await session.send("Input.dispatchTouchEvent",{"type":"touchEnd","touchPoints":[]})
        finally:await session.detach()

    async def exercise(self,start_width):
        async with async_playwright() as pw:
            browser=await pw.chromium.launch(headless=True,args=["--disable-dev-shm-usage"])
            context=await browser.new_context(viewport={"width":start_width,"height":844},is_mobile=True,has_touch=True)
            page=await context.new_page();page.set_default_timeout(8000)
            preview_requests=[]
            detail={"creation":{"id":"work","title":"照片作品","current_version":"v1"},"photo_ids":[1],"state":{},"messages":[],"runs":[],
                    "versions":[{"id":"v1","verified":False,"used_photo_ids":"[1]","report":"{}","created_at":"2026-10-11T08:00:00Z"}]}
            async def route_handler(route):
                path=urlparse(route.request.url).path;data=None
                if path=="/api/v1/auth/me":data={"id":1,"role":"admin","username":"test"}
                elif path=="/api/v1/creations/capabilities":data={"enabled":True}
                elif path=="/api/v1/creations/work":data=detail
                elif path.endswith("/preview"):
                    preview_requests.append(route.request.url)
                    content=prepare_html(SOURCE,{})
                    wrapper='<meta name="viewport" content="width=device-width,initial-scale=1"><style>html,body{margin:0;height:100%;overflow:hidden}iframe{width:100%;height:100%;border:0;display:block}</style><iframe title="作品内容" sandbox="allow-scripts" srcdoc="'+html.escape(content,quote=True)+'"></iframe>'
                    await route.fulfill(content_type="text/html; charset=utf-8",headers={"Content-Security-Policy":CSP.replace("frame-src 'none'","frame-src about:")+"; sandbox allow-scripts"},body=wrapper);return
                elif path.startswith("/api/"):data=[]
                if data is not None:await route.fulfill(content_type="application/json",body=json.dumps(data));return
                file=(DIST/path.lstrip("/")).resolve() if path!="/" else DIST/"index.html"
                if file.is_file() and DIST.resolve() in file.parents:
                    await route.fulfill(content_type=mimetypes.guess_type(str(file))[0] or "application/octet-stream",body=file.read_bytes())
                else:await route.abort()
            await context.route("http://preview-touch.test/**",route_handler)
            await page.goto("http://preview-touch.test/#/creations/work")
            await page.get_by_role("heading",name="照片作品",exact=True).wait_for()
            if start_width>767:
                await page.locator('iframe[title="作品沙箱预览"]').wait_for()
                await page.set_viewport_size({"width":390,"height":844})
            await page.get_by_role("button",name="预览",exact=True).wait_for()
            outer=page.locator('iframe[title="作品沙箱预览"]')
            await outer.wait_for(state="detached")
            if start_width<768:self.assertEqual(preview_requests,[])
            await page.get_by_role("button",name="预览",exact=True).tap()
            await outer.wait_for()
            frame=page.frame_locator('iframe[title="作品沙箱预览"]').frame_locator('iframe[title="作品内容"]')
            await frame.locator("#open").wait_for()
            inner=next(f for f in page.frames if f.url=="about:srcdoc")
            state=await inner.evaluate("window.previewState")
            self.assertGreater(state["width"],300);self.assertGreater(state["height"],200)
            await frame.get_by_role("button",name="展开",exact=True).tap()
            await frame.get_by_text("可交互内容",exact=True).wait_for()
            await self.drag(page,await frame.locator("#pad").bounding_box())
            await frame.get_by_text("照片已移动",exact=True).wait_for()
            self.assertEqual(await inner.evaluate("window.previewState.pointer"),"touch")
            header=page.get_by_role("heading",name="照片作品",exact=True);header_before=await header.bounding_box()
            bounds=await outer.bounding_box()
            await self.swipe(page,bounds["x"]+bounds["width"]/2,bounds["y"]+bounds["height"]-35,bounds["y"]+100)
            await inner.wait_for_function("scrollY>0")
            self.assertEqual(await page.evaluate("document.scrollingElement.scrollTop"),0)
            self.assertAlmostEqual((await header.bounding_box())["y"],header_before["y"],delta=1)
            await page.get_by_role("button",name="对话",exact=True).tap()
            await page.get_by_role("button",name="预览",exact=True).tap()
            self.assertEqual(await inner.evaluate("window.previewState.nonce"),state["nonce"])
            await page.goto("http://preview-touch.test/#/creations/work/preview")
            complete=page.frame_locator('iframe[title="完整作品预览"]').frame_locator('iframe[title="作品内容"]')
            await complete.get_by_role("button",name="展开",exact=True).tap()
            await complete.get_by_text("可交互内容",exact=True).wait_for()
            await self.drag(page,await complete.locator("#pad").bounding_box())
            await complete.get_by_text("照片已移动",exact=True).wait_for()
            await browser.close()

    async def test_saved_mobile_work_initializes_only_when_preview_is_visible(self):
        await self.exercise(390)

    async def test_chrome_device_emulation_reinitializes_preview_for_mobile(self):
        await self.exercise(1440)
