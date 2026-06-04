import grpc from "k6/net/grpc";
import { check } from "k6";

export const options = {
  stages: [
    { duration: "30s", target: 100 },
    { duration: "60s", target: 100 },
    { duration: "30s", target: 0 },
  ],
};

const client = new grpc.Client();

export default () => {
  client.connect(__ENV.ARE_GRPC_ADDR || "127.0.0.1:9090", { plaintext: true });
  const payload = {
    agent_type: "AUTONOMOUS",
    owner_id: "00000000-0000-0000-0000-000000000099",
    external_id: `k6-${__VU}-${__ITER}`,
    metadata: { source: "k6" },
  };
  const res = client.invoke("are.registry.v1.AgentRegistryService/RegisterAgent", payload);
  check(res, {
    "register status OK": (r) => r && r.status === grpc.StatusOK,
  });
  client.close();
};
