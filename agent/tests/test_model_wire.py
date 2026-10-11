import json
import os
import sys
import unittest
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import httpx
from langchain_openai import ChatOpenAI
from langchain_core.messages import AIMessage
from model import CreationModel
from concurrency import FairGate
from design_tools import DesignTools


class ModelWireTests(unittest.IsolatedAsyncioTestCase):
    async def test_langchain_sends_original_screenshots_in_user_message(self):
        captured=[]
        def respond(request):
            body=json.loads(request.content)
            captured.append(body)
            return httpx.Response(200,json={
                "id":"fake","object":"chat.completion","created":1,"model":"deepseek-flash",
                "choices":[{"index":0,"finish_reason":"stop",
                            "message":{"role":"assistant","content":'{"issues":[]}'}}],
                "usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}})
        with patch.dict(os.environ,{"SHUTTERSEEK_CREATIONS_MODEL_KEY":"test-placeholder"}):
            wrapper=CreationModel(FairGate(5))
        async with httpx.AsyncClient(transport=httpx.MockTransport(respond)) as client:
            wrapper.model=ChatOpenAI(model="deepseek-flash",api_key="test-placeholder",
                                    base_url="https://model.invalid",http_async_client=client)
            result,metrics=await wrapper.invoke("review",{"code":"<h1>Hello</h1>"},
                                                ["data:image/png;base64,AAAA"])
        self.assertEqual(result,{"issues":[]})
        image=captured[0]["messages"][1]["content"][1]
        self.assertEqual(captured[0]["messages"][1]["role"],"user")
        self.assertEqual(image["image_url"]["detail"],"original")
        self.assertEqual(image["image_url"]["url"],"data:image/png;base64,AAAA")
        self.assertEqual(metrics["usage"]["total_tokens"],8)


    async def test_langchain_design_functions_return_tool_messages_then_final_json(self):
        captured=[]
        def respond(request):
            body=json.loads(request.content);captured.append(body)
            count=len(captured)
            if count<3:
                name="read_design_skill" if count==1 else "get_design_pattern"
                arguments={} if count==1 else {"name":"photo_desk"}
                message={"role":"assistant","content":None,"tool_calls":[{"id":"call-"+str(count),"type":"function",
                           "function":{"name":name,"arguments":json.dumps(arguments)}}]}
            else:message={"role":"assistant","content":'{"html":"<main>作品</main>","used_photo_ids":[1],"interactions":[]}'}
            return httpx.Response(200,json={"id":"fake","object":"chat.completion","created":1,"model":"deepseek-flash",
                "choices":[{"index":0,"finish_reason":"tool_calls" if count<3 else "stop","message":message}],
                "usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}})
        with patch.dict(os.environ,{"SHUTTERSEEK_CREATIONS_MODEL_KEY":"test-placeholder"}):
            wrapper=CreationModel(FairGate(5))
        async with httpx.AsyncClient(transport=httpx.MockTransport(respond)) as client:
            wrapper.model=ChatOpenAI(model="deepseek-flash",api_key="test-placeholder",base_url="https://model.invalid",http_async_client=client)
            result,metric=await wrapper.invoke_with_tools("Generate",{},[],DesignTools({"photo_ids":[1]}))
        self.assertEqual(result["used_photo_ids"],[1])
        self.assertEqual(metric["tool_calls"],2)
        self.assertEqual(metric["usage"]["total_tokens"],24)
        self.assertEqual(captured[2]["tool_choice"],"none")
        self.assertEqual(captured[1]["messages"][-1]["role"],"tool")
        self.assertEqual(captured[1]["messages"][-1]["tool_call_id"],"call-1")
        self.assertEqual(wrapper.gate.active,0)
        self.assertEqual({x["function"]["name"] for x in captured[0]["tools"]},
                         {"read_design_skill","get_design_pattern","get_photo_context","inspect_candidate"})


    async def test_design_tool_executions_and_model_rounds_are_bounded(self):
        choices=[]
        class Client:
            def bind_tools(self,tools,tool_choice):
                choices.append(tool_choice);return self
            async def ainvoke(self,messages):
                if len(choices)==3:return AIMessage(content='{"accepted":true}')
                return AIMessage(content="",tool_calls=[
                    {"name":"get_design_pattern","args":{"name":"photo_desk"},"id":"call-"+str(len(choices))+"-"+str(i),"type":"tool_call"} for i in range(4)])
        class Tools:
            def __init__(self):self.calls=0
            async def call(self,name,args):
                self.calls+=1;return '{"pattern":"synthetic"}'
        with patch.dict(os.environ,{"SHUTTERSEEK_CREATIONS_MODEL_KEY":"test-placeholder"}):
            wrapper=CreationModel(FairGate(5))
        wrapper.model=Client();tools=Tools()
        result,metric=await wrapper.invoke_with_tools("Generate",{},[],tools)
        self.assertTrue(result["accepted"])
        self.assertEqual(tools.calls,4)
        self.assertEqual(metric["tool_calls"],4)
        self.assertEqual(choices,["auto","auto","none"])
        self.assertEqual(wrapper.gate.active,0)
