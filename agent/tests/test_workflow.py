import asyncio
import copy
import json
import sys
import unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from concurrency import FairGate
from workflow import Workflow, CandidateError
from checkpoint import clean, CheckpointMaterialError

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
                      "state":{"proposal":{"structure":"照片故事","style":"简约",
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
                result={"observations":[{"photo_id":x,"observation":"画面中可见风景",
                                          "unknowns":["地点"]} for x in data["photo_ids"]]}
            elif "Translate these display texts" in instruction:
                result={"translations":[{"id":x["id"],"text":"中文描述，保留原有事实和不确定信息。"} for x in data["texts"]]}
            elif "Propose a content structure" in instruction:
                result={"proposal":{"structure":"照片故事","style":"简约",
                                    "used_photo_ids":[x["photo_id"] for x in data["observations"]]},
                        "reply":"我已根据照片整理好方案。","questions":["这是在哪里拍的？"],"unknowns":["地点"]}
            elif "visually inspect EVERY" in instruction:
                self.reviews+=1
                assert len(images)==2
                assert all(x.startswith("data:image/png;base64,") for x in images)
                result={"issues":([{"severity":"blocking","viewport":"mobile",
                                     "region":"首屏","element":"h1","description":"文字重叠",
                                     "suggestion":"调整布局"}]
                                   if self.reviews<=self.blocking_reviews else []),
                        "summary":"已检查桌面和手机截图"}
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

    async def test_generation_corrects_changed_confirmed_photos_before_render(self):
        workspace = MemoryWorkspace(1)
        workspace.context["photo_ids"] = [1, 2]
        model = FakeModel(FairGate(5))
        original = model.invoke
        calls = []
        async def generate(instruction, data, images=()):
            if "Produce a complete responsive" in instruction and "Repair the HTML" not in instruction:
                calls.append(data)
                self.assertEqual(data["locked_photo_ids"], [1])
                if len(calls) == 1:
                    return {"html":'<img src="ss-photo:2">',"used_photo_ids":[2]}, {}
                self.assertEqual(data["validation_error"], "generation changed confirmed photo selection")
            return await original(instruction, data, images)
        model.invoke = generate
        browser = FakeBrowser(FairGate(1))
        events = []
        async def capture(event): events.append(event)
        await Workflow(workspace, model, browser, capture).execute()
        self.assertEqual(len(calls), 2)
        self.assertEqual(browser.renders, 1)
        self.assertEqual(workspace.finished["used_photo_ids"], [1])
        self.assertTrue(workspace.finished["verified"])
        self.assertIn("correcting", [event["stage"] for event in events])

    async def test_publication_brief_keeps_observations_as_reference_and_omits_unknown_metadata(self):
        workspace=MemoryWorkspace(1)
        state={**workspace.context["state"],**workspace.context}
        state["proposal"]=workspace.context["state"]["proposal"]
        state["observations"]=[{"photo_id":1,"observation":"海岸上可见礁石和海浪","unknowns":["地点未确认"],"analysis":"不要发布这条分析"}]
        state["unknowns"]=["拍摄时间未确认"]
        state["user_facts"]=[{"message_id":"user","text":"照片拍于海边","display_text":"照片拍于海边"}]
        state["messages"]=[{"id":"user","role":"user","content":"想做一个以照片为主的旅行展示"},
                           {"id":"assistant","role":"assistant","content":"地点未确认，需要补充信息"}]
        captured=[]
        model=FakeModel(FairGate(5))
        original=model.invoke
        async def inspect(instruction,data,images=()):
            captured.append(data)
            return await original(instruction,data,images)
        model.invoke=inspect
        result=await Workflow(workspace,model,FakeBrowser(FairGate(1)),emit).generate(state)
        data=captured[0]
        self.assertEqual(data["visual_context"],[{"photo_id":1,"visual_summary":"海岸上可见礁石和海浪"}])
        self.assertEqual(data["copy_policy"]["visual_context"],"reference_only")
        self.assertEqual(data["copy_policy"]["uncertain_facts"],"omit")
        self.assertEqual(data["editorial_facts"],[{"message_id":"user","text":"照片拍于海边"}])
        self.assertEqual(data["user_requests"],[{"id":"user","content":"想做一个以照片为主的旅行展示"}])
        self.assertNotIn("unknowns",data)
        self.assertNotIn("messages",data)
        self.assertNotIn("地点未确认",json.dumps(data,ensure_ascii=False))
        self.assertEqual(result["used_photo_ids"],[1])

    async def test_natural_language_interaction_expectation_is_corrected_before_browser_check(self):
        workspace=MemoryWorkspace(1)
        model=FakeModel(FairGate(5))
        original=model.invoke
        generated=[]
        async def invoke(instruction,data,images=()):
            result,metric=await original(instruction,data,images)
            if "Produce a complete responsive" in instruction and "Repair the HTML" not in instruction:
                generated.append(data)
                result["html"]+='<button id="show">展开</button><p id="detail" hidden>说明</p>'
                result["interactions"]=[{"selector":"#show",
                    "expect_visible":"点击后说明展开且按钮文字变为收起" if len(generated)==1 else "#detail"}]
            return result,metric
        model.invoke=invoke
        browser=FakeBrowser(FairGate(1))
        events=[]
        async def capture(event):events.append(event)
        await Workflow(workspace,model,browser,capture).execute()
        self.assertEqual(len(generated),2)
        self.assertIn("never prose",generated[1]["validation_error"])
        self.assertEqual(browser.renders,1)
        self.assertEqual(workspace.saved["checkpoint"]["channel_values"]["__root__"]["interactions"],
                         [{"selector":"#show","expect_visible":"#detail"}])
        self.assertTrue(workspace.finished["verified"])
        self.assertIn("correcting",[x["stage"] for x in events])

    async def test_invalid_interaction_contract_correction_is_bounded(self):
        workspace=MemoryWorkspace(1)
        model=FakeModel(FairGate(5))
        calls=0
        async def invoke(*args,**kwargs):
            nonlocal calls
            calls+=1
            return {"html":'<img src="ss-photo:1"><h1 id="title">标题</h1>',
                    "used_photo_ids":[1],
                    "interactions":[{"selector":"#title","expect_visible":"标题完整可见"}]},{}
        model.invoke=invoke
        browser=FakeBrowser(FairGate(1))
        with self.assertRaises(CandidateError) as error:
            await Workflow(workspace,model,browser,emit).execute()
        self.assertEqual(error.exception.code,"interaction_manifest_invalid")
        self.assertEqual(calls,2)
        self.assertEqual(browser.renders,0)
        self.assertIsNone(workspace.finished)

    async def test_forbidden_generated_material_is_corrected_before_checkpoint_write(self):
        for material,reason in [
            ('<img src="data:image/svg+xml;base64,private-image">',"candidate_embedded_image"),
            ('<script>const credential="private-key"</script>',"candidate_restricted_material"),
        ]:
            with self.subTest(reason=reason):
                workspace=MemoryWorkspace(1)
                model=FakeModel(FairGate(5))
                original=model.invoke
                calls=[]
                async def invoke(instruction,data,images=()):
                    result,metric=await original(instruction,data,images)
                    if "Produce a complete responsive" in instruction:
                        calls.append(data)
                        if len(calls)==1:result["html"]+=material
                    return result,metric
                model.invoke=invoke
                browser=FakeBrowser(FairGate(1))
                await Workflow(workspace,model,browser,emit).execute()
                self.assertEqual(len(calls),2)
                self.assertIn("validation_error",calls[1])
                self.assertEqual(browser.renders,1)
                self.assertTrue(workspace.finished["verified"])
                self.assertNotIn("private-",json.dumps(workspace.saved))

    async def test_unsafe_candidate_correction_is_bounded_without_rendering_or_checkpointing_bytes(self):
        workspace=MemoryWorkspace(1)
        model=FakeModel(FairGate(5))
        calls=0
        async def invoke(*args,**kwargs):
            nonlocal calls
            calls+=1
            return {"html":'<img src="ss-photo:1"><img src="data:image/png;base64,private-image">',
                    "used_photo_ids":[1],"interactions":[]},{}
        model.invoke=invoke
        browser=FakeBrowser(FairGate(1))
        with self.assertRaises(CandidateError) as error:
            await Workflow(workspace,model,browser,emit).execute()
        self.assertEqual(error.exception.code,"candidate_embedded_image")
        self.assertEqual(calls,2)
        self.assertEqual(browser.renders,0)
        self.assertNotIn("private-image",json.dumps(workspace.saved))
        self.assertIsNone(workspace.finished)

    async def test_visual_repair_also_corrects_forbidden_material_without_extra_visual_round(self):
        workspace=MemoryWorkspace(1)
        model=FakeModel(FairGate(5),blocking_reviews=1)
        original=model.invoke
        repairs=[]
        async def invoke(instruction,data,images=()):
            result,metric=await original(instruction,data,images)
            if "Repair the HTML" in instruction:
                repairs.append(data)
                if len(repairs)==1:result["html"]+='<img src="data:image/svg+xml;base64,private-image">'
            return result,metric
        model.invoke=invoke
        browser=FakeBrowser(FairGate(1))
        await Workflow(workspace,model,browser,emit).execute()
        self.assertEqual(len(repairs),2)
        self.assertIn("validation_error",repairs[1])
        self.assertEqual(browser.renders,2)
        self.assertEqual(workspace.finished["report"]["rounds"],2)
        self.assertTrue(workspace.finished["verified"])
        self.assertNotIn("private-image",json.dumps(workspace.saved))

    async def test_generation_contract_correction_is_bounded_and_never_renders_bad_selection(self):
        workspace = MemoryWorkspace(1)
        workspace.context["photo_ids"] = [1, 2]
        model = FakeModel(FairGate(5))
        calls = 0
        async def invalid(*args, **kwargs):
            nonlocal calls
            calls += 1
            return {"html":'<img src="ss-photo:2">',"used_photo_ids":[2]}, {}
        model.invoke = invalid
        browser = FakeBrowser(FairGate(1))
        with self.assertRaises(CandidateError) as error:
            await Workflow(workspace, model, browser, emit).execute()
        self.assertEqual(error.exception.code, "confirmed_photos_changed")
        self.assertEqual(calls, 2)
        self.assertEqual(browser.renders, 0)
        self.assertIsNone(workspace.finished)

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

    async def test_chat_asks_one_question_and_stops_after_three(self):
        workspace=MemoryWorkspace(1,"answer")
        model=FakeModel(FairGate(5))
        original=model.invoke
        budgets=[]
        async def converse(instruction,data,images=()):
            result,metric=await original(instruction,data,images)
            if "Propose a content structure" in instruction:
                budgets.append(data["question_budget"])
                result["questions"]=["想补充第"+str(len(budgets))+"个细节吗？","多余的问题不应展示"]
                result["reply"]="我已经记下你刚才的回答。"
            return result,metric
        model.invoke=converse
        for turn in range(4):
            workspace.saved=None
            workspace.context["messages"].append({"id":str(turn),"role":"user","content":"这是一段新的补充"})
            await Workflow(workspace,model,FakeBrowser(FairGate(1)),emit).execute()
            state=workspace.finished["state"]
            self.assertLessEqual(len(state["questions"]),1)
            workspace.context["state"]=state
        self.assertEqual(budgets,[3,2,1,0])
        self.assertEqual(state["questions"],[])
        self.assertEqual(state["question_count"],3)
        self.assertEqual(len(state["asked_questions"]),3)

    async def test_chat_does_not_hide_extra_questions_inside_reply(self):
        workspace=MemoryWorkspace(1,"answer")
        model=FakeModel(FairGate(5))
        original=model.invoke
        async def bundled(instruction,data,images=()):
            result,metric=await original(instruction,data,images)
            if "Propose a content structure" in instruction:
                result["reply"]="已经整理好方案。在哪里拍的？什么时候拍的？"
                result["questions"]=["你喜欢什么风格？要不要配音乐？"]
            return result,metric
        model.invoke=bundled
        await Workflow(workspace,model,FakeBrowser(FairGate(1)),emit).execute()
        state=workspace.finished["state"]
        self.assertEqual(state["reply"],"已经整理好方案。")
        self.assertEqual(state["questions"],["你喜欢什么风格？"])

    async def test_localization_preserves_user_messages_ids_and_versions(self):
        workspace=MemoryWorkspace(1,"localize")
        workspace.context["state"]["proposal"]["structure"]="A journey"
        workspace.context["state"]["observations"]=[{"photo_id":1,"observation":"A landscape, location unknown","unknowns":["location"]}]
        workspace.context["messages"]=[{"id":"fact","role":"user","content":"Bali in 2022"}]
        workspace.context["html"]="<h1>Previously saved page</h1>"
        original=copy.deepcopy(workspace.context)
        events=[]
        async def capture(event): events.append(event)
        await Workflow(workspace,FakeModel(FairGate(5)),FakeBrowser(FairGate(1)),capture).execute()
        state=workspace.finished["state"]
        self.assertIn("中文",state["observations"][0]["observation"])
        self.assertEqual(state["observations"][0]["photo_id"],1)
        self.assertEqual(state["proposal"]["used_photo_ids"],[1])
        self.assertEqual(state["user_facts"][0]["message_id"],"fact")
        self.assertEqual(state["user_facts"][0]["text"],"Bali in 2022")
        self.assertIn("中文",state["user_facts"][0]["display_text"])
        self.assertEqual(workspace.context,original)
        self.assertEqual(workspace.finished["html"],"")
        self.assertIn("localizing",[x["stage"] for x in events])
        self.assertFalse(any(x["stage"]=="generating" for x in events))

    async def test_phase_progress_is_emitted_before_model_waits(self):
        workspace=MemoryWorkspace(1,"answer")
        events=[]
        async def capture(event):events.append(event)
        model=FakeModel(FairGate(5))
        original=model.invoke
        async def check(instruction,data,images=()):
            expected="observing" if "Observe these photos" in instruction else "planning"
            self.assertEqual(events[-1]["stage"],expected)
            self.assertEqual(events[-1]["status"],"started")
            return await original(instruction,data,images)
        model.invoke=check
        await Workflow(workspace,model,FakeBrowser(FairGate(1)),capture).execute()
        self.assertEqual(events[-1]["stage"],"waiting")

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
        self.assertEqual(len(workspace.finished["state"]["questions"]),1)


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
        with self.assertRaises(CheckpointMaterialError) as error:
            clean({"image":PHOTO})
        self.assertEqual(error.exception.code,"checkpoint_image_material")
        self.assertNotIn(PHOTO,str(error.exception))
        with self.assertRaises(CheckpointMaterialError) as error:
            clean({"credential":"private-key"})
        self.assertEqual(error.exception.code,"checkpoint_credential_material")
        self.assertNotIn("private-key",str(error.exception))


if __name__=="__main__":
    unittest.main()
