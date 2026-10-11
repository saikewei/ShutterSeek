# 创作设计 skill

内置的 [photo-web-design](photo-web-design/SKILL.md) 是为当前 Python Agent 编写的设计规范，
参考 [Anthropic frontend-design](https://github.com/anthropics/skills/tree/main/skills/frontend-design) 的设计流程。
它随镜像发布，每次生成和修复都会加载；不依赖 Codex 或 Claude Code 的 skill 发现机制。

兼容当前 LangChain/OpenAI-compatible 模型协议，使用现有依赖，不升级 LangGraph：
- read_design_skill：读取唯一内置规范，不接受文件路径。
- get_design_pattern：获取可改编的照片桌、胶片条、章节或对照互动方案。
- get_photo_context：仅查询当前作品范围内的视觉参考、尺寸及已经应用的旋转，每次最多 8 张。
- inspect_candidate：静态检查 HTML 的照片引用、控件 ID 和资源，不执行代码。

每次生成/修复的模型调用最多 2 轮工具交互、4 次工具执行，然后要求返回最终 JSON。
所有模型请求仍共用全局 5 个名额。工具只读；渲染、截图、审查和保存仍由原工作流控制。
耗时与用量进入进度指标，检查点不保存工具消息、图片字节或凭证。

设置 SHUTTERSEEK_CREATIONS_DESIGN_TOOLS=false 可关闭模型 function calling；
设计规范仍参与提示词。生成页面仍使用 HTML/CSS/JavaScript、内嵌 SVG 和本地字体，
不提供终端、任意文件、外网、npm 安装或额外浏览器执行工具。
