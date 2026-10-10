"""Optional offline acceptance; uses no application DB, files, or session."""
import asyncio
import base64
import json
import os
import sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from browser import Browser
from model import CreationModel
from concurrency import FairGate
from workflow import REVIEW


async def main():
    if not os.environ.get("SHUTTERSEEK_CREATIONS_MODEL_KEY"):
        raise SystemExit("Configure SHUTTERSEEK_CREATIONS_MODEL_KEY in the development container")
    source='<style>body{margin:0}.title{position:absolute;top:0;font-size:60px}.copy{width:850px;font-size:10px}</style><h1 class="title">Travel story</h1><p class="copy">This paragraph deliberately overlaps the title and overflows mobile.</p>'
    result=await Browser(FairGate(1)).render(source,{})
    directory=Path("tmp/creation-live-review")
    directory.mkdir(parents=True,exist_ok=True)
    images=[];metadata=[]
    for index,shot in enumerate(result["screenshots"]):
        (directory/(str(index)+".png")).write_bytes(shot["data"])
        images.append("data:image/png;base64,"+base64.b64encode(shot["data"]).decode())
        metadata.append(shot["metadata"])
    review,metrics=await CreationModel(FairGate(5)).invoke(REVIEW,{
        "requirements":["Readable responsive travel story"],"code":source,
        "screenshots":metadata,"diagnostics":result["diagnostics"]},images)
    (directory/"report.json").write_text(json.dumps({"review":review,"metrics":metrics},ensure_ascii=False,indent=2))
    print("Saved offline acceptance report to",directory)


if __name__=="__main__":
    asyncio.run(main())
