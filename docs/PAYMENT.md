# External shop recharge

This branch uses external shop product links (链动小铺) and redemption codes only.

In the purchase settings, enable the shop and configure product URLs for the fixed CNY 10 / 20 / 30 / 50 / 100 tiers. Users choose a product at `/purchase`, buy the code at the shop, then redeem it at `/redeem`. Product links receive no identity, session or amount parameters.

The settings are `purchase_subscription_enabled` and `purchase_subscription_products`. See the [external integration API](ADMIN_PAYMENT_INTEGRATION_API.md) for URL validation and code delivery.

Native checkout, payment providers, payment orders and refunds APIs, webhooks and background reconciliation have been removed. Existing payment, audit and affiliate history is retained in the database. Balances, redemption, user subscriptions and affiliate features remain available.

Finish any in-progress native orders or refunds before deploying this change. The code changes do not modify a live database.
