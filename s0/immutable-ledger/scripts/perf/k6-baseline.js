import grpc from 'k6/net/grpc';
import encoding from 'k6/encoding';
import { check } from 'k6';
import exec from 'k6/execution';

const client = new grpc.Client();
client.load(['/work/proto'], 'immutable_ledger.proto');
const target = __ENV.K6_TARGET || 'host.docker.internal:9092';
let connected = false;

export const options = {
  scenarios: {
    write_action_receipt: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writeActionReceipt',
    },
    write_policy_eval: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writePolicyEval',
    },
    write_agent_lifecycle: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writeAgentLifecycle',
    },
    write_credential_lifecycle: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writeCredentialLifecycle',
    },
    write_passport_lifecycle: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writePassportLifecycle',
    },
    write_escalation: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writeEscalation',
    },
    write_drift_event: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writeDriftEvent',
    },
    write_gate_decision: {
      executor: 'constant-arrival-rate',
      rate: 150,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 40,
      maxVUs: 120,
      exec: 'writeGateDecision',
    },
  },
  thresholds: {
    'grpc_req_duration{method:WriteEntry}': ['p(99)<150'],
  },
};

function writeForType(entryType) {
  if (!connected) {
    client.connect(target, { plaintext: true });
    connected = true;
  }
  const now = Date.now();
  const uniq = `${exec.vu.idInTest}-${exec.vu.iterationInScenario}-${now}`;
  const payload = encoding.b64encode(`perf-${now}`, 'rawstd');
  const response = client.invoke('are.ledger.v1.ImmutableLedgerService/WriteEntry', {
    entryType,
    agentId: '11111111-1111-1111-1111-111111111111',
    content: payload,
    contentType: 'application/json',
    sourceId: 'ARE-FOUNDATION-PROOF',
    idempotencyKey: `perf-${entryType}-${uniq}`,
  });
  check(response, { 'write ok': (r) => r && r.status === grpc.StatusOK });
}

export function writeActionReceipt() { writeForType('LEDGER_ENTRY_TYPE_ACTION_RECEIPT'); }
export function writePolicyEval() { writeForType('LEDGER_ENTRY_TYPE_POLICY_EVAL'); }
export function writeAgentLifecycle() { writeForType('LEDGER_ENTRY_TYPE_AGENT_LIFECYCLE'); }
export function writeCredentialLifecycle() { writeForType('LEDGER_ENTRY_TYPE_CREDENTIAL_LIFECYCLE'); }
export function writePassportLifecycle() { writeForType('LEDGER_ENTRY_TYPE_PASSPORT_LIFECYCLE'); }
export function writeEscalation() { writeForType('LEDGER_ENTRY_TYPE_ESCALATION'); }
export function writeDriftEvent() { writeForType('LEDGER_ENTRY_TYPE_DRIFT_EVENT'); }
export function writeGateDecision() { writeForType('LEDGER_ENTRY_TYPE_GATE_DECISION'); }

export function teardown() {
  if (connected) {
    client.close();
  }
}

