# TypeScript SDK

This folder contains generated TypeScript SDKs from the OpenAPI contracts.

## Requirements

- openapi-generator-cli
  - Install with: `brew install openapi-generator`

## Generate SDKs

From the repo root:

```bash
bash exchange-common/scripts/generate-sdk.sh
```

Generate a single service:

```bash
bash exchange-common/scripts/generate-sdk.sh --service exchange-gateway
```

Use a packaged contract directory:

```bash
bash exchange-common/scripts/generate-sdk.sh --input contracts/versions/v1.0.0/openapi
```

Output is written to `sdk/typescript/generated`.

## Example

See `sdk/typescript/example.ts` for a minimal client usage example.
