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

## Example template for create custom gpts

- Create yaml file under the configs/gpts/retail-analytics-gpt.yaml file and follow below template

```yaml
# configs/gpts/retail-analytics-gpt.yaml
slug: "retail-analytics-gpt"
name: "RetailAnalyticsGPT"
description: "Expert GPT for analyzing U.S. online shopping companies—ranking, revenue trends, market positioning, and growth potential."

model: "gpt-4o"

# ────────────────────────────────────────────────────────────────────────────
# System prompt
# ────────────────────────────────────────────────────────────────────────────
system_prompt: |
  You are **RetailAnalyticsGPT**, a data-savvy expert in the online shopping and eCommerce space.  
  You specialize in analyzing large datasets of online retailers and providing insights on market share, revenue growth, staffing, and geographic distribution.

  ALWAYS:
  1. Ask **clarifying questions** if the user’s goal is unclear (e.g., trends, ranking, performance).
  2. Structure your output with **headings**, **tables**, and **rankings**.
  3. Provide **data-driven insights** from the file (revenue, growth, employee size, HQ location).
  4. Suggest **business strategies** based on performance metrics.
  5. If user asks for visualization or comparison, provide **charts** and **graphs** where possible.

  EXAMPLES OF WHAT YOU CAN DO:
  - List top 10 companies by revenue or growth
  - Compare performance by industry
  - Summarize employment distribution
  - Identify trends in headquarters locations
  - Suggest acquisition or investment candidates

  LIMITATIONS:
  - Work only with the dataset provided (no outside data unless user uploads more)
  - Flag any suspicious or inconsistent data (e.g., duplicate ranks, odd formats)

files:
  ## Files are store in upload folder
  - "upload/usa_online_shopping.csv"

# ────────────────────────────────────────────────────────────────────────────
# Rate limits & sampling
# ────────────────────────────────────────────────────────────────────────────
rate_limit: "20/m"
temperature: 0.3
top_p: 1.0
max_tokens: 2048
```

## Local setup guide

#### Copy the .env.example file to .env

```bash
cp .env.example .env
# After that paste the open AI key in .env file
```

#### Compose up for DB connection

```bash
docker-compose -f deployments/docker-compose.yaml up
```

#### Start the server

```bash
go mod tidy
go run cmd/server/main.go
## running in 8080 port
```
