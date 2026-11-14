package server

import (
	"testing"

	"buf.build/go/protovalidate"

	pb "auth-service/pkg/pb"
)

// The login role is optional (taken from the account); a role that is sent must still be a known one.
func TestLoginRequestRoleIsOptional(t *testing.T) {
	ok := &pb.LoginRequest{Username: "alice", Password: "password"}
	if err := protovalidate.Validate(ok); err != nil {
		t.Fatalf("empty role must pass: %v", err)
	}
	bad := &pb.LoginRequest{Username: "alice", Password: "password", Role: "root"}
	if err := protovalidate.Validate(bad); err == nil {
		t.Fatal("unknown role must fail")
	}
}
