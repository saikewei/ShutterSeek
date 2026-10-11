"""Authenticated sharing UI acceptance with fake APIs and no production writes."""
import html
import json
import mimetypes
import sys
import unittest
from pathlib import Path
from urllib.parse import urlparse
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from browser import CSP,prepare_html
from playwright.async_api import async_playwright

DIST=Path(__file__).resolve().parents[2]/"frontend"/"dist"
SHARE="11111111-1111-4111-8111-111111111111"


@unittest.skipUnless((DIST/"index.html").exists(),"build frontend first")
class CreationSharingUITests(unittest.IsolatedAsyncioTestCase):
    async def exercise(self,owner=False,forbidden=False,role="guest"):
        async with async_playwright() as pw:
            browser=await pw.chromium.launch(headless=True,args=["--disable-dev-shm-usage"])
            context=await browser.new_context(viewport={"width":390,"height":844},is_mobile=True,has_touch=True)
            page=await context.new_page();page.set_default_timeout(8000)
            logged=owner;share=None;calls=[];writes=[]
            async def route_handler(route):
                nonlocal logged,share
                request=route.request;path=urlparse(request.url).path;data=None
                calls.append((request.method,path))
                if path=="/api/v1/auth/me":
                    if not logged:
                        await route.fulfill(status=401,content_type="application/json",body='{"error":"未登录"}');return
                    data={"id":7,"username":"test-user","role":role}
                elif path=="/api/v1/auth/login":
                    logged=True;data={"id":7,"username":"test-user","role":role}
                elif path=="/api/v1/creations/capabilities":data={"enabled":True,"sharing_enabled":True,"max_photos":200}
                elif path=="/api/v1/creations/work":
                    data={"creation":{"id":"work","title":"分享测试作品","current_version":"v1"},"photo_ids":[1],"state":{},"messages":[],"runs":[],
                          "versions":[{"id":"v1","verified":True,"report":"{}","used_photo_ids":"[1]","created_at":"2026-10-11T08:00:00Z"}]}
                elif path=="/api/v1/creations/work/share":
                    if request.method=="POST":
                        writes.append(request.post_data_json)
                        self.assertEqual(writes[-1],{"version_id":"v1","base_version":"v1"})
                        share={"id":SHARE,"version_id":"v1","enabled":True}
                    elif request.method=="DELETE":
                        writes.append("revoke")
                        if share:share["enabled"]=False
                        await route.fulfill(status=204);return
                    data=share
                    await route.fulfill(content_type="application/json",body=json.dumps(data));return
                elif path=="/api/v1/creations/shared/"+SHARE:
                    if not logged:
                        await route.fulfill(status=401,content_type="application/json",body='{"error":"未登录"}');return
                    if forbidden:
                        await route.fulfill(status=403,content_type="application/json",body='{"error":"你无权访问作品中的照片"}');return
                    if owner and (not share or not share["enabled"]):
                        await route.fulfill(status=404,content_type="application/json",body='{"error":"作品或资源不存在"}');return
                    data={"id":SHARE,"title":"分享测试作品","version_id":"v1","verified":True,"created_at":"2026-10-11T08:00:00Z"}
                elif path.endswith("/preview"):
                    content=prepare_html('<button id="open" onclick="document.getElementById(\'detail\').hidden=false">展开作品</button><p id="detail" hidden>登录后可查看的作品内容</p>',{})
                    wrapper='<meta name="viewport" content="width=device-width,initial-scale=1"><style>html,body{margin:0;height:100%;overflow:hidden}iframe{width:100%;height:100%;border:0}</style><iframe title="作品内容" sandbox="allow-scripts" srcdoc="'+html.escape(content,quote=True)+'"></iframe>'
                    await route.fulfill(content_type="text/html; charset=utf-8",headers={"Content-Security-Policy":CSP.replace("frame-src 'none'","frame-src about:")+"; sandbox allow-scripts"},body=wrapper);return
                elif path.startswith("/api/"):data=[]
                if data is not None:
                    await route.fulfill(content_type="application/json",body=json.dumps(data));return
                file=(DIST/path.lstrip("/")).resolve() if path!="/" else DIST/"index.html"
                if file.is_file() and DIST.resolve() in file.parents:
                    await route.fulfill(content_type=mimetypes.guess_type(str(file))[0] or "application/octet-stream",body=file.read_bytes())
                else:await route.abort()
            await context.route("http://sharing-ui.test/**",route_handler)
            target="http://sharing-ui.test/#/creations/shared/"+SHARE
            if not owner:
                await page.goto(target)
                await page.get_by_placeholder("用户名",exact=True).wait_for()
                self.assertIn("#/login?redirect=",page.url)
                self.assertFalse(any("/creations/shared/" in path for method,path in calls),"anonymous share fetched content before login")
                await page.get_by_placeholder("用户名",exact=True).fill("test-user")
                await page.get_by_placeholder("密码",exact=True).fill("test-only-passphrase")
                await page.locator('button[type="submit"]').click()
            else:
                await page.goto("http://sharing-ui.test/#/creations/work")
                await page.get_by_role("button",name="分享",exact=True).tap()
                modal=page.get_by_role("dialog",name="分享作品",exact=True)
                await modal.get_by_role("button",name="开启登录后分享",exact=True).tap()
                link=modal.get_by_role("textbox",name="分享链接",exact=True)
                await link.wait_for()
                self.assertEqual(await link.input_value(),target)
                await modal.get_by_role("button",name="复制链接",exact=True).tap()
                await modal.get_by_role("button",name="已复制",exact=True).wait_for()
                await modal.get_by_role("button",name="关闭",exact=True).tap()
                await modal.wait_for(state="detached")
                await page.goto(target)
            if forbidden:
                await page.get_by_role("alert").get_by_text("你无权访问作品中的照片",exact=True).wait_for()
                self.assertEqual(await page.locator("iframe").count(),0)
                self.assertEqual(await page.get_by_role("heading",name="分享测试作品",exact=True).count(),0)
            else:
                await page.get_by_role("heading",name="分享测试作品",exact=True).wait_for()
                self.assertEqual(await page.get_by_role("log").count(),0)
                self.assertEqual(await page.get_by_role("button",name="修改作品名称",exact=True).count(),0)
                self.assertEqual(await page.get_by_role("button",name="还原作品",exact=True).count(),0)
                self.assertEqual(await page.locator("nav").count(),0)
                frame=page.frame_locator('iframe[title="分享作品预览"]').frame_locator('iframe[title="作品内容"]')
                await frame.get_by_role("button",name="展开作品",exact=True).tap()
                await frame.get_by_text("登录后可查看的作品内容",exact=True).wait_for()
                self.assertEqual(await page.locator("iframe").get_attribute("sandbox"),"allow-scripts")
            if owner:
                await page.goto("http://sharing-ui.test/#/creations/work")
                await page.get_by_role("button",name="分享",exact=True).tap()
                await modal.get_by_role("button",name="关闭分享",exact=True).tap()
                await modal.get_by_role("button",name="开启登录后分享",exact=True).wait_for()
                await modal.get_by_role("button",name="关闭",exact=True).tap()
                await page.goto(target)
                await page.get_by_role("alert").get_by_text("作品或资源不存在",exact=True).wait_for()
                self.assertEqual(await page.locator("iframe").count(),0)
                self.assertEqual(writes,[{"version_id":"v1","base_version":"v1"},"revoke"])
            else:self.assertEqual(writes,[])
            await browser.close()

    async def test_guest_logs_in_and_returns_to_readonly_share(self):
        await self.exercise()

    async def test_guest_cannot_view_shared_private_photos(self):
        await self.exercise(forbidden=True)

    async def test_guest_owner_enables_copies_and_revokes_share(self):
        await self.exercise(owner=True)

    async def test_admin_owner_enables_copies_and_revokes_share(self):
        await self.exercise(owner=True,role="admin")
