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
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 {
		t.Fatalf("want intro+outro, got %+v", segs)
	}
	s := internal.NewStore()
	if err := s.Set("mv_1", segs); err != nil {
		t.Fatal(err)
	}
	if len(s.Get("mv_1")) != 2 {
		t.Fatal("persist failed")
	}
}
