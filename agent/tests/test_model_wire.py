import json
import os
import sys
import unittest
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import httpx
from langchain_openai import ChatOpenAI
from model import CreationModel
from concurrency import FairGate


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
