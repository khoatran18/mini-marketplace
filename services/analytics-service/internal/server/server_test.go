package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"analytics-service/internal/service"
	"analytics-service/internal/store"
	pb "analytics-service/pkg/pb"
)

type sink struct{ n int }

func (s *sink) Add(string, any) { s.n++ }

func TestIngestEventsMapsAndValidates(t *testing.T) {
	out := &sink{}
	srv := &Server{Service: &service.Service{Sink: out, Now: time.Now}}
	res, err := srv.IngestEvents(context.Background(), &pb.IngestEventsRequest{Events: []*pb.TrackEvent{
		{EventType: "page_view", AnalyticsConsent: true, AnonymousId: "a", Path: "/"},
		{EventType: "order_placed", AnalyticsConsent: true},
	}})
	if err != nil || res.Accepted != 1 || res.Rejected != 1 || len(res.Reasons) != 1 || out.n != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	// > 50 events per call is refused by the proto validation rule
	many := make([]*pb.TrackEvent, 51)
	for i := range many {
		many[i] = &pb.TrackEvent{EventType: "page_view"}
	}
	if _, err := srv.IngestEvents(context.Background(), &pb.IngestEventsRequest{Events: many}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

func TestQueryValidation(t *testing.T) {
	srv := &Server{Service: &service.Service{Q: &store.Queries{}, Now: time.Now}}
	ctx := context.Background()
	if _, err := srv.Query(ctx, &pb.QueryRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty report: %v", err)
	}
	if _, err := srv.Query(ctx, &pb.QueryRequest{Report: "summary", From: "yesterday", Scope: &pb.Scope{Role: "admin"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad time: %v", err)
	}
	if _, err := srv.Query(ctx, &pb.QueryRequest{Report: "summary"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("no scope means no access: %v", err)
	}
}
