package internal

import (
	"context"
	"testing"

	iov1 "github.com/Muxcore-Media/media-intro-outro/proto/gen/muxcore/introoutro/v1"
)

func TestSkipRPC(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	srv := &ioServer{m: m}
	if _, err := srv.SetSegments(context.Background(), &iov1.SetSegmentsRequest{
		MediaId: "rpc-skip",
		Segments: []*iov1.Segment{
			{Kind: "intro", StartSeconds: 0, EndSeconds: 75, Confidence: 0.9, Source: "manual"},
			{Kind: "outro", StartSeconds: 1100, EndSeconds: 1200, Confidence: 0.9, Source: "manual"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	ok, err := srv.Skip(context.Background(), &iov1.SkipRequest{
		MediaId: "rpc-skip", PositionSeconds: 20, Kind: "intro",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok.GetCanSkip() || ok.GetSeekToSeconds() != 75 || ok.GetSegment().GetKind() != "intro" {
		t.Fatalf("%+v", ok)
	}

	no, err := srv.Skip(context.Background(), &iov1.SkipRequest{
		MediaId: "rpc-skip", PositionSeconds: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if no.GetCanSkip() {
		t.Fatalf("expected no skip: %+v", no)
	}

	_, err = srv.Skip(context.Background(), &iov1.SkipRequest{MediaId: "", PositionSeconds: 1})
	if err == nil {
		t.Fatal("expected media_id required")
	}
}
