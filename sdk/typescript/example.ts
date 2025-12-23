import { Configuration, PublicApi } from "./generated/exchange-gateway";

async function main(): Promise<void> {
  const config = new Configuration({
    basePath: "http://localhost:8080",
  });

  const publicApi = new PublicApi(config);
  const health = await publicApi.healthCheck();
  process.stdout.write(`health: ${JSON.stringify(health)}\n`);
}

main().catch((error) => {
  process.stderr.write(`SDK example failed: ${error}\n`);
  process.exit(1);
});
