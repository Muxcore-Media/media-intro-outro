package internal

import (
	"fmt"
	"strconv"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

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
	}
}

func (m *Module) UpdateSetting(key, value string) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("invalid number: %w", err)
	}
	if f <= 0 {
		return fmt.Errorf("must be > 0")
	}
	switch key {
	case "intro_max_seconds":
		m.introMax = f
	case "outro_max_seconds":
		m.outroMax = f
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}
