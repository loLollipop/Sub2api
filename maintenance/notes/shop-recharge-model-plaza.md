# Fixed shop recharge and passthrough plaza

Baseline: `72de629ad5fc249b9e9a02a6e15964020d999cc2` / `2.0.61-personal.1`.
Release: `2.0.61-personal.2`. Upstream cutoff remains `1b464c0da7ca27cbc7f5435dd812aa691fcf7195`.

## External purchase contract

`purchase_subscription_products` stores a JSON object in the existing settings table. Its only supported keys are `10`, `20`, `30`, `50`, and `100`; each value is that tier's HTTP(S) product URL. Blank values leave a tier unavailable. Omitted product configuration in a partial settings update preserves the existing map; an explicit empty object clears it. Enabling external purchase requires at least one valid configured tier.

The legacy scalar `purchase_subscription_url` is retained for compatibility with stored settings but does not populate tier links. External purchase takes precedence on `/purchase`; `/orders` continues to require internal payment. The selected product URL opens in a new tab without adding an amount, token, or user identity. Credit is granted only through the existing redemption flow.

## Model listing contract

OpenAI passthrough leaves the gateway's routing model catalog unknown (`nil`). The plaza display adapter falls back to the OpenAI defaults only when the same catalog reports a schedulable OpenAI account. Empty groups remain empty; account catalog caching, group allowlists, and channel price precedence are preserved.

## Deployment

No migrations, new dependencies, balance conversion, or rate updates. Publish the tested personal release, back up the live database, runtime configuration, and previous binary, then use the existing single-instance release updater with migration validation. When product links have not been supplied, keep external purchase disabled and configure them in the administrator payment settings before enabling it.
