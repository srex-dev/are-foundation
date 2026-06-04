package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	passportv1 "github.com/srex-dev/are-foundation/s0/passport-issuance-engine/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func main() {
	if err := run(); err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:9094", "passport gRPC address")
	operation := flag.String("operation", "verify", "verify or revoke")
	passportID := flag.String("passport-id", "", "passport id")
	payloadB64 := flag.String("payload-b64", "", "base64 verification payload")
	signatureB64 := flag.String("signature-b64", "", "base64 verification signature")
	reason := flag.String("reason", "passport_authority_probe", "revocation reason")
	revokedBy := flag.String("revoked-by", "passport-grpc-probe", "revoker id")
	flag.Parse()

	if *passportID == "" {
		return fmt.Errorf("passport-id required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	client := passportv1.NewPassportIssuanceServiceClient(conn)

	switch *operation {
	case "verify":
		payload, err := base64.StdEncoding.DecodeString(*payloadB64)
		if err != nil {
			return fmt.Errorf("payload-b64: %w", err)
		}
		signature, err := base64.StdEncoding.DecodeString(*signatureB64)
		if err != nil {
			return fmt.Errorf("signature-b64: %w", err)
		}
		resp, err := client.VerifyPassport(ctx, &passportv1.VerifyPassportRequest{
			PassportId: *passportID,
			Payload:    payload,
			Signature:  signature,
		})
		if err != nil {
			return err
		}
		return writeProto(resp)
	case "revoke":
		resp, err := client.RevokePassport(ctx, &passportv1.RevokePassportRequest{
			PassportId: *passportID,
			Reason:     *reason,
			RevokedBy:  *revokedBy,
		})
		if err != nil {
			return err
		}
		return writeProto(resp)
	default:
		return fmt.Errorf("unsupported operation %q", *operation)
	}
}

func writeProto(msg interface{ ProtoReflect() protoreflect.Message }) error {
	raw, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	body["ok"] = true
	return json.NewEncoder(os.Stdout).Encode(body)
}
