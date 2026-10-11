import asyncio
import base64
import sys
import unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from browser import Browser
from concurrency import FairGate

PNG="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR4nGP4DwQACfsD/fteaysAAAAASUVORK5CYII="


class BrowserTests(unittest.IsolatedAsyncioTestCase):
    async def test_short_page_dedup_and_real_png(self):
        result=await Browser(FairGate(1)).render(
            '<h1>Story</h1><img src="ss-photo:1">',{"1":PNG})
        self.assertEqual(len(result["screenshots"]),2)
        self.assertEqual({x["metadata"]["viewport"] for x in result["screenshots"]},
                         {"desktop","mobile"})
        for shot in result["screenshots"]:
            self.assertEqual(shot["data"][:8],b"\x89PNG\r\n\x1a\n")
        self.assertEqual(result["diagnostics"],[])

    async def test_long_page_coverage_and_interaction_cap(self):
        result=await Browser(FairGate(1)).render(
            '<style>body{margin:0}main{height:3000px}</style>'
            '<button id="show" onclick="document.getElementById(\'extra\').hidden=false">Show</button>'
            '<p id="extra" hidden>Interactive content</p><main>Travel</main>',{},
            [{"selector":"#show","expect_visible":"#extra"}])
        self.assertLessEqual(len(result["screenshots"]),8)
        for viewport in ("desktop","mobile"):
            coverage=[x for x in result["coverage"] if x["viewport"]==viewport]
            offsets=[x["y"] for x in coverage if x["kind"]=="page"]
            self.assertEqual(len(offsets),3)
            self.assertEqual(offsets[0],0)
            self.assertGreater(offsets[-1],offsets[1])
            self.assertTrue(any(x["kind"]=="interaction" for x in coverage))
        self.assertEqual(result["diagnostics"],[])

    async def test_mobile_overflow_missing_image_and_broken_interaction(self):
        result=await Browser(FairGate(1)).render(
            '<div style="width:900px">Overflow</div><img src="data:image/png;base64,broken">'
            '<button id="show">Show</button><p id="extra" hidden>Unavailable</p>',{},
            [{"selector":"#show","expect_visible":"#extra"}])
        descriptions=[x["description"] for x in result["diagnostics"]]
        self.assertIn("页面出现横向溢出",descriptions)
        self.assertIn("部分图片缺失或无法显示",descriptions)
        self.assertTrue(any("交互操作未能正常完成" in x for x in descriptions))
        self.assertTrue(all(x["severity"]=="blocking" for x in result["diagnostics"]))

    async def test_sandbox_blocks_host_session_and_network(self):
        result=await Browser(FairGate(1)).render("""
          <p id="isolated" hidden>Isolated</p>
          <button id="test" onclick="test()">Test isolation</button>
          <script>
          async function test() {
            let blocked = 0;
            try { parent.document.cookie; } catch(e) { blocked++; }
            try { localStorage.getItem('shutterseek_token'); } catch(e) { blocked++; }
            try { await fetch('https://example.invalid/api'); } catch(e) { blocked++; }
            if (blocked === 3) document.getElementById('isolated').hidden = false;
          }
          </script>""",{},[{"selector":"#test","expect_visible":"#isolated"}])
        # CSP console errors are expected from the deliberate network attempt.
        self.assertFalse(any("交互操作未能正常完成" in x["description"]
                             for x in result["diagnostics"]))


    async def test_generated_page_cannot_navigate_its_frame_to_network(self):
        result=await Browser(FairGate(1)).render("""
            <button id="nav" onclick="location.href='https://example.invalid/leak'">Navigate</button>
            <p id="retained">Original page</p>
        """,{},[{"selector":"#nav","expect_visible":"#retained"}])
        # Chromium replaces the frame with its internal blocked-navigation
        # page. No request reaches the route handler or the network.
        self.assertFalse(any(x["description"]=="页面尝试加载外部资源，已被阻止"
                             for x in result["diagnostics"]))
        self.assertTrue(any("交互操作未能正常完成" in x["description"]
                            for x in result["diagnostics"]))

    async def test_nested_frame_cannot_replace_the_inspected_document(self):
        result=await Browser(FairGate(1)).render(
            '<main style="height:3000px"><h1>Original page</h1><iframe srcdoc="Nested page"></iframe></main>',{})
        self.assertEqual(len(result["screenshots"]),6)
        self.assertTrue(any(x["description"]=="页面包含不允许的嵌套内容"
                            for x in result["diagnostics"]))
        for region in result["coverage"]:
            self.assertGreaterEqual(region["page_height"],3000)

    async def test_timeout_frees_browser_for_next_user(self):
        gate=FairGate(1)
        browser=Browser(gate,viewport_timeout=.1,round_timeout=3)
        original=browser._viewport
        async def stalled(*args):
            await asyncio.sleep(10)
        browser._viewport=stalled
        result=await browser.render("<h1>test</h1>",{})
        self.assertTrue(any("超时" in x["description"] for x in result["diagnostics"]))
        self.assertEqual(gate.active,0)
        next_result=await Browser(gate).render("<h1>next user</h1>",{})
        self.assertEqual(len(next_result["screenshots"]),2)


if __name__=="__main__":
    unittest.main()
