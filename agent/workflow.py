"""Recoverable creation graph. No image bytes or credentials enter graph state."""
import base64
import json
import resource
from langgraph.graph import StateGraph, START, END
from checkpoint import GoCheckpointer

OBSERVE = """Observe these photos in the given order. Return
{"observations":[{"photo_id":number,"observation":string,"unknowns":[string]}]}.
Use cautious visual descriptions. No invented facts."""
PLAN = """Propose a content structure, visual style and photo selection first.
Return {"proposal":{"structure":string,"style":string,"used_photo_ids":[number]},
"questions":[string],"unknowns":[string]}. Ask at most 3 questions.
If the user skipped questions, return an empty questions list.
Do not generate HTML. Incorporate the user's explicit answers."""
GENERATE = """Return {"html":string,"used_photo_ids":[number],
"interactions":[{"selector":string,"expect_visible":string}]}.
Produce a complete responsive HTML/CSS/JavaScript page following the confirmed
proposal and user request. All photo src attributes use ss-photo:<id>.
Use no external resources. Do not invent facts. Preserve explicit user facts.
Give each important interactive control a stable CSS selector and describe
one key interaction to inspect. Include useful alt text. Keep HTML under 512 KiB."""
REVIEW = """You must visually inspect EVERY supplied screenshot at its original
detail. The screenshot metadata maps image order to viewport and page region.
Evaluate text overlap, horizontal overflow, cropping, mobile readability,
missing photos and interaction state. Browser diagnostics also matter.
Return {"issues":[{"severity":"blocking|warning|info","viewport":"desktop|mobile",
"region":string,"element":string,"description":string,"suggestion":string}],
"summary":string}. Return no blocking issues only if the inspected regions pass.
Do not claim unseen page regions or untested interactions passed."""
REPAIR = """Repair the HTML using the visual issues and browser diagnostics.
Preserve confirmed content, user facts, photo selection and useful interactions.
""" + GENERATE


def ids(value, scope):
    if not isinstance(value, list) or not value:
        raise ValueError("photo selection is empty")
    result = [int(x) for x in value]
    if any(x not in scope for x in result) or len(result) != len(set(result)):
        raise ValueError("photo selection is outside the work scope")
    return result


def validate_issues(value):
    if not isinstance(value, list):
        raise ValueError("review must contain an issue list")
    required = {"severity", "viewport", "region", "element", "description", "suggestion"}
    for issue in value:
        if not isinstance(issue, dict) or not required <= issue.keys():
            raise ValueError("review issue schema is invalid")
        if issue["severity"] not in {"blocking", "warning", "info"}:
            raise ValueError("review severity is invalid")
        if issue["viewport"] not in {"desktop", "mobile"}:
            raise ValueError("review viewport is invalid")
    return value


