"""Render the same opaque-origin iframe and network policy as the user preview."""
import asyncio
import html as html_module
import re
import time

CSP = ("default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; "
       "img-src data:; font-src data:; connect-src 'none'; media-src 'none'; "
       "object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'")
VIEWPORTS = [("desktop", 1440, 900), ("mobile", 390, 844)]


def prepare_html(source, assets):
    budget=len(source.encode())
    for match in re.finditer(r"ss-photo:([0-9]+)",source):
        budget+=len(assets.get(match[1],""))-len(match[0])
        if budget>64<<20:
            raise ValueError("expanded page exceeds the 64 MiB asset budget")
    source = re.sub(r"ss-photo:([0-9]+)", lambda m: assets.get(m[1], ""), source)
    if len(source.encode()) > 64 << 20:
        raise ValueError("expanded page exceeds the 64 MiB asset budget")
    meta='<meta http-equiv="Content-Security-Policy" content="'+CSP+'">'
    if not re.match(r"\s*<!doctype\s+html",source,re.I):
        source="<!doctype html>"+source
    head=re.search(r"<head(?:\s[^>]*)?>",source,re.I)
    if head:
        return source[:head.end()]+meta+source[head.end():]
    root=re.search(r"<html(?:\s[^>]*)?>",source,re.I)
    if root:
        return source[:root.end()]+"<head>"+meta+"</head>"+source[root.end():]
    doctype=re.search(r"<!doctype[^>]*>",source,re.I)
    return source[:doctype.end()]+"<head>"+meta+"</head>"+source[doctype.end():]


