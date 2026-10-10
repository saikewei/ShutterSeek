import hashlib
import sys
import unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from browser import Browser
from concurrency import FairGate
from workflow import Workflow
from test_workflow import MemoryWorkspace,FakeModel,emit
from test_browser import PNG


class RealPhotoWorkspace(MemoryWorkspace):
    async def call(self,op,payload=None):
        if op=="photo":
            if payload["id"]!=self.photo:raise PermissionError()
            return {"data":PNG}
        return await super().call(op,payload)


class OverlapModel(FakeModel):
    async def invoke(self,instruction,data,images=()):
        result,metric=await super().invoke(instruction,data,images)
        if "html" in result and self.reviews==0:
            result["html"]='<style>h1{position:absolute;top:0}.copy{width:850px}</style><h1>Title</h1><p class="copy">Overlapping text</p><img src="ss-photo:1">'
        return result,metric


class VisualLoopTests(unittest.IsolatedAsyncioTestCase):
    async def test_real_screenshots_reach_review_and_repair_is_rendered_again(self):
        workspace=RealPhotoWorkspace(1)
        model=OverlapModel(FairGate(5),blocking_reviews=1)
        await Workflow(workspace,model,Browser(FairGate(1)),emit).execute()
        self.assertTrue(workspace.finished["verified"])
        self.assertEqual(model.reviews,2)
        self.assertEqual(workspace.finished["report"]["rounds"],2)
        self.assertEqual(len(workspace.assets),4)
        self.assertNotEqual(workspace.assets["0"]["data"],workspace.assets["2"]["data"])
        self.assertEqual([x["id"] for x in workspace.finished["report"]["screenshots"]],["2","3"])
