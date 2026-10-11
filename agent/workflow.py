"""Recoverable creation graph. No image bytes or credentials enter graph state."""
import base64
import json
import resource
import copy
import re
from langgraph.graph import StateGraph, START, END
from checkpoint import GoCheckpointer
from interaction import validate_interaction
from localization import TRANSLATE, display_slots, needs_chinese

OBSERVE = """Observe these photos in the given order. Return
{"observations":[{"photo_id":number,"observation":string,"unknowns":[string]}]}.
Use cautious visual descriptions in Simplified Chinese. No invented facts."""
PLAN = """Propose a content structure, visual style and photo selection first.
Return {"reply":string,
"proposal":{"structure":string,"style":string,"used_photo_ids":[number]},
"questions":[string],"unknowns":[string]} in Simplified Chinese.
Keep reply to 1–3 short sentences, with no questions or questionnaire.
Converse naturally. Acknowledge the user's latest answer before proposing the
next step. Ask at most ONE short, useful question, only in the questions array;
do not hide a questionnaire in reply. Never repeat an answered or skipped question.
Use question_budget: when it is zero, ask no questions and offer a usable plan.
Questions are optional. If the user skipped, ask no further questions this turn.
Do not generate HTML. Incorporate explicit user answers without inventing facts."""
GENERATE = """Return {"html":string,"used_photo_ids":[number],
"interactions":[{"selector":string,"expect_visible":string}]}.
Produce a complete responsive HTML/CSS/JavaScript page with Chinese user-facing text following the confirmed
proposal and user request. All photo src attributes use ss-photo:<id>.
Design a distinctive, photo-led experience matched to the confirmed style and purpose.
Choose composition deliberately: image scale, whitespace, rhythm, typography and useful
interaction can carry the story. Avoid defaulting to identical photo-and-paragraph cards.
Use no external resources. Do not invent facts. Keep factual claims faithful to user messages.
visual_context is PRIVATE DESIGN REFERENCE: use it to choose layout, image grouping,
cropping and visual rhythm. Never paste all visual summaries into visible paragraphs,
captions, titles, alt text, or a list of photo descriptions.
Write only concise editorial copy that helps this particular story. Prefer strong images,
short headings and selective captions. Do not give every photo a paragraph by default.
Longer narrative is appropriate only when requested or supplied by the user.
Omit missing dates, places, names and relationships entirely. Never add "unknown",
"unconfirmed", "pending confirmation", "未确认", "未知", "待确认", analysis notes,
or process disclaimers as metadata badges or placeholder fields in the finished page.
Omitting a detail is required when it is uncertain; never invent a replacement.
Keep alt text brief and factual. Preserve all confirmed photos without requiring captions.
When editing an existing page, remove legacy analysis dumps and uncertainty placeholders;
retain approved narrative and photo selection rather than preserving working notes.
Give a genuine clickable control and its expected result stable ASCII element IDs.
Return at most ONE key interaction to inspect. Both selector and expect_visible
MUST be simple CSS ID selectors, such as "#show-details" and "#details-panel".
Example: {"selector":"#show-details","expect_visible":"#details-panel"}.
expect_visible is an executable selector, NEVER a natural-language description,
an instruction, a sentence, or a description of animation or visual changes.
The selector identifies the control to click; expect_visible identifies the element
that must be visible after clicking. Omit expect_visible when no separate visible
result is appropriate. Do not invent click tests for static headings or decorations.
Return interactions:[] when the page has no genuine click interaction.
Include useful alt text. Keep HTML under 512 KiB.
When locked_photo_ids is present, include EVERY locked photo in the HTML,
include no other photo, and return exactly that list in used_photo_ids.
You may change the layout, but never add, omit or replace confirmed photos."""
REVIEW = """You must visually inspect EVERY supplied screenshot at its original
detail. The screenshot metadata maps image order to viewport and page region.
Evaluate text overlap, horizontal overflow, cropping, mobile readability,
missing photos and interaction state. Browser diagnostics also matter.
Return {"issues":[{"severity":"blocking|warning|info","viewport":"desktop|mobile",
"region":string,"element":string,"description":string,"suggestion":string}],
"summary":string}. Write all human-readable review text in Simplified Chinese. Return no blocking issues only if the inspected regions pass.
Also inspect editorial quality: the page should be photo-led and composed for the
confirmed purpose, not a dump of internal visual descriptions. Treat visible analysis notes,
missing-information placeholders and uncertainty badges as blocking issues.
Do not penalize a deliberately minimal style just for being simple.
Do not claim unseen page regions or untested interactions passed.
A browser check_configuration issue means the interaction test specification is invalid.
Do not infer that page content is invisible or broken from that configuration error;
use screenshot evidence for visual claims and fix the test specification separately."""
REPAIR = """Repair the HTML using the visual issues and browser diagnostics.
Preserve confirmed content, user facts, photo selection and useful interactions.
""" + GENERATE

