import asyncio
import copy
import json
import sys
import unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from concurrency import FairGate
from workflow import Workflow
from checkpoint import clean

PHOTO="data:image/webp;base64,UklGRg=="
SHOT=b"fake screenshot"


class MemoryWorkspace:
    def __init__(self, user, action="confirm"):
        self.user=user
        self.photo=user
        self.saved=None
        self.assets={}
        self.finished=None
        self.context={"creation_id":str(user),"thread_id":str(user),
                      "photo_ids":[user],"title":"test",
                      "state":{"proposal":{"structure":"story","style":"clean",
                                          "used_photo_ids":[user]},"observations":[]},
                      "input":{"action":action},"messages":[]}
    async def call(self,op,payload=None):
        if op=="context":return copy.deepcopy(self.context)
        if op=="checkpoint_get":return {"checkpoint":copy.deepcopy(self.saved)}
        if op=="checkpoint_put":
            self.saved=copy.deepcopy(payload)
            clean(self.saved)
            return {}
        if op=="photo":
            if payload["id"]!=self.photo:raise PermissionError("other user's photo")
            return {"data":PHOTO}
        if op=="asset":
            key=str(len(self.assets))
            self.assets[key]=copy.deepcopy(payload)
            return {"id":key}
        if op=="asset_get":
            if payload["id"] not in self.assets:raise PermissionError()
            return {"data":"data:image/png;base64,"+self.assets[payload["id"]]["data"]}
        if op=="finish":self.finished=copy.deepcopy(payload);return {}
        raise AssertionError(op)


class FakeModel:
    def __init__(self,gate,blocking_reviews=0):
        self.gate=gate
        self.blocking_reviews=blocking_reviews
        self.reviews=0
        self.image_batches=[]
    async def invoke(self,instruction,data,images=()):
        async with self.gate.slot():
            await asyncio.sleep(.005)
            if "Observe these photos" in instruction:
                self.image_batches.append(len(images))
                result={"observations":[{"photo_id":x,"observation":"visible landscape",
                                          "unknowns":["location"]} for x in data["photo_ids"]]}
            elif "Propose a content structure" in instruction:
                result={"proposal":{"structure":"story","style":"clean",
                                    "used_photo_ids":[x["photo_id"] for x in data["observations"]]},
                        "questions":["Where?","When?","What happened?"],"unknowns":["location"]}
            elif "visually inspect EVERY" in instruction:
                self.reviews+=1
                assert len(images)==2
                assert all(x.startswith("data:image/png;base64,") for x in images)
                result={"issues":([{"severity":"blocking","viewport":"mobile",
                                     "region":"first","element":"h1","description":"overlap",
                                     "suggestion":"fix layout"}]
                                   if self.reviews<=self.blocking_reviews else []),
                        "summary":"inspected screenshots"}
            else:
                selected=data.get("proposal",{}).get("used_photo_ids",data.get("photo_ids"))
                result={"html":"<main>"+''.join('<img src=\"ss-photo:'+str(x)+'\">' for x in selected)+"</main>",
                        "used_photo_ids":selected,"interactions":[]}
        return result,{}


class FakeBrowser:
    def __init__(self,gate):
        self.gate=gate
        self.renders=0
    async def render(self,source,assets,interactions):
        async with self.gate.slot():
            self.renders+=1
            await asyncio.sleep(.005)
            assert "ss-photo:" in source
            assert len(assets)==1
            screenshots=[{"data":SHOT,"metadata":{"viewport":v,"kind":"page","y":0}}
                         for v in ("desktop","mobile")]
        return {"screenshots":screenshots,"diagnostics":[],"coverage":["desktop","mobile"],
                "browser_queue_seconds":0,"browser_seconds":.005}


async def emit(event):
    pass


