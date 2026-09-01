package internal_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/media-intro-outro/internal"
)

var updateGoldens = flag.Bool("update-goldens", false, "rewrite detect.golden.json fixtures")

type detectFixture struct {
	MediaID         string             `json:"media_id"`
	Path            string             `json:"path"`
	DurationSeconds float64            `json:"duration_seconds"`
	IntroMaxSeconds float64            `json:"intro_max_seconds"`
	OutroMaxSeconds float64            `json:"outro_max_seconds"`
	Chapters        []internal.Chapter `json:"chapters"`
}

// TestDetectGoldenFixtures runs Detect against local sample media sidecars and
// compares against committed golden JSON (offline; empty .mkv stubs only).
func TestDetectGoldenFixtures(t *testing.T) {
	root := filepath.Join("testdata", "samples")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("samples root missing: %v", err)
	}

	var cases int
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		dir := filepath.Join(root, ent.Name())
		inputPath := filepath.Join(dir, "detect_input.json")
		goldenPath := filepath.Join(dir, "detect.golden.json")
		if _, err := os.Stat(inputPath); err != nil {
			continue
		}
		cases++
		t.Run(ent.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			var fx detectFixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatalf("parse input: %v", err)
			}
			segs, err := internal.Detect(internal.DetectInput{
				MediaID: fx.MediaID, Path: fx.Path, DurationSeconds: fx.DurationSeconds,
				Chapters: fx.Chapters, IntroMaxSeconds: fx.IntroMaxSeconds, OutroMaxSeconds: fx.OutroMaxSeconds,
				MinConfidence: 0.5,
			})
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(segs, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			if *updateGoldens {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden missing (run with -update-goldens): %v", err)
			}
			if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
				t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					ent.Name(), got, want)
			}
		})
	}
	if cases < 5 {
		t.Fatalf("expected >=5 sample fixtures, found %d under %s", cases, root)
	}
}

func TestSkipAPI(t *testing.T) {
	segs, err := internal.Detect(internal.DetectInput{
		MediaID: "skip-ep", DurationSeconds: 1400,
		Chapters: []internal.Chapter{
			{Title: "Opening Credits", StartSeconds: 0, EndSeconds: 90},
			{Title: "Ending", StartSeconds: 1280, EndSeconds: 1400},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := internal.NewStore("")
	if err := store.Set("skip-ep", segs); err != nil {
		t.Fatal(err)
	}

	inIntro, err := store.SkipForMedia("skip-ep", 30, "intro")
	if err != nil {
		t.Fatal(err)
	}
	if !inIntro.CanSkip || inIntro.SeekToSeconds != 90 {
		t.Fatalf("intro skip: %+v", inIntro)
	}

	mid, err := store.SkipForMedia("skip-ep", 500, "")
	if err != nil {
		t.Fatal(err)
	}
	if mid.CanSkip {
		t.Fatalf("expected no skip mid-episode, got %+v", mid)
	}

	inOutro, err := store.SkipForMedia("skip-ep", 1300, "outro")
	if err != nil {
		t.Fatal(err)
	}
	if !inOutro.CanSkip || inOutro.SeekToSeconds != 1400 {
		t.Fatalf("outro skip: %+v", inOutro)
	}

	wrongKind, err := store.SkipForMedia("skip-ep", 30, "outro")
	if err != nil {
		t.Fatal(err)
	}
	if wrongKind.CanSkip {
		t.Fatalf("intro position with outro filter should not skip: %+v", wrongKind)
	}

	missing, err := store.SkipForMedia("no-such", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if missing.CanSkip {
		t.Fatalf("missing media should not skip: %+v", missing)
	}

	_, err = store.SkipForMedia("", 0, "")
	if err == nil {
		t.Fatal("expected media_id required")
	}
}
