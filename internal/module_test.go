package internal

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	iov1 "github.com/Muxcore-Media/media-intro-outro/proto/gen/muxcore/introoutro/v1"
)

func TestDetectRPCPersistGetDeleteList(t *testing.T) {
	dir := t.TempDir()
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	m.store = NewStore(dir)
	if err := m.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := &ioServer{m: m}

	detect, err := srv.Detect(context.Background(), &iov1.DetectRequest{
		MediaId: "rpc-detect", DurationSeconds: 1400, Persist: true,
		Chapters: []*iov1.Chapter{
			{Title: "Opening Credits", StartSeconds: 0, EndSeconds: 90},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(detect.GetSegments()) == 0 {
		t.Fatal("expected segments")
	}

	got, err := srv.GetSegments(context.Background(), &iov1.GetSegmentsRequest{MediaId: "rpc-detect"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetSegments()) == 0 {
		t.Fatal("GetSegments empty after persist")
	}

	list, err := srv.ListMedia(context.Background(), &iov1.ListMediaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetMediaIds()) != 1 {
		t.Fatalf("ListMedia: %+v", list.GetMediaIds())
	}

	reloaded := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	reloaded.store = NewStore(dir)
	if err := reloaded.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	reloadSrv := &ioServer{m: reloaded}
	again, err := reloadSrv.GetSegments(context.Background(), &iov1.GetSegmentsRequest{MediaId: "rpc-detect"})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.GetSegments()) == 0 {
		t.Fatal("segments not reloaded from disk")
	}

	if _, err := srv.DeleteSegments(context.Background(), &iov1.DeleteSegmentsRequest{MediaId: "rpc-detect"}); err != nil {
		t.Fatal(err)
	}
	empty, err := srv.GetSegments(context.Background(), &iov1.GetSegmentsRequest{MediaId: "rpc-detect"})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.GetSegments()) != 0 {
		t.Fatalf("expected empty after delete: %+v", empty.GetSegments())
	}
}

func TestStartHealthz(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: addr})
	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(context.Background()) }()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		resp, err = client.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if resp == nil {
		t.Fatal("healthz unreachable")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("healthz: status=%d body=%q", resp.StatusCode, body)
	}
}

func TestUpdateSettingChangesDetectWindow(t *testing.T) {
	dir := t.TempDir()
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	m.store = NewStore(dir)
	if err := m.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := &ioServer{m: m}
	if err := m.UpdateSetting("min_confidence", "0"); err != nil {
		t.Fatal(err)
	}

	before, err := srv.Detect(context.Background(), &iov1.DetectRequest{
		MediaId: "settings-detect", DurationSeconds: 7200,
	})
	if err != nil {
		t.Fatal(err)
	}
	var introBefore float64
	for _, seg := range before.GetSegments() {
		if seg.GetKind() == "intro" && seg.GetSource() == "heuristic" {
			introBefore = seg.GetEndSeconds()
		}
	}
	if introBefore == 0 {
		t.Fatal("expected heuristic intro before settings change")
	}

	if err := m.UpdateSetting("intro_max_seconds", "60"); err != nil {
		t.Fatal(err)
	}

	after, err := srv.Detect(context.Background(), &iov1.DetectRequest{
		MediaId: "settings-detect-2", DurationSeconds: 7200,
	})
	if err != nil {
		t.Fatal(err)
	}
	var introAfter float64
	for _, seg := range after.GetSegments() {
		if seg.GetKind() == "intro" && seg.GetSource() == "heuristic" {
			introAfter = seg.GetEndSeconds()
		}
	}
	if introAfter != 60 {
		t.Fatalf("intro window after setting change: got %g want 60", introAfter)
	}

	reloaded := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	reloaded.store = NewStore(dir)
	if err := reloaded.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	reloaded.cfgMu.RLock()
	gotMax := reloaded.introMax
	reloaded.cfgMu.RUnlock()
	if gotMax != 60 {
		t.Fatalf("settings not persisted: intro_max=%g", gotMax)
	}
}

func TestInvalidIntroMaxEnvIgnored(t *testing.T) {
	t.Setenv("INTRO_MAX_SECONDS", "not-a-number")
	m := NewModule(Config{})
	if m.introMax != 180 {
		t.Fatalf("expected default intro max, got %g", m.introMax)
	}
}
