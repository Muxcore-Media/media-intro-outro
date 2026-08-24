package internal

import (
	"fmt"
	"strings"
	"sync"
)

type Segment struct {
	Kind         string  `json:"kind"`
	Source       string  `json:"source"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Confidence   float64 `json:"confidence"`
}

type Chapter struct {
	Title        string  `json:"title"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
}

type DetectInput struct {
	MediaID         string
	Path            string
	Chapters        []Chapter
	DurationSeconds float64
	IntroMaxSeconds float64
	OutroMaxSeconds float64
}

type Store struct {
	byID map[string][]Segment
	mu   sync.RWMutex
}

func NewStore() *Store {
	return &Store{byID: map[string][]Segment{}}
}

func (s *Store) Set(mediaID string, segs []Segment) error {
	if strings.TrimSpace(mediaID) == "" {
		return fmt.Errorf("media_id required")
	}
	cp := append([]Segment(nil), segs...)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[mediaID] = cp
	return nil
}

func (s *Store) Get(mediaID string) []Segment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Segment(nil), s.byID[mediaID]...)
}

func (s *Store) Delete(mediaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[mediaID]; !ok {
		return fmt.Errorf("media_id %q not found", mediaID)
	}
	delete(s.byID, mediaID)
	return nil
}

func (s *Store) ListMedia() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.byID))
	for id := range s.byID {
		out = append(out, id)
	}
	return out
}

// Detect builds intro/outro segments from chapters and duration heuristics.
func Detect(in DetectInput) ([]Segment, error) {
	if strings.TrimSpace(in.MediaID) == "" {
		return nil, fmt.Errorf("media_id required")
	}
	introMax := in.IntroMaxSeconds
	if introMax <= 0 {
		introMax = 180
	}
	outroMax := in.OutroMaxSeconds
	if outroMax <= 0 {
		outroMax = 240
	}

	var out []Segment
	for _, ch := range in.Chapters {
		kind := classifyChapter(ch.Title)
		if kind == "" {
			continue
		}
		end := ch.EndSeconds
		if end <= ch.StartSeconds {
			end = ch.StartSeconds + 30
		}
		conf := 0.9
		out = append(out, Segment{
			Kind: kind, StartSeconds: ch.StartSeconds, EndSeconds: end,
			Confidence: conf, Source: "chapters",
		})
	}

	hasIntro, hasOutro := false, false
	for _, seg := range out {
		if seg.Kind == "intro" {
			hasIntro = true
		}
		if seg.Kind == "outro" || seg.Kind == "credits" {
			hasOutro = true
		}
	}

	if !hasIntro && in.DurationSeconds > 0 {
		end := introMax
		if in.DurationSeconds < end*2 {
			end = in.DurationSeconds * 0.08
		}
		if end > 5 {
			out = append(out, Segment{
				Kind: "intro", StartSeconds: 0, EndSeconds: end,
				Confidence: 0.35, Source: "heuristic",
			})
		}
	}
	if !hasOutro && in.DurationSeconds > outroMax {
		start := in.DurationSeconds - outroMax
		if start < in.DurationSeconds*0.7 {
			start = in.DurationSeconds * 0.85
		}
		out = append(out, Segment{
			Kind: "outro", StartSeconds: start, EndSeconds: in.DurationSeconds,
			Confidence: 0.35, Source: "heuristic",
		})
	}
	if out == nil {
		out = []Segment{}
	}
	return out, nil
}

func classifyChapter(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	switch {
	case strings.Contains(t, "intro"), strings.Contains(t, "opening"), strings.Contains(t, "op "):
		return "intro"
	case strings.Contains(t, "outro"), strings.Contains(t, "ending"), strings.Contains(t, "ed "):
		return "outro"
	case strings.Contains(t, "credit"), strings.Contains(t, "cast"):
		return "credits"
	case strings.Contains(t, "recap"), strings.Contains(t, "previously"):
		return "recap"
	default:
		return ""
	}
}

// SkipResult is the player-facing seek hint for a playback position.
type SkipResult struct {
	Segment       *Segment
	SeekToSeconds float64
	CanSkip       bool
}

func isSkippableKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "intro", "outro", "credits", "recap":
		return true
	default:
		return false
	}
}

// Skip finds a skippable segment containing positionSeconds.
// kindFilter is optional (intro|outro|credits|recap); empty matches any skippable kind.
func Skip(segs []Segment, positionSeconds float64, kindFilter string) SkipResult {
	want := strings.ToLower(strings.TrimSpace(kindFilter))
	var best *Segment
	for i := range segs {
		seg := &segs[i]
		if !isSkippableKind(seg.Kind) {
			continue
		}
		if want != "" && !strings.EqualFold(seg.Kind, want) {
			continue
		}
		if positionSeconds < seg.StartSeconds || positionSeconds >= seg.EndSeconds {
			continue
		}
		if best == nil || seg.EndSeconds < best.EndSeconds {
			best = seg
		}
	}
	if best == nil {
		return SkipResult{}
	}
	cp := *best
	return SkipResult{CanSkip: true, SeekToSeconds: best.EndSeconds, Segment: &cp}
}

// SkipForMedia looks up stored segments and returns a skip hint for positionSeconds.
func (s *Store) SkipForMedia(mediaID string, positionSeconds float64, kindFilter string) (SkipResult, error) {
	if strings.TrimSpace(mediaID) == "" {
		return SkipResult{}, fmt.Errorf("media_id required")
	}
	return Skip(s.Get(mediaID), positionSeconds, kindFilter), nil
}
