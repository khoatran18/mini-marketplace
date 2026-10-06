# Documentation index

| Doc | What it covers |
|---|---|
| [architecture.md](architecture.md) | Services, request flow, order saga, ports, repo layout |
| [data-model.md](data-model.md) | Tables per service, status machine, money handling |
| [api.md](api.md) | REST routes, auth/ownership rules, error mapping |
| [events-kafka.md](events-kafka.md) | Topics, keys, payloads, outbox, delivery guarantees |
| [security.md](security.md) | Authn/authz model, secrets, hardening, known gaps |
| [deployment.md](deployment.md) | Compose/Swarm stacks, env vars, TLS, operations |
| [testing.md](testing.md) | How to run and write tests, what is covered |
| [runbook.md](runbook.md) | Troubleshooting stuck orders, outbox, tokens, Kafka |
| [decisions.md](decisions.md) | Architecture decision records |
| [roadmap.md](roadmap.md) | Remaining tech debt and proposed business features |
| [ai/README.md](ai/README.md) | **AI Platform** (thiết kế, chưa code): agent, recsys, RAG chính sách, giám sát/cảnh báo, UI, API contract, hạ tầng, lộ trình |

The OpenAPI spec is generated into `services/api-gateway/docs/` (`swag init -g cmd/main.go -o docs --parseInternal`, run from `services/api-gateway`).
