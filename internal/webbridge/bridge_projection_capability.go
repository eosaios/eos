package webbridge

// Capability 域 projection：从 adapter 只读快照加载 MCP / LSP / Skill / Plugin 卡片。

func (s *BridgeService) loadMCPServers() []MCPServerCard {
	items := s.mcpServersReadOnly()
	out := make([]MCPServerCard, 0, len(items))
	for _, item := range items {
		out = append(out, MCPServerCard{
			Name:    item.Name,
			Type:    item.Type,
			Target:  item.Target,
			Enabled: item.Enabled,
		})
	}
	return out
}

func (s *BridgeService) loadLSPServers() []LSPServerCard {
	items := s.lspServersReadOnly()
	out := make([]LSPServerCard, 0, len(items))
	for _, item := range items {
		out = append(out, LSPServerCard{
			Language: item.Language,
			Status:   item.Status,
			Command:  item.Command,
		})
	}
	return out
}

func (s *BridgeService) loadSkills() []SkillCard {
	items := s.skillsReadOnly()
	out := make([]SkillCard, 0, len(items))
	for _, item := range items {
		out = append(out, SkillCard{
			Name:                   item.Name,
			Description:            item.Description,
			Source:                 item.Source,
			ArgumentHint:           item.ArgumentHint,
			BaseDir:                item.BaseDir,
			AllowedTools:           append([]string(nil), item.AllowedTools...),
			Enabled:                item.Enabled,
			Active:                 item.Active,
			DisableModelInvocation: item.DisableModelInvocation,
			UserInvocable:          item.UserInvocable,
		})
	}
	return out
}

func (s *BridgeService) loadPlugins() []PluginCard {
	items := s.pluginsReadOnly()
	out := make([]PluginCard, 0, len(items))
	for _, item := range items {
		out = append(out, PluginCard{
			Name:        item.Name,
			Description: item.Description,
			Source:      item.Source,
			Command:     item.Command,
			Enabled:     item.Enabled,
		})
	}
	return out
}
