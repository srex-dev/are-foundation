import * as grpc from "@grpc/grpc-js";
import * as protoLoader from "@grpc/proto-loader";
import path from "node:path";
import type { AdmissionEnvelope, AdmissionEnvelopeClient } from "./types";

type GrpcCallback<T> = (err: grpc.ServiceError | null, res: T) => void;

/** gRPC client for `AgentRegistryService.GetAdmissionEnvelope` (insecure channel). */
export function createGrpcAdmissionEnvelopeClient(target: string): AdmissionEnvelopeClient {
  const protoRoot = path.join(__dirname, "..", "..", "..", "s0", "agent-registry-service");
  const protoPath = path.join(protoRoot, "proto", "agent_registry.proto");
  const pkg = protoLoader.loadSync(protoPath, {
    longs: String,
    enums: String,
    defaults: true,
    oneofs: true,
    includeDirs: [path.join(protoRoot)]
  });
  const loaded = grpc.loadPackageDefinition(pkg) as {
    are: { registry: { v1: { AgentRegistryService: grpc.ServiceClientConstructor } } };
  };
  const Client = loaded.are.registry.v1.AgentRegistryService;
  const client = new Client(target, grpc.credentials.createInsecure()) as {
    GetAdmissionEnvelope: (req: { agentId: string }, cb: GrpcCallback<{ envelope?: Record<string, unknown> }>) => void;
  };

  return {
    async getAdmissionEnvelope(agentId: string): Promise<AdmissionEnvelope | null> {
      return await new Promise((resolve, reject) => {
        client.GetAdmissionEnvelope({ agentId }, (err, res) => {
          if (err) {
            if (err.code === grpc.status.NOT_FOUND) {
              resolve(null);
              return;
            }
            reject(err);
            return;
          }
          const e = res?.envelope;
          if (!e) {
            resolve(null);
            return;
          }
          const capsRaw = e.admittedBehavioralCaps as Record<string, number> | undefined;
          const scopesRaw = e.admittedScopes as string[] | undefined;
          const sigRaw = e.signature as Buffer | Uint8Array | undefined;
          resolve({
            envelopeId: String(e.envelopeId ?? ""),
            agentId: String(e.agentId ?? ""),
            policyId: String(e.policyId ?? ""),
            policyVer: String(e.policyVer ?? ""),
            admittedScopes: Array.isArray(scopesRaw) ? scopesRaw.map(String) : [],
            admittedBehavioralCaps: capsRaw && typeof capsRaw === "object" ? { ...capsRaw } : {},
            admittedTsMs: Number(e.admittedTsMs ?? 0),
            issuingAuthority: String(e.issuingAuthority ?? ""),
            signature: sigRaw ? new Uint8Array(sigRaw) : new Uint8Array()
          });
        });
      });
    }
  };
}
