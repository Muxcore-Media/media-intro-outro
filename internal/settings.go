package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

type persistedSettings struct {
	IntroMaxSeconds  float64 `json:"intro_max_seconds"`
	OutroMaxSeconds  float64 `json:"outro_max_seconds"`
	HeuristicEnabled bool    `json:"heuristic_enabled"`
	MinConfidence    float64 `json:"min_confidence"`
}

func (m *Module) settingsPath() string {
	if m.store == nil || m.store.dataDir == "" {
		return ""
	}
	return filepath.Join(m.store.dataDir, "settings.json")
}

func (m *Module) loadSettings() error {
	path := m.settingsPath()
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path) //nolint:gosec // path is module settings under dataDir
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read settings: %w", err)
	}
	var ps persistedSettings
	if err := json.Unmarshal(raw, &ps); err != nil {
		return fmt.Errorf("parse settings: %w", err)
	}
	m.cfgMu.Lock()
	if ps.IntroMaxSeconds > 0 {
		m.introMax = ps.IntroMaxSeconds
	}
	if ps.OutroMaxSeconds > 0 {
		m.outroMax = ps.OutroMaxSeconds
	}
	m.heuristicEnabled = ps.HeuristicEnabled
	if ps.MinConfidence > 0 {
		m.minConfidence = ps.MinConfidence
	}
	m.cfgMu.Unlock()
	return nil
}

func (m *Module) persistSettings() error {
	path := m.settingsPath()
	if path == "" {
		return nil
	}
	ps := persistedSettings{
		IntroMaxSeconds:  m.introMax,
		OutroMaxSeconds:  m.outroMax,
		HeuristicEnabled: m.heuristicEnabled,
		MinConfidence:    m.minConfidence,
	}
	raw, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil { //nolint:gosec // tmp and path are both under dataDir
		return fmt.Errorf("rename settings: %w", err)
	}
	return nil
}

func (m *Module) Settings() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{Key: "intro_max_seconds", Label: "Intro max (s)", Type: contracts.SettingTypeInt,
			Value: fmt.Sprintf("%g", m.introMax), Default: "180",
			Description: "Heuristic intro window; INTRO_MAX_SECONDS", Group: "Detection"},
		{Key: "outro_max_seconds", Label: "Outro max (s)", Type: contracts.SettingTypeInt,
			Value: fmt.Sprintf("%g", m.outroMax), Default: "240",
			Description: "Heuristic outro window; OUTRO_MAX_SECONDS", Group: "Detection"},
		{Key: "heuristic_enabled", Label: "Heuristic detection", Type: contracts.SettingTypeBool,
			Value: strconv.FormatBool(m.heuristicEnabled), Default: "true",
			Description: "Enable duration-based intro/outro when chapters are absent", Group: "Detection"},
		{Key: "min_confidence", Label: "Min confidence", Type: contracts.SettingTypeString,
			Value: fmt.Sprintf("%g", m.minConfidence), Default: "0.5",
			Description: "Minimum confidence to offer a segment to the player", Group: "Detection"},
	}
}

func (m *Module) UpdateSetting(key, value string) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	switch key {
	case "intro_max_seconds", "outro_max_seconds":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid number: %w", err)
		}
		if f <= 0 {
			return fmt.Errorf("must be > 0")
		}
		if key == "intro_max_seconds" {
			m.introMax = f
		} else {
			m.outroMax = f
		}
	case "heuristic_enabled":
		v := strings.EqualFold(value, "true") || value == "1"
		m.heuristicEnabled = v
	case "min_confidence":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid number: %w", err)
		}
		if f < 0 || f > 1 {
			return fmt.Errorf("must be between 0 and 1")
		}
		m.minConfidence = f
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return m.persistSettings()
}
