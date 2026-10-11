import hashlib
import json
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


    async def test_failed_photo_interaction_reaches_visual_repair_without_checkpoint_failure(self):
        workspace=RealPhotoWorkspace(1)
        workspace.context["input"]["action"]="modify"
        model=FakeModel(FairGate(5))
        original=model.invoke
        reviewed=[]
        async def invoke(instruction,data,images=()):
            if "visually inspect EVERY" in instruction:
                reviewed.append(data)
                self.assertTrue(images)
                self.assertTrue(all(x.startswith("data:image/png;base64,") for x in images))
                return {"issues":[],"summary":"已检查桌面和手机截图"},{}
            result,metric=await original(instruction,data,images)
            if "html" in result and "Repair the HTML" not in instruction:
                result["html"]='<img id="photo" src="ss-photo:1"><div style="position:fixed;inset:0;z-index:10">遮挡层</div>'
                result["interactions"]=[{"selector":"#photo"}]
            return result,metric
        model.invoke=invoke
        await Workflow(workspace,model,Browser(FairGate(1)),emit).execute()
        self.assertTrue(workspace.finished["verified"])
        self.assertEqual(workspace.finished["report"]["rounds"],2)
        self.assertEqual(len(reviewed),2)
        self.assertTrue(all(issue["operation"] == ("tap" if issue["viewport"] == "mobile" else "click") for issue in reviewed[0]["diagnostics"]))
        self.assertEqual(reviewed[1]["diagnostics"],[])
        self.assertNotIn("data:image/",json.dumps(workspace.saved))
        self.assertEqual(len(workspace.assets),6)