CANDIDATE_ERRORS = {
    "candidate_html_invalid": "candidate HTML is missing or too large",
    "photo_selection_invalid": "photo selection is invalid or outside the work scope",
    "photo_references_mismatch": "used photo list does not match HTML references",
    "confirmed_photos_changed": "generation changed confirmed photo selection",
    "interaction_manifest_invalid": "interaction manifest requires objects with simple CSS #id selectors; expect_visible must be a selector, never prose",
}


class CandidateError(ValueError):
    def __init__(self, code):
        self.code = code
        super().__init__(CANDIDATE_ERRORS[code])


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


def publication_context(state):
    # Only user-sourced facts and visual references enter the publication brief.
    return {
        "proposal":state["proposal"],
        "visual_context":[{"photo_id":item["photo_id"],
                           "visual_summary":item.get("observation","")}
                          for item in state.get("observations",[])],
        "editorial_facts":[{"message_id":item["message_id"],"text":item["text"]}
                           for item in state.get("user_facts",[])],
        "user_requests":[{"id":item["id"],"content":item["content"]}
                         for item in state.get("messages",[]) if item["role"]=="user"],
        "copy_policy":{"visual_context":"reference_only","uncertain_facts":"omit",
                       "default":"photo_led_selective_copy"},
    }


class Workflow:
    def __init__(self, workspace, model, browser, emit):
        self.workspace, self.model, self.browser, self.emit = workspace, model, browser, emit

    async def _call(self, instruction, data, images=(), stage=None):
        operation = stage or {OBSERVE:"observing", PLAN:"planning", REVIEW:"reviewing",
                              REPAIR:"repairing", TRANSLATE:"localizing"}.get(instruction,"generating")
        await self.emit({"stage":operation,"status":"started"})
        result, metric = await self.model.invoke(instruction, data, images)
        await self.emit({"stage":operation,"status":"finished",**metric})
        return result

    async def chinese(self, value):
        value = copy.deepcopy(value)
        slots = display_slots(value)
        for offset in range(0, len(slots), 24):
            batch = slots[offset:offset + 24]
            result = await self._call(TRANSLATE, {"texts":[
                {"id":index,"text":entry[2]} for index,entry in enumerate(batch)]})
            translations = result.get("translations")
            if not isinstance(translations, list) or len(translations) != len(batch):
                raise ValueError("Chinese translations are incomplete")
            seen = set()
            for entry in translations:
                index = entry.get("id")
                text = entry.get("text")
                if (not isinstance(index,(int,float)) or int(index)!=index or
                        index not in range(len(batch)) or index in seen or
                        not isinstance(text,str) or not text.strip() or needs_chinese(text)):
                    raise ValueError("Chinese translation schema is invalid")
                seen.add(index)
                container,key,_ = batch[int(index)]
                container[key] = text
        return value

    async def localize(self, state):
        result = await self.chinese(state)
        if state["input"]["action"] == "localize":
            result.update(phase="waiting",html="",reply="图片描述和方案已更新为中文。你可以继续聊想法，或确认方案开始生成。")
        return result

    async def observe(self, state):
        observations = list(state.get("observations", []))
        completed = {int(x["photo_id"]) for x in observations}
        todo = [x for x in state["photo_ids"] if x not in completed]
        # Every batch is its own graph node and checkpoint, so recovery does
        # not repeat all earlier photo-understanding calls.
        batch = todo[:8]
        if batch:
            await self.emit({"stage":"observing","completed":len(observations),
                             "total":len(state["photo_ids"]),"batch_size":len(batch)})
            images = [(await self.workspace.call("photo", {"id":x}))["data"] for x in batch]
            result = await self.chinese(await self._call(OBSERVE, {"photo_ids":batch}, images))
            records = result.get("observations")
            if not isinstance(records, list) or len(records)!=len(batch) or {int(x["photo_id"]) for x in records} != set(batch):
                raise ValueError("observations do not match the current batch")
            observations.extend(records)
            await self.emit({"stage":"observing","completed":len(observations),
                             "total":len(state["photo_ids"])})
        return {**state, "observations":observations}

    async def plan(self, state):
        asked = list(state.get("asked_questions",[]))
        count = max(int(state.get("question_count",len(asked))),len(asked))
        result = await self._call(PLAN, {
            "title":state["title"],"observations":state["observations"],
            "messages":state["messages"],"previous_proposal":state.get("proposal"),
            "skipped":state["input"]["action"]=="skip",
            "asked_questions":asked,"question_budget":max(0,3-count),
            "previous_question":next(iter(state.get("questions",[])),None),
        })
        questions = result.get("questions", [])
        if not isinstance(questions,list) or any(not isinstance(x,str) for x in questions):
            raise ValueError("questions must contain text")
        result["questions"] = questions[:1] if count < 3 and state["input"]["action"] != "skip" else []
        result = await self.chinese(result)
        proposal = result.get("proposal")
        if (not isinstance(proposal,dict) or not isinstance(proposal.get("structure"),str)
                or not isinstance(proposal.get("style"),str) or not proposal["structure"].strip()
                or not proposal["style"].strip()):
            raise ValueError("proposal is incomplete")
        proposal["used_photo_ids"] = ids(proposal.get("used_photo_ids"), state["photo_ids"])
        questions = [re.split(r"(?<=[?？])",q.strip(),maxsplit=1)[0]
                     for q in result.get("questions",[]) if q.strip()][:1]
        questions = [q for q in questions if q not in asked]
        reply = result.get("reply") or "我根据照片和你的想法整理了这份方案。你可以继续调整，也可以直接开始生成。"
        if not isinstance(reply,str):
            raise ValueError("chat reply must be text")
        reply = re.sub(r"[^。！？.!?\n]*[?？]","",reply).strip() or "我已经整理好当前方案，你可以继续调整，也可以直接开始生成。"
        # Explicit facts stay tied to user messages, never copied to photo metadata.
        return {**state, "proposal":proposal, "questions":questions,"reply":reply,
                "question_count":count+len(questions),"asked_questions":asked+questions,
                "unknowns":result.get("unknowns",[]),
                "user_facts":state.get("user_facts",[]),
                "phase":"waiting", "html":""}

    async def generate(self, state):
        data = {**publication_context(state),"previous_html":state.get("previous_html","")}
        if state["input"]["action"] == "confirm":
            data["locked_photo_ids"] = state["proposal"]["used_photo_ids"]
        # One bounded contract correction precedes rendering; visual repairs stay capped at two.
        for attempt in range(2):
            await self.emit({"stage":"generating" if attempt == 0 else "correcting",
                             "attempt":attempt + 1})
            result = await self._call(GENERATE, data, stage="generating" if attempt == 0 else "correcting")
            try:
                return self.candidate(state, result, 0)
            except CandidateError as error:
                if attempt == 1:
                    raise
                data = {**data, "rejected_candidate":result,
                        "validation_error":str(error),
                        "correction":"Correct the candidate contract and preserve every locked photo."}

    def candidate(self, state, result, repairs):
        source = result.get("html")
        if not isinstance(source,str) or not source.strip() or len(source.encode())>512*1024:
            raise CandidateError("candidate_html_invalid")
        if not source.lstrip().lower().startswith("<!doctype"):
            source="<!doctype html>"+source
        try:
            selected = ids(result.get("used_photo_ids"),state["photo_ids"])
        except (ValueError, TypeError):
            raise CandidateError("photo_selection_invalid") from None
        import re
        referenced={int(x) for x in re.findall(r"ss-photo:([0-9]+)",source)}
        if referenced != set(selected):
            raise CandidateError("photo_references_mismatch")
        if state["input"]["action"]=="confirm" and set(selected)!=set(state["proposal"]["used_photo_ids"]):
            raise CandidateError("confirmed_photos_changed")
        interactions = result.get("interactions",[])
        if not isinstance(interactions,list):
            raise CandidateError("interaction_manifest_invalid")
        try:
            for item in interactions[:1]:
                validate_interaction(item)
        except ValueError:
            raise CandidateError("interaction_manifest_invalid") from None
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
                "element":"document","description":"缺少必要的页面截图",
                "suggestion":"重新检查桌面和手机页面"}]
            summary="本次未能完整检查桌面和手机页面"
            reviewed=False
        else:
            result=await self._call(REVIEW,{
                "requirements":state["messages"],"proposal":state["proposal"],
                "code":state["html"],"screenshots":state["screenshots"],
                "diagnostics":state["browser"]["diagnostics"],
            },images)
            result=await self.chinese(result)
            issues=validate_issues(result.get("issues"))
            issues=issues+state["browser"]["diagnostics"]
            summary=result.get("summary","")
            reviewed=True
        blocking=any(x["severity"]=="blocking" for x in issues)
        report={"rounds":state["repairs"]+1,"issues":issues,"summary":summary,
                "screenshots":state["screenshots"],"coverage":state["browser"]["coverage"],
                "browser_ok":not state["browser"]["diagnostics"],"visual_reviewed":reviewed,
                "limitations":"仅检查报告列出的页面区域和交互；模型仍可能遗漏问题。",
                "memory_peak_kib":resource.getrusage(resource.RUSAGE_SELF).ru_maxrss}
        await self.emit({"stage":"reviewed",**report})
        return {**state,"report":report,"verified":reviewed and not blocking,
                "phase":"repairing" if blocking and state["repairs"]<2 else "done"}

    async def repair(self, state):
        await self.emit({"stage":"repairing","round":state["repairs"]+1})
        result=await self._call(REPAIR,{
            **publication_context(state),
            "html":state["html"],"issues":state["report"]["issues"],
            "photo_ids":state["photo_ids"],"interactions":state["interactions"],
            **({"locked_photo_ids":state["proposal"]["used_photo_ids"]}
               if state["input"]["action"] == "confirm" else {}),
        })
        return self.candidate(state,result,state["repairs"]+1)

    def graph(self):
        builder=StateGraph(dict)
        for name in ("localize","observe","plan","generate","render","review","repair"):
            builder.add_node(name,getattr(self,name))
        builder.add_edge(START,"localize")
        builder.add_conditional_edges("localize",lambda state:
            END if state["input"]["action"] == "localize" else
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
        previous_facts={x["message_id"]:x for x in state.get("user_facts",[])}
        state["user_facts"]=[{"message_id":x["id"],"text":x["content"],
                             "display_text":previous_facts.get(x["id"],{}).get("display_text",x["content"])}
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
                    ("observations","proposal","questions","unknowns","user_facts","reply","question_count","asked_questions")
                    if key in result}
        waiting=result["phase"]=="waiting"
        message=(json.dumps({"reply":result.get("reply",""),"proposal":result.get("proposal"),"questions":result.get("questions",[])},
                            ensure_ascii=False) if waiting else
                 ("视觉检查通过，作品已保存。" if result["verified"] else
                  "已达到两轮修复上限。候选作品已保存，但未通过视觉检查，请查看报告。"))
        await self.emit({"stage":"saving"})
        await self.workspace.call("finish",{
            "state":work_state,"report":result.get("report",{}),"html":result.get("html",""),
            "used_photo_ids":result.get("used_photo_ids",[]),
            "verified":result.get("verified",False),"message":message,
        })
        await self.emit({"stage":"waiting" if waiting else "completed",
                         "verified":result.get("verified",False)})
