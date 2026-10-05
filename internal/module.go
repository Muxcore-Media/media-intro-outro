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
	"time"

	manifest "github.com/Muxcore-Media/media-intro-outro"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/core/sdk/go/module/meshtls"
	iov1 "github.com/Muxcore-Media/media-intro-outro/proto/gen/muxcore/introoutro/v1"
)

type Module struct {
	lis              net.Listener
	store            *Store
	grpcSrv          *grpc.Server
	httpSrv          *http.Server
	id               string
	grpcAddr         string
	httpAddr         string
	introMax         float64
	outroMax         float64
	heuristicEnabled bool
	minConfidence    float64
	cfgMu            sync.RWMutex
}

type Config struct {
	ID, GRPCAddr, HTTPAddr string
	IntroMax, OutroMax     float64
	HeuristicEnabled       bool
	MinConfidence          float64
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
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.5
	}
	cfg.HeuristicEnabled = true

	if v := os.Getenv("INTRO_MAX_SECONDS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		switch {
		case err != nil:
			slog.Warn("invalid INTRO_MAX_SECONDS, using default", "value", v, "error", err)
		case f <= 0:
			slog.Warn("invalid INTRO_MAX_SECONDS, using default", "value", v)
		default:
			cfg.IntroMax = f
		}
	}
	if v := os.Getenv("OUTRO_MAX_SECONDS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		switch {
		case err != nil:
			slog.Warn("invalid OUTRO_MAX_SECONDS, using default", "value", v, "error", err)
		case f <= 0:
			slog.Warn("invalid OUTRO_MAX_SECONDS, using default", "value", v)
		default:
			cfg.OutroMax = f
		}
	}
	if v := os.Getenv("MUXCORE_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	dataDir := os.Getenv("INTRO_OUTRO_DATA_DIR")
	return &Module{
		id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr,
		introMax: cfg.IntroMax, outroMax: cfg.OutroMax,
		heuristicEnabled: cfg.HeuristicEnabled, minConfidence: cfg.MinConfidence,
		store: NewStore(dataDir),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "Intro / Outro Detection", Version: modulesdk.ManifestVersion(manifest.ManifestJSON),
		Roles:        []string{"media", "analysis"},
		Description:  "Intro/outro skip segment detection",
		Capabilities: []string{"media.intro_outro", "intro_outro", "settings"},
		HTTPAddr:     m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	if m.store.dataDir == "" {
		return nil
	}
	if err := os.MkdirAll(m.store.dataDir, 0o700); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	if err := m.store.Load(); err != nil {
		return fmt.Errorf("load segments: %w", err)
	}
	if err := m.loadSettings(); err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	srvOpt, err := meshtls.ServerOption()
	if err != nil {
		return fmt.Errorf("grpc mesh TLS: %w", err)
	}
	m.grpcSrv = grpc.NewServer(srvOpt)
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
	m.httpSrv = &http.Server{
		Addr:              m.httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
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

func (s *ioServer) detectInput(req *iov1.DetectRequest) DetectInput {
	s.m.cfgMu.RLock()
	introMax, outroMax := s.m.introMax, s.m.outroMax
	heuristicEnabled, minConf := s.m.heuristicEnabled, s.m.minConfidence
	s.m.cfgMu.RUnlock()
	chapters := make([]Chapter, 0, len(req.GetChapters()))
	for _, c := range req.GetChapters() {
		chapters = append(chapters, Chapter{
			Title: c.GetTitle(), StartSeconds: c.GetStartSeconds(), EndSeconds: c.GetEndSeconds(),
		})
	}
	return DetectInput{
		MediaID: req.GetMediaId(), Path: req.GetPath(), DurationSeconds: req.GetDurationSeconds(),
		Chapters: chapters, IntroMaxSeconds: introMax, OutroMaxSeconds: outroMax,
		HeuristicEnabled: heuristicEnabled, MinConfidence: minConf,
	}
}

func (s *ioServer) Detect(_ context.Context, req *iov1.DetectRequest) (*iov1.DetectResponse, error) {
	segs, err := s.m.store.Detect(s.detectInput(req))
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
	if err := validateSegments(segs); err != nil {
		return nil, err
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

func (s *ioServer) Skip(_ context.Context, req *iov1.SkipRequest) (*iov1.SkipResponse, error) {
	res, err := s.m.store.SkipForMedia(req.GetMediaId(), req.GetPositionSeconds(), req.GetKind())
	if err != nil {
		return nil, err
	}
	out := &iov1.SkipResponse{CanSkip: res.CanSkip, SeekToSeconds: res.SeekToSeconds}
	if res.Segment != nil {
		out.Segment = &iov1.Segment{
			Kind: res.Segment.Kind, StartSeconds: res.Segment.StartSeconds, EndSeconds: res.Segment.EndSeconds,
			Confidence: res.Segment.Confidence, Source: res.Segment.Source,
		}
	}
	return out, nil
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
