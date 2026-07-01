# 清洁工具知识库扩品目录

`cmd/kb-seed` 会读取本目录下的 `.md`、`.markdown` 和 `.json` 文件，并写入 MySQL + Qdrant 知识库。

新增产品的推荐方式：

1. 复制 `products/breeze-mop-m1.md` 或 `products/window-squeegee-ws2.json`。
2. 修改 `doc_id`、`title`、`category`、`brand`、`doc_type`、`version`、`model` 或 `models`。
3. 在正文中写入商品参数、适用场景、限制、配件、使用维护和售后边界。
4. 运行 `go run ./cmd/kb-seed`，或用 `go run ./cmd/kb-seed -kb-paths docs/knowledge-base -builtin=false` 只灌文档目录。

常用 `doc_type`：

| doc_type | 用途 |
|---|---|
| `product_detail` | 单个产品详情、卖点、适用场景、限制 |
| `product_parameter` | 参数表、规格表 |
| `product_comparison` | 多产品对比 |
| `purchase_guide` | 选购指南、场景推荐 |
| `accessory_compatibility` | 耗材和配件兼容 |
| `user_manual` | 使用、安装、清洁、维护步骤 |
| `troubleshooting` | 故障排查 |
| `after_sales_policy` | 售后、退换、保修政策 |
| `faq` | 常见问答 |

`model`/`models` 会写入检索 metadata。用户之后问 `BM-M1 适合木地板吗` 或 `WS2 参数` 时，Agent 会优先按这些型号召回相关文档。
