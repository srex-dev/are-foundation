import express from "express";
import pino from "pino";
import client from "prom-client";
import { evaluateWithBundles } from "./evaluator";
import { createGrpcAdmissionEnvelopeClient } from "./grpc-admission-envelope-client";
import { OpaIntegrationService } from "./service";
import { BundlePayload, PolicyRegistryClient } from "./types";

class StaticRegistry implements PolicyRegistryClient {
  private readonly bundles: BundlePayload[];

  public constructor() {
    const raw = process.env.ARE_OPA_DEFAULT_BUNDLE_NAME;
    const bundleName = raw && raw.trim() !== "" ? raw.trim() : "default-policy";
    this.bundles = [
      {
        bundleId: "bundle-default-1",
        bundleName,
        version: "1.0.0",
        regoSource: "ALLOW action=READ prefix=resource/\nDENY action=DELETE prefix=resource/protected"
      }
    ];
  }

  public async getActiveBundles(): Promise<BundlePayload[]> {
    return this.bundles;
  }

  public async getActiveBundle(bundleName: string): Promise<BundlePayload> {
    const found = this.bundles.find((b) => b.bundleName === bundleName);
    if (!found) {
      throw new Error("bundle_not_found");
    }
    return found;
  }

  public async getBundle(bundleName: string, version: string): Promise<BundlePayload> {
    const found = this.bundles.find((b) => b.bundleName === bundleName && b.version === version);
    if (!found) {
      throw new Error("bundle_not_found");
    }
    return found;
  }
}

async function bootstrap(): Promise<void> {
  const logger = pino({ level: process.env.ARE_OPA_LOG_LEVEL ?? "info" });
  const app = express();
  const healthPort = Number(process.env.ARE_OPA_HEALTH_PORT ?? "8080");
  const registry = new StaticRegistry();
  const admissionAddr = process.env.ARE_AGENT_REGISTRY_GRPC_ADDR?.trim();
  const service = new OpaIntegrationService(
    registry,
    30,
    10000,
    evaluateWithBundles,
    admissionAddr ? { admissionEnvelopeClient: createGrpcAdmissionEnvelopeClient(admissionAddr) } : {}
  );
  await service.startupLoadBundles();

  const bundleGauge = new client.Gauge({
    name: "are_opa_bundles_loaded_total",
    help: "Loaded OPA bundles"
  });

  app.get("/healthz", (_req, res) => {
    res.status(200).send("ok");
  });
  app.get("/readyz", (_req, res) => {
    const loaded = service.getLoadedBundles().bundles.length;
    if (loaded < 1) {
      res.status(503).send("not_ready");
      return;
    }
    res.status(200).send("ready");
  });
  app.get("/metrics", async (_req, res) => {
    bundleGauge.set(service.getLoadedBundles().bundles.length);
    res.set("Content-Type", client.register.contentType);
    res.end(await client.register.metrics());
  });

  app.listen(healthPort, () => {
    logger.info({ healthPort }, "opa integration started");
  });
}

void bootstrap();