class WorkflowTests(unittest.IsolatedAsyncioTestCase):
    async def test_five_users_isolated_and_limits(self):
        models=FairGate(5);browsers=FairGate(1)
        workspaces=[MemoryWorkspace(x) for x in range(1,6)]
        tasks=[Workflow(w,FakeModel(models),FakeBrowser(browsers),emit).execute()
               for w in workspaces]
        await asyncio.gather(*tasks)
        self.assertEqual(models.peak,5)
        self.assertEqual(browsers.peak,1)
        for workspace in workspaces:
            self.assertTrue(workspace.finished["verified"])
            self.assertEqual(workspace.finished["used_photo_ids"],[workspace.user])
            self.assertEqual(workspace.saved["config"]["configurable"]["thread_id"],str(workspace.user))
            self.assertNotIn("data:image/",json.dumps(workspace.saved))

    async def test_two_repairs_then_unverified_candidate(self):
        workspace=MemoryWorkspace(1);model=FakeModel(FairGate(5),99)
        browser=FakeBrowser(FairGate(1))
        await Workflow(workspace,model,browser,emit).execute()
        self.assertEqual(browser.renders,3)
        self.assertEqual(model.reviews,3)
        self.assertFalse(workspace.finished["verified"])
        self.assertEqual(workspace.finished["report"]["rounds"],3)
        self.assertIn("html",workspace.finished)

    async def test_repair_requires_new_screenshots(self):
        workspace=MemoryWorkspace(1);model=FakeModel(FairGate(5),1)
        browser=FakeBrowser(FairGate(1))
        await Workflow(workspace,model,browser,emit).execute()
        self.assertEqual(browser.renders,2)
        self.assertTrue(workspace.finished["verified"])
        self.assertEqual(len(workspace.assets),4)

    async def test_skip_releases_execution_with_proposal(self):
        workspace=MemoryWorkspace(1,"skip")
        await Workflow(workspace,FakeModel(FairGate(5)),FakeBrowser(FairGate(1)),emit).execute()
        self.assertEqual(workspace.finished["state"]["questions"],[])
        self.assertEqual(workspace.finished["html"],"")
        self.assertIn("proposal",workspace.finished["state"])

    async def test_completed_checkpoint_replays_without_model_calls(self):
        workspace=MemoryWorkspace(1);model=FakeModel(FairGate(5))
        browser=FakeBrowser(FairGate(1))
        await Workflow(workspace,model,browser,emit).execute()
        renders=browser.renders;reviews=model.reviews
        workspace.finished=None
        await Workflow(workspace,model,browser,emit).execute()
        self.assertEqual(browser.renders,renders)
        self.assertEqual(model.reviews,reviews)
        self.assertTrue(workspace.finished["verified"])

    async def test_interrupted_observation_resumes_after_committed_batch(self):
        workspace=MemoryWorkspace(1,"answer")
        workspace.context["photo_ids"]=list(range(1,18))
        async def callback(op,payload=None):
            if op=="photo":
                if payload["id"] not in workspace.context["photo_ids"]:raise PermissionError()
                return {"data":PHOTO}
            return await MemoryWorkspace.call(workspace,op,payload)
        workspace.call=callback
        model=FakeModel(FairGate(5))
        original=model.invoke
        calls=0
        async def fail_once(instruction,data,images=()):
            nonlocal calls
            if "Observe these photos" in instruction:
                calls+=1
                if calls==2:raise RuntimeError("simulated model interruption")
            return await original(instruction,data,images)
        model.invoke=fail_once
        with self.assertRaises(RuntimeError):
            await Workflow(workspace,model,FakeBrowser(FairGate(1)),emit).execute()
        self.assertIsNotNone(workspace.saved)
        model.invoke=original
        await Workflow(workspace,model,FakeBrowser(FairGate(1)),emit).execute()
        self.assertEqual(model.image_batches,[8,8,1])
        self.assertEqual(len(workspace.finished["state"]["observations"]),17)
        self.assertEqual(len(workspace.finished["state"]["questions"]),3)


class GateTests(unittest.IsolatedAsyncioTestCase):
    async def test_fifo_and_cancelled_waiter_do_not_leak_capacity(self):
        gate=FairGate(1);order=[]
        async def task(number):
            async with gate.slot():
                order.append(number)
                await asyncio.sleep(.01)
        first=asyncio.create_task(task(0))
        await asyncio.sleep(0)
        cancelled=asyncio.create_task(task(1))
        remaining=[asyncio.create_task(task(x)) for x in range(2,6)]
        await asyncio.sleep(0)
        cancelled.cancel()
        await asyncio.gather(first,cancelled,*remaining,return_exceptions=True)
        self.assertEqual(order,[0,2,3,4,5])
        self.assertEqual(gate.active,0)

    async def test_timeout_releases_browser_slot(self):
        gate=FairGate(1)
        with self.assertRaises(TimeoutError):
            async with asyncio.timeout(.01):
                async with gate.slot():await asyncio.sleep(1)
        async with gate.slot():self.assertEqual(gate.active,1)
        self.assertEqual(gate.active,0)

    def test_reject_embedded_image_checkpoint(self):
        with self.assertRaises(ValueError):clean({"image":PHOTO})


if __name__=="__main__":
    unittest.main()
