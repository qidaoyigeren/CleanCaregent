# Product Packs

Product packs are structured seed files for cleaning-tool products.

- `products` sync into MySQL `products`, `product_skus`, and `product_inventory`.
- `compatibility` extends the in-memory accessory compatibility matrix.
- `diagnosis` extends the in-memory guided troubleshooting engine.
- Static RAG content still belongs in `docs/knowledge-base` unless the pack uses `documents`.

Run checks before ingest:

```bash
go run ./cmd/kb-validate
```

Seed MySQL, Qdrant, and the static KB:

```bash
go run ./cmd/kb-seed
```

If a document keeps the same `doc_id` and `version` but its content changes, bump `version` or run:

```bash
go run ./cmd/kb-seed -force
```
