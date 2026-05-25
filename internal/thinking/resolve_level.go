// Package thinking provides unified thinking configuration processing.
package thinking

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

// ResolveThinkingLevel returns the normalized thinking level for a request when configured.
// It follows the same extraction and validation rules as ApplyThinking, but returns
// the effective level without mutating the request payload.
func ResolveThinkingLevel(body []byte, model, fromFormat, toFormat, providerKey string) (string, bool) {
	config, ok := resolveThinkingConfig(body, model, fromFormat, toFormat, providerKey)
	if !ok {
		return "", false
	}
	return thinkingLevelFromConfig(config)
}

func resolveThinkingConfig(body []byte, model, fromFormat, toFormat, providerKey string) (ThinkingConfig, bool) {
	providerFormat := strings.ToLower(strings.TrimSpace(toFormat))
	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	if providerKey == "" {
		providerKey = providerFormat
	}
	fromFormat = strings.ToLower(strings.TrimSpace(fromFormat))
	if fromFormat == "" {
		fromFormat = providerFormat
	}
	if GetProviderApplier(providerFormat) == nil {
		return ThinkingConfig{}, false
	}

	suffixResult := ParseSuffix(model)
	baseModel := suffixResult.ModelName
	modelInfo := registry.LookupModelInfo(baseModel, providerKey)

	if IsUserDefinedModel(modelInfo) {
		var config ThinkingConfig
		if suffixResult.HasSuffix {
			config = parseSuffixToConfig(suffixResult.RawSuffix, providerFormat, model)
		} else {
			config = extractThinkingConfig(body, fromFormat)
			if !hasThinkingConfig(config) && fromFormat != providerFormat {
				config = extractThinkingConfig(body, providerFormat)
			}
		}
		if !hasThinkingConfig(config) {
			return ThinkingConfig{}, false
		}
		config = normalizeUserDefinedConfig(config, fromFormat, providerFormat)
		return config, true
	}

	if modelInfo == nil || modelInfo.Thinking == nil {
		return ThinkingConfig{}, false
	}

	var config ThinkingConfig
	if suffixResult.HasSuffix {
		config = parseSuffixToConfig(suffixResult.RawSuffix, providerFormat, model)
	} else {
		config = extractThinkingConfig(body, providerFormat)
	}
	if !hasThinkingConfig(config) {
		return ThinkingConfig{}, false
	}
	validated, err := ValidateConfig(config, modelInfo, fromFormat, providerFormat, suffixResult.HasSuffix)
	if err != nil || validated == nil {
		return ThinkingConfig{}, false
	}
	return *validated, true
}

func thinkingLevelFromConfig(config ThinkingConfig) (string, bool) {
	switch config.Mode {
	case ModeNone:
		return string(LevelNone), true
	case ModeAuto:
		return string(LevelAuto), true
	case ModeLevel:
		level := strings.ToLower(strings.TrimSpace(string(config.Level)))
		if level == "" {
			return "", false
		}
		return level, true
	case ModeBudget:
		level, ok := ConvertBudgetToLevel(config.Budget)
		if !ok {
			return "", false
		}
		return level, true
	default:
		return "", false
	}
}
