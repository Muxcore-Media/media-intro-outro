package internal_test

import (
	"testing"

	"github.com/Muxcore-Media/media-intro-outro/internal"
)

func TestDetectFromChapters(t *testing.T) {
	segs, err := internal.Detect(internal.DetectInput{
		MediaID: "ep_1", DurationSeconds: 1400,
		Chapters: []internal.Chapter{
			{Title: "Opening Credits", StartSeconds: 0, EndSeconds: 90},
			{Title: "Ending", StartSeconds: 1280, EndSeconds: 1400},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) < 2 {
		t.Fatalf("segs=%+v", segs)
	}
	kinds := map[string]bool{}
	for _, s := range segs {
		kinds[s.Kind] = true
		if s.Source != "chapters" {
			t.Fatalf("expected chapters source, got %s", s.Source)
		}
	}
	if !kinds["intro"] || !kinds["outro"] {
		t.Fatalf("kinds=%v", kinds)
	}
}

func TestDetectHeuristic(t *testing.T) {
	segs, err := internal.Detect(internal.DetectInput{
		MediaID: "mv_1", DurationSeconds: 7200, IntroMaxSeconds: 120, OutroMaxSeconds: 180,
		HeuristicEnabled: true, MinConfidence: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 {
		t.Fatalf("want intro+outro, got %+v", segs)
	}
	s := internal.NewStore("")
	if err := s.Set("mv_1", segs); err != nil {
		t.Fatal(err)
	}
	if len(s.Get("mv_1")) != 2 {
		t.Fatal("persist failed")
	}
}

func TestDetectRecapBlocksHeuristicIntro(t *testing.T) {
	segs, err := internal.Detect(internal.DetectInput{
		MediaID: "recap-ep", DurationSeconds: 2100,
		IntroMaxSeconds: 180, HeuristicEnabled: true, MinConfidence: 0,
		Chapters: []internal.Chapter{
			{Title: "Previously On", StartSeconds: 0, EndSeconds: 45},
			{Title: "Closing Credits", StartSeconds: 2000, EndSeconds: 2100},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, seg := range segs {
		if seg.Source == "heuristic" && seg.Kind == "intro" {
			t.Fatalf("heuristic intro must not overlap recap window: %+v", segs)
		}
	}
	hasRecap := false
	for _, seg := range segs {
		if seg.Kind == "recap" {
			hasRecap = true
		}
	}
	if !hasRecap {
		t.Fatalf("expected recap segment: %+v", segs)
	}
}

func TestClassifyChapter(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"OP Theme", "intro"},
		{"ED Theme", "outro"},
		{"Opening Theme Song", "intro"},
		{"Title Sequence", "intro"},
		{"Opening Credits", "intro"},
		{"Ending Credits", "outro"},
		{"Closing Credits", "credits"},
		{"Previously On", "recap"},
		{"Podcast Broadcast Cast", ""},
		{"Main Feature", ""},
	}
	for _, tc := range tests {
		got := internal.ClassifyChapter(tc.title)
		if got != tc.want {
			t.Errorf("classifyChapter(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestStorePersistReload(t *testing.T) {
	dir := t.TempDir()
	s1 := internal.NewStore(dir)
	segs := []internal.Segment{
		{Kind: "intro", StartSeconds: 0, EndSeconds: 90, Confidence: 0.9, Source: "manual"},
	}
	if err := s1.Set("persist-ep", segs); err != nil {
		t.Fatal(err)
	}
	s2 := internal.NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got := s2.Get("persist-ep")
	if len(got) != 1 || got[0].EndSeconds != 90 {
		t.Fatalf("reload: %+v", got)
	}
	if err := s2.Delete("persist-ep"); err != nil {
		t.Fatal(err)
	}
	s3 := internal.NewStore(dir)
	if err := s3.Load(); err != nil {
		t.Fatal(err)
	}
	if len(s3.Get("persist-ep")) != 0 {
		t.Fatal("expected delete persisted")
	}
}

func TestSeriesReuse(t *testing.T) {
	store := internal.NewStore("")
	ep1, err := internal.Detect(internal.DetectInput{
		MediaID: "show-s01e01", DurationSeconds: 1400,
		Chapters: []internal.Chapter{
			{Title: "Opening Credits", StartSeconds: 0, EndSeconds: 90},
			{Title: "Ending", StartSeconds: 1280, EndSeconds: 1400},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("show-s01e01", ep1); err != nil {
		t.Fatal(err)
	}
	ep2, err := store.Detect(internal.DetectInput{
		MediaID: "show-s01e02", DurationSeconds: 1400,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ep2) != 2 {
		t.Fatalf("series reuse: %+v", ep2)
	}
	if ep2[0].Source != "series" {
		t.Fatalf("expected series source, got %+v", ep2)
	}
}

func TestValidateSegments(t *testing.T) {
	if err := internal.ValidateSegments([]internal.Segment{
		{Kind: "intro", StartSeconds: 0, EndSeconds: 90},
	}); err != nil {
		t.Fatal(err)
	}
	if err := internal.ValidateSegments([]internal.Segment{
		{Kind: "bad", StartSeconds: 0, EndSeconds: 90},
	}); err == nil {
		t.Fatal("expected invalid kind error")
	}
	if err := internal.ValidateSegments([]internal.Segment{
		{Kind: "intro", StartSeconds: 10, EndSeconds: 10},
	}); err == nil {
		t.Fatal("expected end > start error")
	}
}
