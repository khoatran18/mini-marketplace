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
| [protobuf.md](protobuf.md) | Regenerating `.pb.go` offline (buf.build unreachable), history of proto changes |
| [decisions.md](decisions.md) | Architecture decision records |
| [roadmap.md](roadmap.md) | Remaining tech debt and proposed business features |
| [platform/README.md](platform/README.md) | **Thiết kế nền tảng (chưa code)**: health/ready/metrics, thanh toán mô phỏng, vòng đời đơn, tracking + ClickHouse, analytics, cảnh báo, UI, API, hạ tầng, lộ trình |
| [AI-DRAFT.md](AI-DRAFT.md) | ⚠️ **Nháp ý tưởng AI – không dùng, không code theo** |

The OpenAPI spec is generated into `services/api-gateway/docs/` (`swag init -g cmd/main.go -o docs --parseInternal`, run from `services/api-gateway`).
