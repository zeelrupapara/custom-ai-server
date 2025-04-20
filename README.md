## Custom AI Server
Now u can use your model and create custom specialized GPT 

```
System Components:
1) Admin-Only Custom GPT Configuration
- Backend devs define GPTs via YAML/JSON configs (no user access).

2) Multi-GPT WebSocket Interaction
- Users connect to specific GPTs via WebSocket (/ws/{gpt-slug}).

3) Model-Agnostic Interface
- Abstract AI model layer (support for GPT-4, Gemini, DeepSeek, etc.).

4) File Attachment & Prompt Templating
- Attach files (PDF, TXT) and pre-define prompts per GPT.
```

```
                      +----------------+
                      | Admin Config    | (YAML/JSON files)
                      +--------+-------+
                               | Load on startup
                               v
+---------+  WebSocket  +------+--------+       +-----------------+
| User    +-------------> API Gateway  +-------> GPT Dispatcher   |
+---------+ (JWT Auth)  +------+--------+       +--------+--------+
                               |                         |
                               v                         v
                      +--------+--------+       +--------+--------+
                      | Redis Sessions |       | AI Model Interface|
                      +-----------------+       +--------+--------+
                                                         |
                                                         v
                                                +--------+--------+
                                                | Model Impl      |
                                                | (GPT-4, Gemini)|
                                                +----------------+
```