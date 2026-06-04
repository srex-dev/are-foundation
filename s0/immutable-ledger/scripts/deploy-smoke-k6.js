import grpc from 'k6/net/grpc';
import encoding from 'k6/encoding';
import { check } from 'k6';

const client = new grpc.Client();
client.load(['/work/proto'], 'immutable_ledger.proto');

const target = __ENV.K6_TARGET || 'host.docker.internal:9092';

export const options = { vus: 1, iterations: 1 };

export default function () {
  client.connect(target, { plaintext: true });

  const payload = encoding.b64encode(`smoke-${Date.now()}`, 'rawstd');
  const write = client.invoke('are.ledger.v1.ImmutableLedgerService/WriteEntry', {
    entryType: 'LEDGER_ENTRY_TYPE_ACTION_RECEIPT',
    agentId: '11111111-1111-1111-1111-111111111111',
    content: payload,
    contentType: 'application/json',
    sourceId: 'ARE-FOUNDATION-PROOF',
    idempotencyKey: `smoke-${Date.now()}`,
  });
  check(write, { 'write ok': (r) => r && r.status === grpc.StatusOK });

  const get = client.invoke('are.ledger.v1.ImmutableLedgerService/GetEntry', {
    entryId: write.message.entryId,
  });
  check(get, { 'get ok': (r) => r && r.status === grpc.StatusOK });

  const verify = client.invoke('are.ledger.v1.ImmutableLedgerService/VerifyEntry', {
    entryId: write.message.entryId,
  });
  check(verify, { 'verify ok': (r) => r && r.status === grpc.StatusOK });
  check(verify, { 'verify valid': (r) => r && r.message.hashValid === true && r.message.chainLinkValid === true });

  client.close();
}

