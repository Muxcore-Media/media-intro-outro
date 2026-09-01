package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

type DetectInput struct { //nolint:govet // field order matches RPC input grouping
	MediaID          string
	Path             string
	Chapters         []Chapter
	DurationSeconds  float64
	IntroMaxSeconds  float64
	OutroMaxSeconds  float64
	HeuristicEnabled bool
	MinConfidence    float64
	SeriesLookup     func(mediaID string) []Segment
}

type Store struct { //nolint:govet // mu guards byID and dataDir together
	byID    map[string][]Segment
	mu      sync.RWMutex
	dataDir string
}

func NewStore(dataDir string) *Store {
	return &Store{byID: map[string][]Segment{}, dataDir: strings.TrimSpace(dataDir)}
}

func (s *Store) Load() error {
	if s.dataDir == "" {
		return nil
	}
	path := filepath.Join(s.dataDir, "segments.json")
	raw, err := os.ReadFile(path) //nolint:gosec // path is segments.json under dataDir
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read segments: %w", err)
	}
	var data map[string][]Segment
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse segments: %w", err)
	}
	s.mu.Lock()
	s.byID = data
	if s.byID == nil {
		s.byID = map[string][]Segment{}
	}
	s.mu.Unlock()
	return nil
}

func (s *Store) flush() error {
	if s.dataDir == "" {
		return nil
	}
	s.mu.RLock()
	data := make(map[string][]Segment, len(s.byID))
	for id, segs := range s.byID {
		data[id] = append([]Segment(nil), segs...)
	}
	s.mu.RUnlock()
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal segments: %w", err)
	}
	raw = append(raw, '\n')
	path := filepath.Join(s.dataDir, "segments.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil { //nolint:gosec // tmp is segments.json.tmp under dataDir
		return fmt.Errorf("write segments: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil { //nolint:gosec // tmp and path are both under dataDir
		return fmt.Errorf("rename segments: %w", err)
	}
	return nil
}

func (s *Store) Set(mediaID string, segs []Segment) error {
	if strings.TrimSpace(mediaID) == "" {
		return fmt.Errorf("media_id required")
	}
	if err := validateSegments(segs); err != nil {
		return err
	}
	cp := append([]Segment(nil), segs...)
	s.mu.Lock()
	s.byID[mediaID] = cp
	s.mu.Unlock()
	return s.flush()
}

func (s *Store) Get(mediaID string) []Segment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Segment(nil), s.byID[mediaID]...)
}

func (s *Store) Delete(mediaID string) error {
	s.mu.Lock()
	if _, ok := s.byID[mediaID]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("media_id %q not found", mediaID)
	}
	delete(s.byID, mediaID)
	s.mu.Unlock()
	return s.flush()
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

func (s *Store) Detect(in DetectInput) ([]Segment, error) {
	in.SeriesLookup = s.lookupSeriesSegments
	return Detect(in)
}

func (s *Store) lookupSeriesSegments(mediaID string) []Segment {
	prefix := seriesPrefix(mediaID)
	if prefix == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for id, segs := range s.byID {
		if id == mediaID || seriesPrefix(id) != prefix || len(segs) == 0 {
			continue
		}
		out := make([]Segment, 0, len(segs))
		for _, seg := range segs {
			if !isSkippableKind(seg.Kind) {
				continue
			}
			cp := seg
			cp.Source = "series"
			out = append(out, cp)
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

var seriesEpisodeRE = regexp.MustCompile(`(?i)^(.+?)[-_.]?s\d+e\d+$`)

func seriesPrefix(mediaID string) string {
	id := strings.TrimSpace(mediaID)
	if m := seriesEpisodeRE.FindStringSubmatch(id); len(m) > 1 {
		return strings.TrimRight(m[1], "-_.")
	}
	return ""
}

var (
	opWordRE = regexp.MustCompile(`\bop\b`)
	edWordRE = regexp.MustCompile(`\bed\b`)
)

// ClassifyChapter maps a chapter title to a skippable segment kind (exported for tests).
func ClassifyChapter(title string) string {
	return classifyChapter(title)
}

// ValidateSegments checks segment kinds and time bounds (exported for tests).
func ValidateSegments(segs []Segment) error {
	return validateSegments(segs)
}

func validateSegments(segs []Segment) error {
	for i, seg := range segs {
		kind := strings.ToLower(strings.TrimSpace(seg.Kind))
		switch kind {
		case "intro", "outro", "credits", "recap":
		default:
			return fmt.Errorf("segment %d: invalid kind %q (want intro|outro|credits|recap)", i, seg.Kind)
		}
		if seg.StartSeconds < 0 || seg.EndSeconds < 0 {
			return fmt.Errorf("segment %d: times must be non-negative", i)
		}
		if seg.EndSeconds <= seg.StartSeconds {
			return fmt.Errorf("segment %d: end_seconds must be > start_seconds", i)
		}
	}
	return nil
}

// Detect builds intro/outro segments from chapters, series reuse, and duration heuristics.
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
	minConf := in.MinConfidence
	if minConf < 0 {
		minConf = 0.5
	}
	heuristicEnabled := in.HeuristicEnabled
	if len(in.Chapters) == 0 && in.SeriesLookup != nil {
		if copied := in.SeriesLookup(in.MediaID); len(copied) > 0 {
			return copied, nil
		}
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
		out = append(out, Segment{
			Kind: kind, StartSeconds: ch.StartSeconds, EndSeconds: end,
			Confidence: 0.9, Source: "chapters",
		})
	}

	hasIntro, hasOutro, hasRecap := false, false, false
	for _, seg := range out {
		switch seg.Kind {
		case "intro":
			hasIntro = true
		case "outro", "credits":
			hasOutro = true
		case "recap":
			hasRecap = true
		}
	}

	if heuristicEnabled && !hasIntro && !hasRecap && in.DurationSeconds > 0 {
		end := introMax
		if in.DurationSeconds < end*2 {
			end = in.DurationSeconds * 0.08
		}
		if end > 5 {
			conf := 0.35
			if conf >= minConf {
				out = append(out, Segment{
					Kind: "intro", StartSeconds: 0, EndSeconds: end,
					Confidence: conf, Source: "heuristic",
				})
			}
		}
	}
	if heuristicEnabled && !hasOutro && in.DurationSeconds > outroMax {
		start := in.DurationSeconds - outroMax
		if start < in.DurationSeconds*0.7 {
			start = in.DurationSeconds * 0.85
		}
		conf := 0.35
		if conf >= minConf {
			out = append(out, Segment{
				Kind: "outro", StartSeconds: start, EndSeconds: in.DurationSeconds,
				Confidence: conf, Source: "heuristic",
			})
		}
	}
	if out == nil {
		out = []Segment{}
	}
	return out, nil
}

func classifyChapter(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	if opWordRE.MatchString(t) ||
		strings.Contains(t, "title sequence") ||
		strings.Contains(t, "opening theme") {
		return "intro"
	}
	if edWordRE.MatchString(t) {
		return "outro"
	}
	switch {
	case strings.Contains(t, "intro"), strings.Contains(t, "opening"):
		return "intro"
	case strings.Contains(t, "outro"), strings.Contains(t, "ending"):
		return "outro"
	case strings.Contains(t, "credit"):
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
