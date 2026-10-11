"""Bounded, read-only design functions compatible with the existing model runtime."""
import json
from collections import Counter
import re
from pathlib import Path
from html.parser import HTMLParser

SKILL_PATH = Path(__file__).resolve().parent / "skills/photo-web-design/SKILL.md"
PATTERNS = {
    "photo_desk": {"idea":"Arrange a few photos as a tactile desk; focus one with click or keyboard and offer an intentional shuffle.",
                   "interaction":"A focus button reveals a larger photo; always show usable content without dragging.","layout":"Overlapping accents with a clean mobile stack; never obscure the active photo or labels."},
    "filmstrip": {"idea":"An immersive opening leads into a navigable strip of images.","interaction":"Previous/next controls and touch scrolling; the active image has a stable result ID.","layout":"Alternate a wide image with a smaller detail; add a clear active-photo indicator."},
    "chapter_journey": {"idea":"Use a distinctive composition for each approved narrative scene.","interaction":"A compact chapter navigator or revealable note supports exploration.","layout":"Mix full-width, paired and offset images without adding excessive empty sections."},
    "comparison": {"idea":"Let the viewer compare two supplied images without inventing chronology.","interaction":"A labeled slider or toggle exposes the images with keyboard and touch support.","layout":"A large comparison surface followed by a compact supporting image group."},
}

def design_guidance():
    return SKILL_PATH.read_text(encoding="utf-8")


def schema(name, description, properties, required=()):
    return {"type":"function","function":{"name":name,"description":description,
            "parameters":{"type":"object","properties":properties,"required":list(required),"additionalProperties":False}}}

TOOL_SCHEMAS = [
    schema("read_design_skill","Read the bundled photo website design skill; no file path is accepted.",{}),
    schema("get_design_pattern","Read an interaction/layout recipe to adapt to the approved brief.",
           {"name":{"type":"string","enum":list(PATTERNS)}},["name"]),
    schema("get_photo_context","Read visual references and work-local rotation metadata for at most eight authorized photos.",
           {"photo_ids":{"type":"array","items":{"type":"integer"},"minItems":1,"maxItems":8}},["photo_ids"]),
    schema("inspect_candidate","Inspect HTML structure and references without executing code or launching a browser.",
           {"html":{"type":"string","maxLength":524288}},["html"]),
]

class Structure(HTMLParser):
    def __init__(self):
        super().__init__(); self.controls=[]; self.ids=[]; self.external=False
    def handle_starttag(self,tag,attrs):
        values=dict(attrs)
        if values.get("id"): self.ids.append(values["id"])
        if tag in ("button","input","select") or values.get("role")=="button":
            self.controls.append({"tag":tag,"id":values.get("id")})
        for key in ("src","href"):
            value=values.get(key,"")
            if value.startswith(("https://","http://","//")): self.external=True

class DesignTools:
    def __init__(self,state,emit=None):
        self.scope={int(x) for x in state.get("proposal",{}).get("used_photo_ids",state["photo_ids"])}
        self.observations={int(x["photo_id"]):x for x in state.get("observations",[])}
        self.rotations=state.get("photo_edits",{})
        self.emit=emit

    async def call(self,name,arguments):
        if self.emit: await self.emit({"stage":"designing","status":"started"})
        try:
            if not isinstance(arguments,dict): raise ValueError()
            if name=="read_design_skill" and not arguments:
                result={"skill":design_guidance()}
            elif name=="get_design_pattern" and set(arguments)=={"name"}:
                result=PATTERNS[arguments["name"]]
            elif name=="get_photo_context" and set(arguments)=={"photo_ids"}:
                ids=arguments["photo_ids"]
                if (not isinstance(ids,list) or not 1<=len(ids)<=8 or
                    any(isinstance(x,bool) or not isinstance(x,int) or x not in self.scope for x in ids)):
                    raise ValueError()
                result={"photos":[{"photo_id":x,"visual_reference":self.observations.get(x,{}).get("observation",""),
                                   "asset_rotation":self.rotations.get(str(x),0),"asset_already_rotated":True,
                                   "visual_design":self.observations.get(x,{}).get("visual_design",{}),
                                   "width":self.observations.get(x,{}).get("width"),"height":self.observations.get(x,{}).get("height")} for x in ids]}
            elif name=="inspect_candidate" and set(arguments)=={"html"}:
                source=arguments["html"]
                if not isinstance(source,str) or len(source.encode())>524288: raise ValueError()
                parsed=Structure();parsed.feed(source)
                referenced={int(x) for x in re.findall(r"ss-photo:([0-9]+)",source)}
                result={"bytes":len(source.encode()),"photo_ids":sorted(referenced),
                        "out_of_scope":sorted(referenced-self.scope),"control_ids":parsed.controls,
                        "duplicate_ids":sorted({x for x,count in Counter(parsed.ids).items() if count>1}),
                        "external_resources":parsed.external,"embedded_images":"data:image/" in source}
            else: raise ValueError()
        except (ValueError,KeyError,TypeError):
            result={"error":"Unsupported design tool or invalid arguments."}
        if self.emit: await self.emit({"stage":"designing","status":"finished"})
        return json.dumps(result,ensure_ascii=False)