class Browser:
    def __init__(self, gate, viewport_timeout=15, round_timeout=30):
        self.gate = gate
        self.viewport_timeout = viewport_timeout
        self.round_timeout = round_timeout

    async def render(self, source, assets, interactions=()):
        from playwright.async_api import async_playwright
        screenshots, diagnostics, coverage = [], [], []
        async with self.gate.slot() as queued:
            started = time.monotonic()
            pw = None
            browser = None
            try:
                async with asyncio.timeout(self.round_timeout):
                    pw = await async_playwright().start()
                    browser = await pw.chromium.launch(
                        headless=True, args=["--disable-dev-shm-usage"], timeout=10000)
                    for name, width, height in VIEWPORTS:
                        try:
                            async with asyncio.timeout(self.viewport_timeout):
                                context = await browser.new_context(
                                    viewport={"width": width, "height": height},
                                    reduced_motion="reduce", service_workers="block")
                                try:
                                    result = await self._viewport(
                                        context, source, assets, name, width, height,
                                        interactions, 8-len(screenshots))
                                    screenshots.extend(result[0])
                                    diagnostics.extend(result[1])
                                    coverage.extend(result[2])
                                finally:
                                    await context.close()
                        except TimeoutError:
                            diagnostics.append(self.timeout_issue(name))
                    await browser.close()
                    browser = None
                    await pw.stop()
                    pw = None
            except TimeoutError:
                diagnostics.append(self.timeout_issue("mobile"))
            finally:
                # Killing the Playwright transport tears down its owned Chromium
                # processes, even when untrusted script prevents graceful close.
                if pw is not None:
                    process = pw._impl_obj._connection._transport._proc
                    if process is not None and process.returncode is None:
                        process.kill()
                    try:
                        await asyncio.wait_for(pw.stop(), 1)
                    except Exception:
                        pass
        return {"screenshots": screenshots, "diagnostics": diagnostics,
                "coverage": coverage, "browser_queue_seconds": queued,
                "browser_seconds": time.monotonic()-started}

    @staticmethod
    def timeout_issue(viewport):
        return {"severity": "blocking", "viewport": viewport,
                "region": "all", "element": "document",
                "suggestion": "减少页面渲染工作量",
                "description": "页面效果检查超时"}

    async def _viewport(self, context, source, assets, name, width, height,
                        interactions, remaining):
        images, issues, coverage = [], [], []
        async def deny(route):
            issues.append({"severity": "blocking", "viewport": name, "region": "network",
                           "element": route.request.resource_type,
                           "description": "页面尝试加载外部资源，已被阻止",
                           "suggestion": "请使用作品内的照片和内嵌资源"})
            await route.abort()
        await context.route("**/*", deny)
        page = await context.new_page()
        page.on("pageerror", lambda error: issues.append({
            "severity": "blocking", "viewport": name, "region": "interaction",
            "element": "script", "description": "页面脚本运行出错",
            "technical_detail": str(error)[:500],
            "suggestion": "修复页面脚本错误"}))
        content = prepare_html(source, assets)
        await page.set_content('<meta http-equiv="Content-Security-Policy" content="'+CSP.replace("frame-src 'none'","frame-src about:")+'"><iframe sandbox="allow-scripts" style="border:0;width:100%;height:100vh" '
                               'srcdoc="' + html_module.escape(content, quote=True) +
                               '"></iframe><style>html,body{margin:0;overflow:hidden}</style>')
        handle=await page.locator("iframe").first.element_handle()
        frame=await handle.content_frame()
        if frame is None:
            raise RuntimeError("generated frame is unavailable")
        await frame.wait_for_load_state("domcontentloaded")
        await frame.evaluate("""async () => {
          await Promise.race([Promise.all([
            document.fonts.ready,
            ...Array.from(document.images).map(img => img.complete ?
              Promise.resolve() : new Promise(resolve => {
                img.addEventListener('load', resolve, {once:true});
                img.addEventListener('error', resolve, {once:true});
              }))
          ]), new Promise(r => setTimeout(r, 2000))]);
          document.getAnimations().forEach(a => a.pause());
          document.documentElement.style.scrollBehavior='auto';
          if(document.body) document.body.style.scrollBehavior='auto';
        }""")
        measurements = await frame.evaluate("""() => ({
          height: Math.max(document.body?.scrollHeight || 0, document.documentElement.scrollHeight),
          overflow: document.documentElement.scrollWidth > innerWidth + 2,
          missing: Array.from(document.images).filter(i => {
            const rect=i.getBoundingClientRect();
            return rect.bottom>0 && rect.top<innerHeight && (!i.complete || !i.naturalWidth);
          }).map(i => i.getAttribute('src')?.slice(0,80))
        })""")
        if await frame.locator("iframe,object,embed").count():
            issues.append({"severity":"blocking","viewport":name,"region":"all",
                           "element":"iframe,object,embed",
                           "description":"页面包含不允许的嵌套内容",
                           "suggestion":"改用普通页面元素"})
        if not frame.url.startswith("about:srcdoc"):
            issues.append({"severity":"blocking","viewport":name,"region":"all",
                           "element":"document","description":"页面尝试跳转，已被阻止",
                           "suggestion":"让交互在当前作品内完成"})
        if measurements["overflow"]:
            issues.append({"severity":"blocking","viewport":name,"region":"all",
                           "element":"document","description":"页面出现横向溢出",
                           "suggestion":"调整宽度以适配屏幕"})
        if measurements["missing"]:
            issues.append({"severity":"blocking","viewport":name,"region":"images",
                           "element":"img","description":"部分图片缺失或无法显示",
                           "suggestion":"使用当前作品中的有效照片"})
        maximum = max(0, measurements["height"]-height)
        offsets = list(dict.fromkeys([0, maximum//2, maximum]))
        for y in offsets:
            if len(images) >= remaining:
                break
            await frame.evaluate("(y) => scrollTo(0,y)", y)
            await frame.evaluate("""async () => {
              await Promise.race([
                Promise.all(Array.from(document.images).filter(img => {
                  const rect=img.getBoundingClientRect();
                  return rect.bottom > 0 && rect.top < innerHeight;
                }).map(img => img.complete ? Promise.resolve() : new Promise(resolve => {
                  img.addEventListener('load',resolve,{once:true});
                  img.addEventListener('error',resolve,{once:true});
                }))),
                new Promise(resolve => setTimeout(resolve,1000))
              ]);
              document.getAnimations().forEach(a => a.pause());
            }""")
            await page.wait_for_timeout(80)
            missing=await frame.evaluate("""() => Array.from(document.images).some(img => {
              const rect=img.getBoundingClientRect();
              return rect.bottom>0 && rect.top<innerHeight && (!img.complete || !img.naturalWidth);
            })""")
            if missing:
                issues.append({"severity":"blocking","viewport":name,
                               "region":"y="+str(y),"element":"img",
                               "description":"部分图片缺失或无法显示",
                               "suggestion":"修复此区域的图片"})
            region = {"viewport":name,"x":0,"y":y,"width":width,"height":height,
                      "kind":"page","page_height":measurements["height"]}
            images.append({"data":await page.screenshot(type="png", animations="disabled"),
                           "metadata":region})
            coverage.append(region)
        # At most two additional screenshots across the whole round.
        for item in list(interactions)[:1]:
            if len(images) >= remaining:
                break
            selector = item.get("selector", "")[:300]
            if not selector:
                continue
            try:
                await frame.locator(selector).first.click(timeout=1000)
                expected = item.get("expect_visible")
                if expected:
                    try:
                        await frame.locator(expected[:300]).first.wait_for(state="visible",timeout=1000)
                    except Exception as error:
                        raise ValueError("操作后的预期内容未显示") from error
            except Exception as error:
                issues.append({"severity":"blocking","viewport":name,"region":"interaction",
                               "element":selector,"description":"交互操作未能正常完成",
                               "technical_detail":str(error)[:500],
                               "suggestion":"修复此交互操作"})
            position=await frame.evaluate("() => ({x:scrollX,y:scrollY})")
            region = {"viewport":name,"kind":"interaction","selector":selector,
                      **position,"width":width,"height":height}
            if not frame.url.startswith("about:srcdoc"):
                issues.append({"severity":"blocking","viewport":name,"region":"interaction",
                               "element":selector,"description":"页面尝试跳转，已被阻止",
                               "suggestion":"让交互在当前作品内完成"})
            images.append({"data":await page.screenshot(type="png", animations="disabled"),
                           "metadata":region})
            coverage.append(region)
        return images, issues, coverage
