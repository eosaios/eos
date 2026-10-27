package utils

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"unicode"
)

type TokenEstimateCache struct {
	mu         sync.Mutex
	maxEntries int
	counter    uint64
	entries    map[uint64]tokenCacheEntry
}

type tokenCacheEntry struct {
	tokens   int
	lastUsed uint64
}

func NewTokenEstimateCache(maxEntries int) *TokenEstimateCache {
	if maxEntries <= 0 {
		maxEntries = 1024
	}
	return &TokenEstimateCache{
		maxEntries: maxEntries,
		entries:    make(map[uint64]tokenCacheEntry, maxEntries),
	}
}

func (c *TokenEstimateCache) Get(key uint64) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return 0, false
	}
	c.counter++
	e.lastUsed = c.counter
	c.entries[key] = e
	return e.tokens, true
}

func (c *TokenEstimateCache) Put(key uint64, tokens int) {
	if tokens < 0 {
		tokens = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	c.entries[key] = tokenCacheEntry{tokens: tokens, lastUsed: c.counter}
	if len(c.entries) <= c.maxEntries {
		return
	}
	var oldestKey uint64
	var oldestUsed uint64
	first := true
	for k, e := range c.entries {
		if first || e.lastUsed < oldestUsed {
			first = false
			oldestKey = k
			oldestUsed = e.lastUsed
		}
	}
	delete(c.entries, oldestKey)
}

func TokenEstimateKey(model string, text string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(model))))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(text))
	return h.Sum64()
}

func EstimateTokensWeighted(model string, text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}

	p := tokenProfileForModel(model, text)

	var cjk, latin, digit, space, symbol, other int
	for _, r := range text {
		switch {
		case r == '\n' || r == '\r' || r == '\t' || unicode.IsSpace(r):
			space++
		case unicode.In(r, unicode.Han):
			cjk++
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if unicode.IsDigit(r) {
				digit++
			} else {
				latin++
			}
		case isCommonCodeSymbol(r):
			symbol++
		default:
			other++
		}
	}

	sum := float64(cjk)*p.cjk + float64(latin)*p.latin + float64(digit)*p.digit + float64(space)*p.space + float64(symbol)*p.symbol + float64(other)*p.other
	tokens := int(math.Ceil(sum))
	if tokens < 1 {
		return 1
	}
	return tokens
}

type tokenProfile struct {
	cjk    float64
	latin  float64
	digit  float64
	space  float64
	symbol float64
	other  float64
}

func tokenProfileForModel(model string, text string) tokenProfile {
	m := strings.ToLower(strings.TrimSpace(model))
	isCode := looksLikeCode(text)

	p := tokenProfile{
		cjk:    1.50,
		latin:  0.25,
		digit:  0.25,
		space:  0.05,
		symbol: 0.33,
		other:  0.50,
	}

	switch {
	case strings.Contains(m, "qwen"):
		p.latin = 0.27
		p.digit = 0.27
	case strings.Contains(m, "deepseek"):
		p.latin = 0.26
		p.digit = 0.26
	case strings.Contains(m, "glm"):
		p.latin = 0.26
		p.digit = 0.26
	case strings.Contains(m, "kimi"):
		p.latin = 0.26
		p.digit = 0.26
	}

	if isCode {
		p.cjk = 1.20
		p.latin = 0.30
		p.digit = 0.30
		p.space = 0.06
		p.symbol = 0.35
		p.other = 0.45
	}

	return p
}

func looksLikeCode(s string) bool {
	if strings.Contains(s, "```") {
		return true
	}
	if strings.Contains(s, "*** Begin Patch") || strings.Contains(s, "diff --git") {
		return true
	}
	snippet := s
	if len(snippet) > 4000 {
		snippet = snippet[:4000]
	}
	score := 0
	if strings.Contains(snippet, "func ") || strings.Contains(snippet, "package ") {
		score += 2
	}
	if strings.Contains(snippet, "{") || strings.Contains(snippet, "}") {
		score++
	}
	if strings.Contains(snippet, ";") {
		score++
	}
	if strings.Count(snippet, "\n") >= 8 {
		score++
	}
	return score >= 3
}

func isCommonCodeSymbol(r rune) bool {
	switch r {
	case '{', '}', '[', ']', '(', ')', '<', '>', ';', ':', ',', '.', '/', '\\', '|', '&', '^', '%', '$', '#', '@', '!', '?', '+', '-', '*', '=', '~', '`', '"', '\'':
		return true
	default:
		return false
	}
}
