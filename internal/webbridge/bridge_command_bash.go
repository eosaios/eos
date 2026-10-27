package webbridge

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1

import (
	"context"
	"errors"
	"strings"
	"time"
)

func (svc *CommandService) RunBashCommand(input string) (BootstrapState, error) {
	s := svc.bridge
	if s == nil {
		return BootstrapState{}, errors.New("bridge service is not available")
	}
	command := strings.TrimSpace(input)
	if command == "" {
		return s.LoadBootstrap(), errors.New("bash command is required")
	}
	now := time.Now().Format(time.RFC3339)
	s.stateMu.Lock()
	s.bash = BashState{
		Command:   command,
		Output:    []string{"$ " + command, "Connecting to Bash bridge..."},
		Status:    "running",
		UpdatedAt: now,
	}
	s.emitShellUpdated()
	s.stateMu.Unlock()
	go svc.runBashCommand(command)
	return s.LoadBootstrap(), nil
}

func (svc *CommandService) runBashCommand(command string) {
	s := svc.bridge
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	stream, err := s.runBashRPC(ctx, command)
	if err != nil {
		s.stateMu.Lock()
		svc.appendBashOutputLocked(err.Error())
		s.bash.Status = "failed"
		s.bash.UpdatedAt = time.Now().Format(time.RFC3339)
		s.pushNotificationLocked("Bash Failed", err.Error(), "danger")
		s.emitShellUpdated()
		s.stateMu.Unlock()
		return
	}
	finalized := false
	for event := range stream {
		message := strings.TrimSpace(event.EffectiveMessage())
		s.stateMu.Lock()
		switch event.Kind() {
		case "request.failed":
			svc.appendBashOutputLocked(fallbackText(message, "Command failed"))
			s.bash.Status = "failed"
			s.pushNotificationLocked("Bash Failed", fallbackText(message, "Command failed"), "danger")
			finalized = true
		case "text.final":
			svc.appendBashOutputLocked(fallbackText(message, "Command completed"))
			s.bash.Status = "completed"
			s.pushNotificationLocked("Bash Completed", command, "success")
			finalized = true
		default:
			svc.appendBashOutputLocked(message)
		}
		s.bash.UpdatedAt = time.Now().Format(time.RFC3339)
		s.emitShellUpdated()
		s.stateMu.Unlock()
	}
	if finalized {
		return
	}
	s.stateMu.Lock()
	s.bash.Status = "completed"
	s.bash.UpdatedAt = time.Now().Format(time.RFC3339)
	s.emitShellUpdated()
	s.stateMu.Unlock()
}

func (svc *CommandService) appendBashOutputLocked(line string) {
	s := svc.bridge
	if s == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	s.bash.Output = append(s.bash.Output, line)
	if len(s.bash.Output) > 48 {
		s.bash.Output = append([]string(nil), s.bash.Output[len(s.bash.Output)-48:]...)
	}
}
