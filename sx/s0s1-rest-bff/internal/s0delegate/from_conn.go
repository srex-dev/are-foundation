package s0delegate

import (
	registryv1 "github.com/srex-dev/are-foundation/s0/agent-registry-service/proto"
	passportv1 "github.com/srex-dev/are-foundation/s0/passport-issuance-engine/proto"
	"google.golang.org/grpc"
)

// FromClientConn returns a Backend that owns c. Both registry and passport clients use the
// same connection (typical when one in-process test server registers both services).
func FromClientConn(c *grpc.ClientConn) *Backend {
	if c == nil {
		return &Backend{}
	}
	return &Backend{
		Reg:   registryv1.NewAgentRegistryServiceClient(c),
		Pass:  passportv1.NewPassportIssuanceServiceClient(c),
		conns: []*grpc.ClientConn{c},
	}
}
