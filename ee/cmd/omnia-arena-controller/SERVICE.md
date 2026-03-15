# Arena Controller Service (Enterprise)

## Owns
- K8s controller-manager for Arena CRDs:
  - ArenaSource — git-synced evaluation content
  - ArenaJob — batch evaluation job orchestration
  - ArenaDevSession — interactive dev console sessions
  - SessionPrivacyPolicy — data privacy rules
- Worker pod creation and lifecycle management
- Template API server for Arena project scaffolding
- Redis Streams work queue management

## Inputs
- **K8s API**: watch events for Arena CRDs
- **HTTP**: template rendering requests from dashboard

## Outputs
- **K8s API**: worker pods, services, configmaps, CRD status updates
- **Redis Streams**: work items for eval workers
- **HTTP**: template API responses

## Does NOT Own
- Eval execution (Arena Eval Worker's job)
- Session storage (Session API's job)
- Agent runtime management (Operator's job)
- LLM provider interaction (Runtime's job)

## Dependencies
- controller-runtime / client-go
- Redis (work queue)
- Arena CRD types (`ee/api/`)
