package main

import (
	"regexp"
	"strings"
)

var diagnosticCredential = regexp.MustCompile(`(?i)(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|password|encrypted[_-]?key)([\s"':=]+)[^\s,;]+`)
var diagnosticBearer = regexp.MustCompile(`(?i)bearer\s+[^\s,;]+`)
var diagnosticKey = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+`)

func redactDiagnostics(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	text = diagnosticBearer.ReplaceAllString(text, "Bearer [REDACTED]")
	text = diagnosticCredential.ReplaceAllString(text, "$1=[REDACTED]")
	return diagnosticKey.ReplaceAllString(text, "[REDACTED]")
}