class Workflow:
    def __init__(self, workspace, model, browser, emit):
        self.workspace, self.model, self.browser, self.emit = workspace, model, browser, emit

    async def _call(self, instruction, data, images=()):
        result, metric = await self.model.invoke(instruction, data, images)
        await self.emit({"stage": "model", **metric})
        return result

    async def observe(self, state):
        observations = list(state.get("observations", []))
        completed = {int(x["photo_id"]) for x in observations}
        todo = [x for x in state["photo_ids"] if x not in completed]
        # Every batch is its own graph node and checkpoint, so recovery does
        # not repeat all earlier photo-understanding calls.
        batch = todo[:8]
        if batch:
            images = [(await self.workspace.call("photo", {"id":x}))["data"] for x in batch]
            result = await self._call(OBSERVE, {"photo_ids":batch}, images)
            records = result.get("observations")
            if not isinstance(records, list) or len(records)!=len(batch) or {int(x["photo_id"]) for x in records} != set(batch):
                raise ValueError("observations do not match the current batch")
            observations.extend(records)
            await self.emit({"stage":"observing","completed":len(observations),
                             "total":len(state["photo_ids"])})
        return {**state, "observations":observations}

    async def plan(self, state):
        result = await self._call(PLAN, {
            "title":state["title"],"observations":state["observations"],
            "messages":state["messages"],"previous_proposal":state.get("proposal"),
            "skipped":state["input"]["action"]=="skip",
        })
        proposal = result.get("proposal")
        if (not isinstance(proposal,dict) or not isinstance(proposal.get("structure"),str)
                or not isinstance(proposal.get("style"),str) or not proposal["structure"].strip()
                or not proposal["style"].strip()):
            raise ValueError("proposal is incomplete")
        proposal["used_photo_ids"] = ids(proposal.get("used_photo_ids"), state["photo_ids"])
        questions = result.get("questions", [])
        if not isinstance(questions, list) or len(questions)>3 or any(not isinstance(x,str) for x in questions):
            raise ValueError("at most three text questions are allowed")
        if state["input"]["action"]=="skip":
            questions=[]
        # Explicit facts stay tied to user messages, never copied to photo metadata.
        return {**state, "proposal":proposal, "questions":questions,
                "unknowns":result.get("unknowns",[]),
                "user_facts":[{"message_id":x["id"],"text":x["content"]}
                              for x in state["messages"] if x["role"]=="user"],
                "phase":"waiting", "html":""}

    async def generate(self, state):
        result = await self._call(GENERATE, {
            "proposal":state["proposal"],"observations":state["observations"],
            "facts":state.get("user_facts",[]),"unknowns":state.get("unknowns",[]),
            "messages":state["messages"],"previous_html":state.get("previous_html",""),
        })
        return self.candidate(state, result, 0)

    def candidate(self, state, result, repairs):
        source = result.get("html")
        if not isinstance(source,str) or not source.strip() or len(source.encode())>512*1024:
            raise ValueError("candidate HTML is missing or too large")
        if not source.lstrip().lower().startswith("<!doctype"):
            source="<!doctype html>"+source
        selected = ids(result.get("used_photo_ids"),state["photo_ids"])
        import re
        referenced={int(x) for x in re.findall(r"ss-photo:([0-9]+)",source)}
        if referenced != set(selected):
            raise ValueError("used photo list does not match HTML references")
        if state["input"]["action"]=="confirm" and set(selected)!=set(state["proposal"]["used_photo_ids"]):
            raise ValueError("generation changed confirmed photo selection")
        interactions = result.get("interactions",[])
        if not isinstance(interactions,list):
            raise ValueError("interaction manifest is invalid")
        return {**state,"html":source,"used_photo_ids":selected,
                "interactions":interactions[:1],"repairs":repairs,"phase":"rendering"}

    async def render(self, state):
        await self.emit({"stage":"rendering","round":state["repairs"]+1})
        assets={str(x):(await self.workspace.call("photo",{"id":x}))["data"]
                for x in state["used_photo_ids"]}
        result = await self.browser.render(state["html"], assets, state["interactions"])
        refs=[]
        for shot in result.pop("screenshots"):
            metadata={**shot["metadata"],"round":state["repairs"]+1}
            saved=await self.workspace.call("asset",{
                "data":base64.b64encode(shot["data"]).decode(),
                "metadata":metadata,
            })
            refs.append({"id":saved["id"],**metadata})
        await self.emit({"stage":"rendered","round":state["repairs"]+1,
                         "screenshots":refs,"browser_seconds":result["browser_seconds"],
                         "browser_queue_seconds":result["browser_queue_seconds"]})
        return {**state,"screenshots":refs,"browser":result}

    async def review(self, state):
        await self.emit({"stage":"reviewing","round":state["repairs"]+1})
        images=[(await self.workspace.call("asset_get",{"id":x["id"]}))["data"]
                for x in state["screenshots"]]
        # Missing coverage is a blocker; it must never become a verified version.
        coverage = {x["viewport"] for x in state["screenshots"]}
        if coverage != {"desktop","mobile"}:
            issues=state["browser"]["diagnostics"] or [{
                "severity":"blocking","viewport":"mobile","region":"all",
                "element":"document","description":"Required screenshots missing",
                "suggestion":"Restore both viewport renders"}]
            summary="Visual inspection could not cover both viewports"
            reviewed=False
        else:
            result=await self._call(REVIEW,{
                "requirements":state["messages"],"proposal":state["proposal"],
                "code":state["html"],"screenshots":state["screenshots"],
                "diagnostics":state["browser"]["diagnostics"],
            },images)
            issues=validate_issues(result.get("issues"))
            issues=issues+state["browser"]["diagnostics"]
            summary=result.get("summary","")
            reviewed=True
        blocking=any(x["severity"]=="blocking" for x in issues)
        report={"rounds":state["repairs"]+1,"issues":issues,"summary":summary,
                "screenshots":state["screenshots"],"coverage":state["browser"]["coverage"],
                "browser_ok":not state["browser"]["diagnostics"],"visual_reviewed":reviewed,
                "limitations":"Only the listed regions and interactions were checked; model review may miss defects.",
                "memory_peak_kib":resource.getrusage(resource.RUSAGE_SELF).ru_maxrss}
        await self.emit({"stage":"reviewed",**report})
        return {**state,"report":report,"verified":reviewed and not blocking,
                "phase":"repairing" if blocking and state["repairs"]<2 else "done"}

    async def repair(self, state):
        await self.emit({"stage":"repairing","round":state["repairs"]+1})
        result=await self._call(REPAIR,{
            "html":state["html"],"issues":state["report"]["issues"],
            "proposal":state["proposal"],"facts":state.get("user_facts",[]),
            "photo_ids":state["photo_ids"],"interactions":state["interactions"],
        })
        return self.candidate(state,result,state["repairs"]+1)

    def graph(self):
        builder=StateGraph(dict)
        for name in ("observe","plan","generate","render","review","repair"):
            builder.add_node(name,getattr(self,name))
        builder.add_conditional_edges(START,lambda state:
            "generate" if state["input"]["action"] in ("confirm","modify") else "observe")
        builder.add_conditional_edges("observe",lambda state:
            "observe" if len(state["observations"])<len(state["photo_ids"]) else "plan")
        builder.add_edge("plan",END)
        builder.add_edge("generate","render")
        builder.add_edge("render","review")
        builder.add_conditional_edges("review",lambda state:
            "repair" if state["phase"]=="repairing" else END)
        builder.add_edge("repair","render")
        return builder.compile(checkpointer=GoCheckpointer(self.workspace))

    async def execute(self):
        context=await self.workspace.call("context")
        saved=(await self.workspace.call("checkpoint_get")).get("checkpoint")
        state={**context.get("state",{}),**{k:v for k,v in context.items() if k!="state"}}
        state["previous_html"]=context.get("html","")
        state["html"]=""
        if state["input"]["action"]=="confirm" and state["input"].get("proposal"):
            proposal=state["input"]["proposal"]
            ids(proposal.get("used_photo_ids"),state["photo_ids"])
            state["proposal"]=proposal
        # Every new user message is the only source of factual additions.
        state["user_facts"]=[{"message_id":x["id"],"text":x["content"]}
                             for x in state["messages"] if x["role"]=="user"]
        state.setdefault("observations",[])
        if state["input"]["action"]=="modify" and not state.get("proposal"):
            raise ValueError("confirm a proposal first")
        graph=self.graph()
        result=await graph.ainvoke(None if saved else state,
            {"configurable":{"thread_id":state["thread_id"]},"recursion_limit":80},
            durability="sync")
        # Strip run-specific context and heavy code before saving work facts.
        work_state={key:result[key] for key in
                    ("observations","proposal","questions","unknowns","user_facts")
                    if key in result}
        waiting=result["phase"]=="waiting"
        message=(json.dumps({"proposal":result["proposal"],"questions":result["questions"]},
                            ensure_ascii=False) if waiting else
                 ("视觉检查通过，作品已保存。" if result["verified"] else
                  "已达到两轮修复上限。候选作品已保存，但未通过视觉检查，请查看报告。"))
        await self.workspace.call("finish",{
            "state":work_state,"report":result.get("report",{}),"html":result.get("html",""),
            "used_photo_ids":result.get("used_photo_ids",[]),
            "verified":result.get("verified",False),"message":message,
        })
        await self.emit({"stage":"waiting" if waiting else "completed",
                         "verified":result.get("verified",False)})
