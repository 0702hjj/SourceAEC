# Projects, chat and SSE

Projects own one chat session and route the in-process agent. Project creation/deletion uses `/api/v1/chat/projects`;
planning artifacts use `/api/v1/projects/{id}`. Both prefixes refer to the same project.

## Projects

POST `/api/v1/chat/projects` accepts title/kind, with kind `ifc`, `cad` or `cad->ifc`.
IFC initializes a skeleton model/version; CAD pipelines initialize models when needed.
GET `/api/v1/chat/projects/{id}` returns projectId/title/kind/createdAt/models; each model has id/kind/name/status.
Status is resolved from current model state, not a stale project projection.
Kind is immutable. DELETE removes the project, sessions/events, plans, all owned models and associated data.
Download required results before replacing a project/pipeline.

## Sessions

| Method/path | Contract |
| --- | --- |
| GET `/api/v1/chat/sessions` | Session list, including chatSessionId, legacy opencodeSessionId, modelId/projectId/title/createdAt |
| POST `/api/v1/chat/sessions` | Create/reuse by title/projectId; one session per project; legacy title/modelId remains |
| POST `/api/v1/chat/sessions/{cid}/messages` | Send text; asynchronous accepted=true |
| GET `/api/v1/chat/sessions/{cid}/messages` | History; clients deduplicate by message ID |
| GET `/api/v1/chat/sessions/{cid}/events` | SSE stream |
| POST `/api/v1/chat/sessions/{cid}/answer` | interruptId/answer resumes the agent |
| POST `/api/v1/chat/sessions/{cid}/abort` | Abort the active turn |

## SSE events

| Event | Payload/purpose |
| --- | --- |
| session.status | busy/idle turn boundary |
| message.updated | Message skeleton |
| message.part.updated | Final text/reasoning/tool part |
| message.part.delta | Stream delta |
| subagent.status | subagentId/persona/status/task |
| question.ask | interruptId/question |
| viewer.staged | modelId/kind intermediate preview |
| viewer.committed | Committed model refresh |
| model.created | modelId/kind/title/projectId |
| session.error | Error |
| session.idle | Turn ended |

Last-Event-ID supports reconnect with the latest 64 buffered events. EventSource may use a query token;
ordinary API calls require the Authorization header.

## Planning artifacts

GET/PUT `/api/v1/projects/{projectID}/{name}` reads/replaces plan, bim_supplement or building, recording plan history.
GET `/plan_history` lists versions; GET `/plan_history/{base}/{target}/diff` accepts v{n}/current.
POST `/deliver` accepts plan/bimSupplement.
Other skill-work intermediates such as design.json are not PlanStore versions and are cleaned with the project.
See [agents](/en/integrations/agents) and [skills](/en/integrations/skills).
