package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	iov1 "github.com/Muxcore-Media/media-intro-outro/proto/gen/muxcore/introoutro/v1"
)

type Module struct {
	id, grpcAddr, httpAddr string
	introMax, outroMax     float64
	cfgMu                  sync.RWMutex
	store                  *Store
	grpcSrv                *grpc.Server
	lis                    net.Listener
	httpSrv                *http.Server
}

type Config struct {
	ID, GRPCAddr, HTTPAddr string
	IntroMax, OutroMax     float64
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "media-intro-outro"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9710"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9711"
	}
	if cfg.IntroMax <= 0 {
		cfg.IntroMax = 180
	}
	if cfg.OutroMax <= 0 {
		cfg.OutroMax = 240
	}
	if v := os.Getenv("INTRO_MAX_SECONDS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.IntroMax = f
		}
	}
	if v := os.Getenv("OUTRO_MAX_SECONDS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.OutroMax = f
		}
	}
	if v := os.Getenv("MUXCORE_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	return &Module{
		id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr,
		introMax: cfg.IntroMax, outroMax: cfg.OutroMax, store: NewStore(),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "Intro / Outro Detection", Version: "0.1.0",
		Roles:        []string{"media", "analysis"},
		Description:  "Intro/outro skip segment detection (scaffold)",
		Capabilities: []string{"media.intro_outro", "intro_outro", "settings"},
		HTTPAddr:     m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error { return nil }

func (m *Module) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	m.grpcSrv = grpc.NewServer()
	iov1.RegisterIntroOutroServiceServer(m.grpcSrv, &ioServer{m: m})
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("intro-outro gRPC listening", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(lis); err != nil {
			slog.Error("gRPC serve", "error", err)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	m.httpSrv = &http.Server{Addr: m.httpAddr, Handler: mux}
	go func() {
		slog.Info("health listening", "addr", m.httpAddr)
		if err := m.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health serve", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	return nil
}

func (m *Module) Health(ctx context.Context) error { return nil }

type ioServer struct {
	iov1.UnimplementedIntroOutroServiceServer
	m *Module
}

func (s *ioServer) Detect(_ context.Context, req *iov1.DetectRequest) (*iov1.DetectResponse, error) {
	s.m.cfgMu.RLock()
	introMax, outroMax := s.m.introMax, s.m.outroMax
	s.m.cfgMu.RUnlock()
	chapters := make([]Chapter, 0, len(req.GetChapters()))
	for _, c := range req.GetChapters() {
		chapters = append(chapters, Chapter{
			Title: c.GetTitle(), StartSeconds: c.GetStartSeconds(), EndSeconds: c.GetEndSeconds(),
		})
	}
	segs, err := Detect(DetectInput{
		MediaID: req.GetMediaId(), Path: req.GetPath(), DurationSeconds: req.GetDurationSeconds(),
		Chapters: chapters, IntroMaxSeconds: introMax, OutroMaxSeconds: outroMax,
	})
	if err != nil {
		return nil, err
	}
	if req.GetPersist() {
		if err := s.m.store.Set(req.GetMediaId(), segs); err != nil {
			return nil, err
		}
	}
	return &iov1.DetectResponse{MediaId: req.GetMediaId(), Segments: toPBSegs(segs)}, nil
}

func (s *ioServer) GetSegments(_ context.Context, req *iov1.GetSegmentsRequest) (*iov1.GetSegmentsResponse, error) {
	return &iov1.GetSegmentsResponse{MediaId: req.GetMediaId(), Segments: toPBSegs(s.m.store.Get(req.GetMediaId()))}, nil
}

func (s *ioServer) SetSegments(_ context.Context, req *iov1.SetSegmentsRequest) (*iov1.SetSegmentsResponse, error) {
	segs := fromPBSegs(req.GetSegments())
	for i := range segs {
		if segs[i].Source == "" {
			segs[i].Source = "manual"
		}
	}
	if err := s.m.store.Set(req.GetMediaId(), segs); err != nil {
		return nil, err
	}
	return &iov1.SetSegmentsResponse{MediaId: req.GetMediaId(), Segments: toPBSegs(segs)}, nil
}

func (s *ioServer) DeleteSegments(_ context.Context, req *iov1.DeleteSegmentsRequest) (*iov1.DeleteSegmentsResponse, error) {
	if err := s.m.store.Delete(req.GetMediaId()); err != nil {
		return nil, err
	}
	return &iov1.DeleteSegmentsResponse{Success: true}, nil
}

func (s *ioServer) ListMedia(_ context.Context, _ *iov1.ListMediaRequest) (*iov1.ListMediaResponse, error) {
	return &iov1.ListMediaResponse{MediaIds: s.m.store.ListMedia()}, nil
}

func toPBSegs(in []Segment) []*iov1.Segment {
	out := make([]*iov1.Segment, 0, len(in))
	for _, s := range in {
		out = append(out, &iov1.Segment{
			Kind: s.Kind, StartSeconds: s.StartSeconds, EndSeconds: s.EndSeconds,
			Confidence: s.Confidence, Source: s.Source,
		})
	}
	return out
}

func fromPBSegs(in []*iov1.Segment) []Segment {
	out := make([]Segment, 0, len(in))
	for _, s := range in {
		out = append(out, Segment{
			Kind: s.GetKind(), StartSeconds: s.GetStartSeconds(), EndSeconds: s.GetEndSeconds(),
			Confidence: s.GetConfidence(), Source: s.GetSource(),
		})
	}
	return out
}
