// Package server is the gRPC layer.
package server

import (
	"context"
	"time"

	"buf.build/go/protovalidate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"analytics-service/internal/ingest"
	"analytics-service/internal/service"
	pb "analytics-service/pkg/pb"
)

// Server implements pb.AnalyticsServiceServer.
type Server struct {
	pb.UnimplementedAnalyticsServiceServer
	Service *service.Service
}

// IngestEvents queues tracking events (at most 50 per call).
func (s *Server) IngestEvents(_ context.Context, req *pb.IngestEventsRequest) (*pb.IngestEventsResponse, error) {
	if err := protovalidate.Validate(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	raws := make([]ingest.Raw, 0, len(req.Events))
	for _, e := range req.Events {
		raws = append(raws, ingest.Raw{
			EventID: e.EventId, EventType: e.EventType, TsClient: e.TsClient, AnonymousID: e.AnonymousId, SessionID: e.SessionId,
			Surface: e.Surface, Path: e.Path, Referrer: e.Referrer, PropsJSON: e.PropsJson, ProductID: e.ProductId,
			Position: e.Position, DeviceType: e.DeviceType, OS: e.Os, AppVersion: e.AppVersion, AnalyticsConsent: e.AnalyticsConsent,
			UserID: e.UserId, Role: e.Role, StoreID: e.StoreId, IPHash: e.IpHash, UAFamily: e.UaFamily, Country: e.Country,
		})
	}
	accepted, reasons := s.Service.Ingest(raws)
	return &pb.IngestEventsResponse{Accepted: int32(accepted), Rejected: int32(len(raws) - accepted), Reasons: reasons}, nil
}

// Query runs one report.
func (s *Server) Query(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	if err := protovalidate.Validate(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	r := service.Request{Report: req.Report, Granularity: req.Granularity, Sort: req.Sort, Limit: int(req.Limit)}
	if req.Scope != nil {
		r.Role, r.StoreID = req.Scope.Role, req.Scope.StoreId
	}
	for _, p := range []struct {
		in  string
		out *time.Time
	}{{req.From, &r.From}, {req.To, &r.To}} {
		if p.in == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, p.in)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "from/to must be RFC 3339")
		}
		*p.out = t
	}
	res, err := s.Service.Query(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.QueryResponse{AsOf: res.AsOf.Format(time.RFC3339), Source: res.Source, Cached: res.Cached, DataJson: string(res.Data)}, nil
}
