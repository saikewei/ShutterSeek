import json
import sys
import unittest
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from design_tools import DesignTools

class DesignToolTests(unittest.IsolatedAsyncioTestCase):
    async def test_tools_use_only_this_work_and_never_accept_file_paths(self):
        tools=DesignTools({"photo_ids":[1],"observations":[{"photo_id":1,"observation":"海边风景"}],"photo_edits":{"1":90}})
        photo=json.loads(await tools.call("get_photo_context",{"photo_ids":[1]}))["photos"][0]
        self.assertEqual(photo["visual_reference"],"海边风景")
        self.assertEqual(photo["asset_rotation"],90)
        self.assertTrue(photo["asset_already_rotated"])
        self.assertIn("error",json.loads(await tools.call("get_photo_context",{"photo_ids":[2]})))
        self.assertIn("error",json.loads(await tools.call("shell",{"command":"cat /etc/passwd"})))
        with patch("design_tools.design_guidance") as read:
            self.assertIn("error",json.loads(await tools.call("read_design_skill",{"path":"../../.env.local"})))
            read.assert_not_called()
        other=DesignTools({"photo_ids":[2]})
        self.assertIn("error",json.loads(await other.call("get_photo_context",{"photo_ids":[1]})))

    async def test_skill_patterns_and_structural_check_have_no_execution_capability(self):
        tools=DesignTools({"photo_ids":[1]})
        skill=json.loads(await tools.call("read_design_skill",{}))["skill"]
        self.assertIn("No React",skill)
        self.assertIn("read-only helpers",skill)
        pattern=json.loads(await tools.call("get_design_pattern",{"name":"photo_desk"}))
        self.assertIn("interaction",pattern)
        inspected=json.loads(await tools.call("inspect_candidate",{"html":'<img src="ss-photo:2"><button id="go">查看</button><img src="https://example.invalid/p.png">'}))
        self.assertEqual(inspected["out_of_scope"],[2])
        self.assertTrue(inspected["external_resources"])
        self.assertEqual(inspected["control_ids"],[{"tag":"button","id":"go"}])
